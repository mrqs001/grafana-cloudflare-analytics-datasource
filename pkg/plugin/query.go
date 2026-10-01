package plugin

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/mrqs001/grafana-cloudflare-analytics-datasource/pkg/cloudflare"
)

var wholeVariablePattern = regexp.MustCompile(`^\$(?:[a-zA-Z_]\w*|\{[^}]+\})$`)
var zonePattern = regexp.MustCompile(`^[a-fA-F0-9]{32}$`)
var dimensions = map[string]string{
	"status": "edgeResponseStatus", "originStatus": "originResponseStatus", "hostname": "clientRequestHTTPHost", "cacheStatus": "cacheStatus", "country": "clientCountryName", "colo": "coloCode", "method": "clientRequestHTTPMethodName", "requestSource": "requestSource", "path": "clientRequestPath", "securityAction": "securityAction", "securitySource": "securitySource",
}
var intervals = []time.Duration{time.Minute, 5 * time.Minute, 15 * time.Minute, time.Hour, 6 * time.Hour, 24 * time.Hour}

type Filter struct {
	Field    string   `json:"field"`
	Operator string   `json:"operator"`
	Values   []string `json:"values"`
}
type Query struct {
	ZoneIDs   []string `json:"zoneIds"`
	ZoneMode  string   `json:"zoneMode"`
	ZoneID    string   `json:"zoneId"`
	Metric    string   `json:"metric"`
	GroupBy   []string `json:"groupBy"`
	Filters   []Filter `json:"filters"`
	Interval  string   `json:"interval"`
	Format    string   `json:"format"`
	MaxSeries int      `json:"maxSeries"`
	Fill      string   `json:"fill"`
}
type plan struct {
	query     Query
	from, to  time.Time
	interval  time.Duration
	timeField string
	fields    []string
	filter    map[string]any
	settings  cloudflare.Settings
}

