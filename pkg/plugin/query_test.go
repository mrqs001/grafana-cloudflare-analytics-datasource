package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/mrqs001/grafana-cloudflare-analytics-datasource/pkg/cloudflare"
)

func testSettings() cloudflare.Settings {
	s := cloudflare.Settings{Enabled: true, MaxDuration: 86400, NotOlderThan: 31 * 86400, MaxPageSize: 10000}
	s.AvailableFields = []string{"count", "sum_edgeResponseBytes", "avg_sampleInterval", "dimensions_datetimeMinute", "dimensions_datetimeFiveMinutes", "dimensions_datetimeFifteenMinutes", "dimensions_datetimeHour", "dimensions_date"}
	for _, field := range dimensions {
		s.AvailableFields = append(s.AvailableFields, "dimensions_"+field)
	}
	return s
}
func TestIntervalAndRetention(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		duration time.Duration
		interval time.Duration
		field    string
	}{{15 * time.Minute, time.Minute, "datetimeMinute"}, {6 * time.Hour, time.Minute, "datetimeMinute"}, {24 * time.Hour, 5 * time.Minute, "datetimeFiveMinutes"}, {7 * 24 * time.Hour, time.Hour, "datetimeHour"}, {30 * 24 * time.Hour, 6 * time.Hour, "datetimeHour"}} {
		q := Query{ZoneID: "01234567890123456789012345678901"}
		if err := validate(&q, ""); err != nil {
			t.Fatal(err)
		}
		p, err := makePlan(q, backend.DataQuery{TimeRange: backend.TimeRange{From: now.Add(-tc.duration), To: now}, MaxDataPoints: 600}, testSettings(), now)
		if err != nil || p.interval != tc.interval || p.timeField != tc.field {
			t.Errorf("%s got %s %s err %v", tc.duration, p.interval, p.timeField, err)
		}
	}
	q := Query{Format: "timeSeries"}
	_, err := makePlan(q, backend.DataQuery{TimeRange: backend.TimeRange{From: now.Add(-32 * 24 * time.Hour), To: now}}, testSettings(), now)
	if err == nil {
		t.Fatal("retention must fail")
	}
}
func TestPartialBucketRatesAndSamplingNotMultiplied(t *testing.T) {
	start := time.Date(2026, 10, 1, 12, 0, 30, 0, time.UTC)
	p := plan{query: Query{Metric: "requestRate", Format: "timeSeries", MaxSeries: 20}, from: start, to: start.Add(60 * time.Second), interval: time.Minute, timeField: "datetimeMinute"}
	var rows []cloudflare.Row
	_ = json.Unmarshal([]byte(`[{"count":60,"avg":{"sampleInterval":100},"dimensions":{"datetimeMinute":"2026-10-01T12:00:00Z"}},{"count":30,"avg":{"sampleInterval":100},"dimensions":{"datetimeMinute":"2026-10-01T12:01:00Z"}}]`), &rows)
	f, err := frames(p, rows, queryStats{MaxSampleInterval: 100, SamplingKnown: true}, "A")
	if err != nil {
		t.Fatal(err)
	}
	if *f[0].Fields[1].At(0).(*float64) != 2 || *f[0].Fields[1].At(1).(*float64) != 1 {
		t.Fatal("partial bucket rates incorrect or double scaled")
	}
	if f[0].Fields[0].At(0).(time.Time) != start {
		t.Fatal("timestamp lies outside requested range")
	}
}
func TestMergeSplitRowsTopSeriesAndNulls(t *testing.T) {
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	p := plan{query: Query{Metric: "requests", Format: "timeSeries", MaxSeries: 1, GroupBy: []string{"status"}}, from: start, to: start.Add(3 * time.Minute), interval: time.Minute, timeField: "datetimeMinute"}
	var rows []cloudflare.Row
	_ = json.Unmarshal([]byte(`[{"count":2,"dimensions":{"datetimeMinute":"2026-10-01T12:00:00Z","edgeResponseStatus":200}},{"count":3,"dimensions":{"datetimeMinute":"2026-10-01T12:00:00Z","edgeResponseStatus":200}},{"count":1,"dimensions":{"datetimeMinute":"2026-10-01T12:01:00Z","edgeResponseStatus":404}}]`), &rows)
	f, err := frames(p, rows, queryStats{}, "A")
	if err != nil {
		t.Fatal(err)
	}
	if len(f) != 1 || *f[0].Fields[1].At(0).(*float64) != 5 || f[0].Fields[1].At(1).(*float64) != nil {
		t.Fatal("merge/top/null semantics failed")
	}
	if f[0].Fields[1].Labels["status"] != "200" {
		t.Fatal("wrong top series")
	}
	if len(f[0].Meta.Notices) < 3 {
		t.Fatal("missing sampling/top notices")
	}
}
func TestTotalBandwidthAndEmpty(t *testing.T) {
	p := plan{query: Query{Metric: "bandwidth", Format: "total", MaxSeries: 20}, from: time.Unix(0, 0), to: time.Unix(60, 0), interval: time.Minute}
	var rows []cloudflare.Row
	_ = json.Unmarshal([]byte(`[{"count":10,"sum":{"edgeResponseBytes":600}}]`), &rows)
	f, err := frames(p, rows, queryStats{}, "A")
	if err != nil || math.Abs(f[0].Fields[0].At(0).(float64)-10) > 1e-9 {
		t.Fatal("bad total rate", err)
	}
	p.query.Format = "timeSeries"
	f, err = frames(p, nil, queryStats{}, "A")
	if err != nil || f[0].Fields[0].Len() != 0 {
		t.Fatal("empty results must remain empty")
	}
}
func TestFilterValidationAndInjection(t *testing.T) {
	for _, f := range []Filter{{Field: "status", Operator: "eq", Values: []string{"abc"}}, {Field: "hostname", Operator: "eq", Values: []string{"$host"}}, {Field: "hostname) {count}", Operator: "eq", Values: []string{"x"}}, {Field: "hostname", Operator: "eq", Values: []string{"a", "b"}}} {
		q := Query{ZoneID: "01234567890123456789012345678901", Filters: []Filter{f}}
		if err := validate(&q, ""); err == nil {
			t.Errorf("accepted invalid filter %+v", f)
		}
	}
	q := Query{ZoneID: "01234567890123456789012345678901", Filters: []Filter{{Field: "hostname", Operator: "in", Values: []string{"a\"} malicious", "b,c"}}}}
	if err := validate(&q, ""); err != nil {
		t.Fatal("literal values should remain safe GraphQL variables", err)
	}
}