func validate(q *Query, defaultZone string) error {
	if q.ZoneID == "" {
		q.ZoneID = defaultZone
	}
	if !zonePattern.MatchString(q.ZoneID) {
		return errors.New("provide a valid 32-character zone ID")
	}
	if q.Metric == "" {
		q.Metric = "requests"
	}
	switch q.Metric {
	case "requests", "requestRate", "bytes", "bandwidth":
	default:
		return errors.New("unsupported metric")
	}
	if q.Format == "" {
		q.Format = "timeSeries"
	}
	if q.Format != "timeSeries" && q.Format != "total" {
		return errors.New("format must be timeSeries or total")
	}
	if q.Fill == "" {
		q.Fill = "null"
	}
	if q.Fill != "null" && q.Fill != "zero" {
		return errors.New("fill must be null or zero")
	}
	if len(q.GroupBy) > 3 {
		return errors.New("select at most three grouping dimensions")
	}
	seen := map[string]bool{}
	for _, key := range q.GroupBy {
		if dimensions[key] == "" || seen[key] {
			return errors.New("unknown or duplicate grouping dimension")
		}
		seen[key] = true
	}
	if q.MaxSeries == 0 {
		q.MaxSeries = 20
	}
	if q.MaxSeries < 1 || q.MaxSeries > 200 {
		return errors.New("series limit must be between 1 and 200")
	}
	if len(q.Filters) > 20 {
		return errors.New("at most 20 filters are supported")
	}
	for _, f := range q.Filters {
		if dimensions[f.Field] == "" {
			return errors.New("unsupported filter field")
		}
		switch f.Operator {
		case "eq", "neq", "in", "notIn":
		case "gt", "geq", "lt", "leq":
			if f.Field != "status" && f.Field != "originStatus" {
				return errors.New("range filters are supported for edge and origin status")
			}
		case "like", "notLike":
			if f.Field != "path" {
				return errors.New("pattern filters are supported for URI path")
			}
		default:
			return errors.New("unsupported filter operator")
		}
		if len(f.Values) == 0 || len(f.Values) > 100 {
			return errors.New("each filter needs 1 to 100 values")
		}
		if f.Operator != "in" && f.Operator != "notIn" && len(f.Values) != 1 {
			return errors.New("this filter needs one value; use in / not in for multi-value variables")
		}
		for _, v := range f.Values {
			if len(v) > 2048 || wholeVariablePattern.MatchString(v) {
				return errors.New("filter contains an unresolved variable or exceeds 2048 characters")
			}
			if f.Field == "status" || f.Field == "originStatus" {
				n, e := strconv.Atoi(v)
				if e != nil || n < 0 || n > 999 {
					return errors.New("status filter must be an integer from 0 to 999")
				}
			}
		}
	}
	return nil
}
func makePlan(q Query, dq backend.DataQuery, s cloudflare.Settings, now time.Time) (plan, error) {
	p := plan{query: q, from: dq.TimeRange.From.UTC(), to: dq.TimeRange.To.UTC(), settings: s, filter: map[string]any{}}
	if !p.to.After(p.from) {
		return p, errors.New("time range must have a positive duration")
	}
	if p.from.Before(now.Add(-time.Duration(s.NotOlderThan) * time.Second)) {
		return p, fmt.Errorf("requested range exceeds this zone's retention of %.1f days; choose a newer start time", float64(s.NotOlderThan)/86400)
	}
	if p.to.After(now.Add(time.Minute)) {
		return p, errors.New("future time ranges are not supported")
	}
	target := dq.Interval
	maxPoints := dq.MaxDataPoints
	if maxPoints <= 0 {
		maxPoints = 600
	}
	if maxPoints > 2000 {
		maxPoints = 2000
	}
	required := time.Duration(math.Ceil(float64(p.to.Sub(p.from)) / float64(maxPoints)))
	if target < required {
		target = required
	}
	if q.Interval != "" && q.Interval != "auto" {
		var err error
		p.interval, err = time.ParseDuration(q.Interval)
		if err != nil {
			return p, errors.New("unsupported interval")
		}
		valid := false
		for _, i := range intervals {
			if p.interval == i {
				valid = true
			}
		}
		if !valid {
			return p, errors.New("supported intervals: 1m, 5m, 15m, 1h, 6h, 24h")
		}
	} else {
		p.interval = intervals[len(intervals)-1]
		for _, i := range intervals {
			if i >= target {
				p.interval = i
				break
			}
		}
	}
	if p.query.Format == "timeSeries" && p.to.Sub(p.from)/p.interval > 2000 {
		return p, errors.New("interval produces more than 2000 time buckets; choose Auto or a larger interval")
	}
	// Use the coarsest native dimension that divides the requested bucket exactly.
	if q.Format == "timeSeries" {
		for _, x := range []struct {
			d     time.Duration
			field string
		}{{24 * time.Hour, "date"}, {time.Hour, "datetimeHour"}, {15 * time.Minute, "datetimeFifteenMinutes"}, {5 * time.Minute, "datetimeFiveMinutes"}, {time.Minute, "datetimeMinute"}} {
			if p.interval%x.d == 0 && s.Has("dimensions_"+x.field) {
				p.timeField = x.field
				break
			}
		}
		if p.timeField == "" {
			return p, errors.New("no supported time dimension is available for this interval on this plan")
		}
		p.fields = append(p.fields, p.timeField)
	}
	for _, f := range q.GroupBy {
		field := dimensions[f]
		if !s.Has("dimensions_" + field) {
			return p, fmt.Errorf("grouping by %s is unavailable on this zone's plan", f)
		}
		p.fields = append(p.fields, field)
	}
	if !s.Has("count") {
		return p, errors.New("request count is unavailable")
	}
	if (q.Metric == "bytes" || q.Metric == "bandwidth") && !s.Has("sum_edgeResponseBytes") {
		return p, errors.New("response bytes are unavailable on this plan")
	}
	clauses := []map[string]any{}
	for _, f := range q.Filters {
		field := dimensions[f.Field]
		if !s.Has("dimensions_" + field) {
			return p, fmt.Errorf("filter %s is unavailable on this zone's plan", f.Field)
		}
		values := make([]any, len(f.Values))
		for i, v := range f.Values {
			values[i] = v
			if f.Field == "status" || f.Field == "originStatus" {
				values[i], _ = strconv.Atoi(v)
			}
		}
		suffix := map[string]string{"eq": "", "neq": "_neq", "in": "_in", "notIn": "_notin", "gt": "_gt", "geq": "_geq", "lt": "_lt", "leq": "_leq", "like": "_like", "notLike": "_notlike"}[f.Operator]
		var value any = values
		if f.Operator != "in" && f.Operator != "notIn" {
			value = values[0]
		}
		clauses = append(clauses, map[string]any{field + suffix: value})
	}
	if len(clauses) > 0 {
		p.filter["AND"] = clauses
	}
	return p, nil
}

type queryStats struct {
	Ranked                 bool
	SeriesLowerBound       bool
	Calls, Rows, CacheHits int
	MaxSampleInterval      float64
	SamplingKnown          bool
}

type fetchBudget struct{ calls, rows, cacheHits int }

func (d *Datasource) fetch(ctx context.Context, p plan) ([]cloudflare.Row, queryStats, error) {
	return d.fetchWithBudget(ctx, p, &fetchBudget{})
}
func (d *Datasource) fetchWithBudget(ctx context.Context, p plan, budget *fetchBudget) ([]cloudflare.Row, queryStats, error) {
	stats := queryStats{SamplingKnown: p.settings.Has("avg_sampleInterval")}
	all := []cloudflare.Row{}
	limit := min(p.settings.MaxPageSize, 10000)
	// Ranking is exact over one supported range. Never merge per-chunk top-N lists:
	// a globally important group could be absent from every chunk's local top-N.
	stats.Ranked = p.query.Format == "total" && len(p.query.GroupBy) > 0 && p.to.Sub(p.from) <= time.Duration(p.settings.MaxDuration)*time.Second
	if stats.Ranked {
		limit = min(limit, p.query.MaxSeries+1)
	}
	var fetchRange func(time.Time, time.Time) error
	fetchRange = func(from, to time.Time) error {
		if budget.calls >= 48 {
			return errors.New("query exceeded 48 Cloudflare requests; narrow the time range or reduce grouping cardinality")
		}
		filter := make(map[string]any, len(p.filter)+2)
		for k, v := range p.filter {
			filter[k] = v
		}
		filter["datetime_geq"] = from.Format(time.RFC3339Nano)
		filter["datetime_lt"] = to.Format(time.RFC3339Nano)
		budget.calls++
		rows, cached, err := d.client.Rows(ctx, p.query.ZoneID, filter, p.fields, limit, p.query.Metric == "bytes" || p.query.Metric == "bandwidth", stats.SamplingKnown, stats.Ranked)
		if err != nil {
			return err
		}
		if cached {
			stats.CacheHits++
			budget.cacheHits++
		} else {
			stats.Calls++
		}
		if stats.Ranked && len(rows) >= limit {
			stats.SeriesLowerBound = true
		}
		if !stats.Ranked && len(rows) >= limit {
			// A full page may be truncated. Discard it and split into disjoint ranges.
			// Split down to one second; never return an incomplete total as a complete one.
			mid := from.Add(to.Sub(from) / 2).Truncate(time.Second)
			if !mid.After(from) || !mid.Before(to) {
				return errors.New("row limit reached within one second; add filters or remove a grouping dimension")
			}
			if err := fetchRange(from, mid); err != nil {
				return err
			}
			return fetchRange(mid, to)
		}
		stats.Rows += len(rows)
		budget.rows += len(rows)
		if budget.rows > 100000 {
			return errors.New("query exceeded 100000 rows; add filters or use a coarser interval")
		}
		for _, r := range rows {
			stats.MaxSampleInterval = max(stats.MaxSampleInterval, r.Avg.SampleInterval)
		}
		all = append(all, rows...)
		return nil
	}
	for from := p.from; from.Before(p.to); {
		to := from.Add(time.Duration(p.settings.MaxDuration) * time.Second)
		if to.After(p.to) {
			to = p.to
		}
		if err := fetchRange(from, to); err != nil {
			return nil, stats, err
		}
		from = to
	}
	return all, stats, nil
}

type series struct {
	labels data.Labels
	values map[int64]float64
	total  float64
}