type fakeClient struct {
	calls  int
	ranges [][2]time.Time
	failAt int
	full   bool
}

func (f *fakeClient) Close() {}
func (f *fakeClient) Zones(context.Context) ([]cloudflare.Zone, error) {
	return []cloudflare.Zone{{ID: "01234567890123456789012345678901"}}, nil
}
func (f *fakeClient) Accounts(context.Context) ([]cloudflare.Account, error) { return nil, nil }
func (f *fakeClient) Settings(context.Context, string) (cloudflare.Settings, error) {
	return testSettings(), nil
}
func (f *fakeClient) Rows(_ context.Context, _ string, filter map[string]any, _ []string, limit int, _ bool, _ bool, _ bool) ([]cloudflare.Row, error) {
	f.calls++
	a, _ := time.Parse(time.RFC3339Nano, filter["datetime_geq"].(string))
	b, _ := time.Parse(time.RFC3339Nano, filter["datetime_lt"].(string))
	f.ranges = append(f.ranges, [2]time.Time{a, b})
	if f.failAt == f.calls {
		return nil, errors.New("upstream error")
	}
	n := 1
	if f.full || b.Sub(a) > time.Minute {
		n = limit
	}
	rows := make([]cloudflare.Row, n)
	for i := range rows {
		rows[i].Count = b.Sub(a).Seconds()
		rows[i].Dimensions = map[string]any{"datetimeMinute": a.Truncate(time.Minute).Format(time.RFC3339)}
	}
	return rows, nil
}
func TestSplitFullPagesWithoutDoubleCounting(t *testing.T) {
	client := &fakeClient{}
	d := Datasource{client: client}
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	p := plan{query: Query{Metric: "requests", MaxSeries: 20, Format: "timeSeries"}, from: start, to: start.Add(4 * time.Minute), interval: time.Minute, timeField: "datetimeMinute", settings: testSettings()}
	p.settings.MaxPageSize = 2
	rows, stats, err := d.fetch(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	sum := 0.
	for _, r := range rows {
		sum += r.Count
	}
	if sum != 240 || stats.Calls != 7 {
		t.Fatalf("got sum %v calls %v", sum, stats.Calls)
	}
	for i := 1; i < len(client.ranges); i++ {
		if client.ranges[i][0].Before(start) || client.ranges[i][1].After(p.to) {
			t.Fatal("range widened")
		}
	}
}
func TestRangeChunkingAndNoPartialSuccess(t *testing.T) {
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	p := plan{query: Query{}, from: start, to: start.Add(3 * time.Minute), interval: time.Minute, settings: testSettings()}
	p.settings.MaxDuration = 60
	c := &fakeClient{}
	d := Datasource{client: c}
	rows, stats, err := d.fetch(context.Background(), p)
	if err != nil || len(rows) != 3 || stats.Calls != 3 {
		t.Fatal("chunks", err)
	}
	for i := 1; i < len(c.ranges); i++ {
		if !c.ranges[i][0].Equal(c.ranges[i-1][1]) {
			t.Fatal("gaps/overlap")
		}
	}
	d.client = &fakeClient{failAt: 2}
	rows, _, err = d.fetch(context.Background(), p)
	if err == nil || rows != nil {
		t.Fatal("partial results returned after upstream error")
	}
	d.client = &fakeClient{full: true}
	p.to = start.Add(time.Second)
	rows, _, err = d.fetch(context.Background(), p)
	if err == nil || rows != nil {
		t.Fatal("truncation must fail loudly")
	}
}
func TestQueryDataIndependentErrors(t *testing.T) {
	d := Datasource{client: &fakeClient{}}
	res, err := d.QueryData(context.Background(), &backend.QueryDataRequest{Queries: []backend.DataQuery{{RefID: "A", JSON: []byte(`{"zoneId":"bad"}`)}, {RefID: "B", JSON: []byte(`{"zoneId":"also-bad"}`)}}})
	if err != nil || len(res.Responses) != 2 || res.Responses["A"].Error == nil || res.Responses["B"].Error == nil {
		t.Fatal("independent failures", err)
	}
}

func TestServerRankedTotalsAreOnlyUsedForWholeRange(t *testing.T) {
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	p := plan{query: Query{Metric: "requests", Format: "total", GroupBy: []string{"path"}, MaxSeries: 2}, from: start, to: start.Add(4 * time.Minute), interval: time.Minute, settings: testSettings()}
	d := Datasource{client: &fakeClient{full: true}}
	rows, stats, err := d.fetch(context.Background(), p)
	if err != nil || !stats.Ranked || !stats.SeriesLowerBound || len(rows) != 3 || stats.Calls != 1 {
		t.Fatalf("ranked totals: %+v %v", stats, err)
	}
	p.settings.MaxDuration = 60
	d.client = &fakeClient{}
	_, stats, err = d.fetch(context.Background(), p)
	if err != nil || stats.Ranked || stats.Calls != 4 {
		t.Fatalf("cannot merge local top-N lists: %+v %v", stats, err)
	}
}