func frames(p plan, rows []cloudflare.Row, stats queryStats, ref string) (data.Frames, error) {
	groups := map[string]*series{}
	for _, r := range rows {
		labels := data.Labels{}
		for _, g := range p.query.GroupBy {
			v, ok := r.Dimensions[dimensions[g]]
			if !ok {
				return nil, errors.New("Cloudflare omitted a requested grouping dimension")
			}
			labels[g] = fmt.Sprint(v)
		}
		key := labels.String()
		s := groups[key]
		if s == nil {
			s = &series{labels: labels, values: map[int64]float64{}}
			groups[key] = s
		}
		t := p.from
		if p.timeField != "" {
			raw, ok := r.Dimensions[p.timeField].(string)
			if !ok {
				return nil, errors.New("Cloudflare omitted the time dimension")
			}
			var err error
			t, err = time.Parse(time.RFC3339, raw)
			if p.timeField == "date" {
				t, err = time.Parse("2006-01-02", raw)
			}
			if err != nil {
				return nil, errors.New("Cloudflare returned an invalid bucket timestamp")
			}
		}
		bucket := (t.Unix() / int64(p.interval.Seconds())) * int64(p.interval.Seconds())
		value := r.Count
		if p.query.Metric == "bytes" || p.query.Metric == "bandwidth" {
			value = r.Sum.Bytes
		}
		// Adaptive group aggregates are already estimates. NEVER multiply by sampleInterval.
		s.values[bucket] += value
		s.total += value
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		a, b := groups[keys[i]].total, groups[keys[j]].total
		if a == b {
			return keys[i] < keys[j]
		}
		return a > b
	})
	totalSeries := len(keys)
	if len(keys) > p.query.MaxSeries {
		keys = keys[:p.query.MaxSeries]
	}
	notices := []data.Notice{{Severity: data.NoticeSeverityInfo, Text: "Cloudflare adaptive analytics returns estimates. Missing buckets are unknown unless zero fill is selected; recent data may be delayed."}}
	if stats.MaxSampleInterval > 1 {
		notices = append(notices, data.Notice{Severity: data.NoticeSeverityWarning, Text: fmt.Sprintf("Sampled data: maximum returned group avg(sampleInterval) = %.2f. Counts and bytes are already scaled by Cloudflare.", stats.MaxSampleInterval)})
	}
	if !stats.SamplingKnown {
		notices = append(notices, data.Notice{Severity: data.NoticeSeverityWarning, Text: "Sampling metadata is unavailable on this plan; values can still be estimates."})
	}
	if totalSeries > len(keys) || stats.SeriesLowerBound {
		notices = append(notices, data.Notice{Severity: data.NoticeSeverityWarning, Text: fmt.Sprintf("Showing top %d of at least %d series ranked over the full range. Displayed series do not represent the full total.", len(keys), totalSeries)})
	}
	meta := func() *data.FrameMeta {
		return &data.FrameMeta{Notices: notices, Custom: map[string]any{"dataset": cloudflare.Dataset, "intervalSeconds": p.interval.Seconds(), "rangeFrom": p.from, "rangeTo": p.to, "bucketTimestamp": "start", "rangeSemantics": "[from,to)", "apiRequests": stats.Calls, "cacheHits": stats.CacheHits, "rows": stats.Rows, "maxSampleInterval": stats.MaxSampleInterval, "samplingKnown": stats.SamplingKnown, "totalSeries": totalSeries, "seriesCountIsLowerBound": stats.SeriesLowerBound, "serverRanked": stats.Ranked}}
	}
	unit := "short"
	switch p.query.Metric {
	case "requestRate":
		unit = "reqps"
	case "bytes":
		unit = "bytes"
	case "bandwidth":
		unit = "Bps"
	}
	rate := func(value, seconds float64) float64 {
		if p.query.Metric == "requestRate" || p.query.Metric == "bandwidth" {
			return value / seconds
		}
		return value
	}
	if p.query.Format == "total" {
		fields := []*data.Field{}
		for _, g := range p.query.GroupBy {
			values := []string{}
			for _, k := range keys {
				values = append(values, groups[k].labels[g])
			}
			fields = append(fields, data.NewField(g, nil, values))
		}
		values := []float64{}
		for _, k := range keys {
			values = append(values, rate(groups[k].total, p.to.Sub(p.from).Seconds()))
		}
		value := data.NewField(p.query.Metric, nil, values)
		value.Config = &data.FieldConfig{Unit: unit}
		fields = append(fields, value)
		f := data.NewFrame(p.query.Metric, fields...)
		f.RefID = ref
		f.Meta = meta()
		f.Meta.PreferredVisualization = data.VisTypeTable
		return data.Frames{f}, nil
	}
	out := data.Frames{}
	for _, key := range keys {
		s := groups[key]
		times := []time.Time{}
		values := []*float64{}
		for t := time.Unix((p.from.Unix()/int64(p.interval.Seconds()))*int64(p.interval.Seconds()), 0).UTC(); t.Before(p.to); t = t.Add(p.interval) {
			start, end := t, t.Add(p.interval)
			if start.Before(p.from) {
				start = p.from
			}
			if end.After(p.to) {
				end = p.to
			}
			times = append(times, start)
			v, ok := s.values[t.Unix()]
			if ok || p.query.Fill == "zero" {
				v = rate(v, end.Sub(start).Seconds())
				values = append(values, &v)
			} else {
				values = append(values, nil)
			}
		}
		f := data.NewFrame(p.query.Metric, data.NewField("Time", nil, times), data.NewField(p.query.Metric, s.labels, values))
		f.RefID = ref
		f.Meta = meta()
		f.Meta.Type = data.FrameTypeTimeSeriesMulti
		f.Meta.PreferredVisualization = data.VisTypeGraph
		f.Fields[1].Config = &data.FieldConfig{Unit: unit}
		out = append(out, f)
	}
	if len(out) == 0 {
		f := data.NewFrame(p.query.Metric, data.NewField("Time", nil, []time.Time{}), data.NewField(p.query.Metric, nil, []float64{}))
		f.RefID = ref
		f.Meta = meta()
		f.Meta.Notices = append(f.Meta.Notices, data.Notice{Severity: data.NoticeSeverityInfo, Text: "No matching analytics groups were returned. This does not prove zero origin traffic."})
		out = append(out, f)
	}
	return out, nil
}
