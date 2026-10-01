package plugin

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/data"
	"github.com/mrqs001/grafana-cloudflare-analytics-datasource/pkg/cloudflare"
)

const maxQueryZones = 20

// Defaults are a convenience, not an authorization boundary. Cloudflare token
// scope and Grafana datasource permissions remain authoritative.
func (d *Datasource) resolveZones(ctx context.Context, q Query) ([]cloudflare.Zone, error) {
	mode, ids := q.ZoneMode, q.ZoneIDs
	if mode == "" {
		if len(ids) > 0 || q.ZoneID != "" {
			mode = "selected"
		} else {
			mode = "default"
		}
	}
	if mode == "default" {
		mode, ids = d.settings.DefaultZoneMode, d.settings.DefaultZoneIDs
		if mode == "" {
			if len(ids) > 0 || d.settings.DefaultZoneID != "" {
				mode = "selected"
			} else {
				mode = "all"
			}
		}
		if len(ids) == 0 && d.settings.DefaultZoneID != "" {
			ids = []string{d.settings.DefaultZoneID}
		}
	} else if mode == "selected" && len(ids) == 0 && q.ZoneID != "" {
		ids = []string{q.ZoneID}
	}
	if mode != "selected" && mode != "all" {
		return nil, errors.New("zone selection must be datasource default, selected zones or all zones")
	}
	if mode == "selected" && len(ids) == 0 {
		return nil, errors.New("select at least one zone, or choose All zones")
	}
	if mode == "all" {
		ids = nil
	}
	unique := map[string]bool{}
	for _, id := range ids {
		if !zonePattern.MatchString(id) {
			return nil, errors.New("zone IDs must contain 32 hexadecimal characters; resolve dashboard variables before querying")
		}
		unique[strings.ToLower(id)] = true
	}
	if mode == "selected" && len(unique) > maxQueryZones {
		return nil, fmt.Errorf("select at most %d zones per query", maxQueryZones)
	}
	discovered, err := d.client.Zones(ctx)
	if mode == "all" {
		if err != nil {
			return nil, fmt.Errorf("all-zone discovery failed: %w", err)
		}
		if len(discovered) == 0 {
			return nil, errors.New("no zones are visible to this token")
		}
		for _, z := range discovered {
			unique[z.ID] = true
		}
	}
	if len(unique) > maxQueryZones {
		return nil, fmt.Errorf("all zones includes %d zones; select at most %d per query to stay within API budgets", len(unique), maxQueryZones)
	}
	names := map[string]string{}
	for _, z := range discovered {
		names[z.ID] = z.Name
	}
	// Explicit IDs keep working without Zone Read. Analytics access is checked
	// separately for every selected zone, including IDs absent from discovery.
	out := make([]cloudflare.Zone, 0, len(unique))
	for id := range unique {
		name := names[id]
		if name == "" {
			name = id
		}
		out = append(out, cloudflare.Zone{ID: id, Name: name})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return out[i].ID < out[j].ID
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func (d *Datasource) queryZones(ctx context.Context, qm Query, dq backend.DataQuery) backend.DataResponse {
	zones, err := d.resolveZones(ctx, qm)
	if err != nil {
		return queryError(err)
	}
	plans := make([]plan, len(zones))
	now := time.Now()
	commonInterval := time.Duration(0)
	// Plan every zone first. Unsupported fields/retention fail the whole query,
	// and Auto uses one shared output interval for meaningful overlays.
	for i, z := range zones {
		single := qm
		single.ZoneID = z.ID
		if err := validate(&single, ""); err != nil {
			return backend.ErrDataResponse(backend.StatusBadRequest, err.Error())
		}
		s, err := d.client.Settings(ctx, z.ID)
		if err != nil {
			return queryError(fmt.Errorf("zone %s: %w", z.Name, err))
		}
		p, err := makePlan(single, dq, s, now)
		if err != nil {
			return backend.ErrDataResponse(backend.StatusBadRequest, fmt.Sprintf("zone %s: %s", z.Name, err))
		}
		plans[i] = p
		commonInterval = max(commonInterval, p.interval)
	}
	budget := &fetchBudget{}
	out := data.Frames{}
	zoneMetadata := []map[string]any{}
	points := 0
	for i, p := range plans {
		p.query.Interval = commonInterval.String()
		p, err = makePlan(p.query, dq, p.settings, now)
		if err != nil {
			return queryError(err)
		}
		rows, stats, err := d.fetchWithBudget(ctx, p, budget)
		if err != nil {
			return queryError(fmt.Errorf("zone %s: %w", zones[i].Name, err))
		}
		fs, err := frames(p, rows, stats, dq.RefID)
		if err != nil {
			return queryError(err)
		}
		zoneMeta := map[string]any{}
		for key, value := range fs[0].Meta.Custom.(map[string]any) {
			zoneMeta[key] = value
		}
		zoneMeta["zone"], zoneMeta["zoneId"] = zones[i].Name, zones[i].ID
		zoneMetadata = append(zoneMetadata, zoneMeta)
		for _, f := range fs {
			points += f.Rows()
			if points > 400000 {
				return backend.ErrDataResponse(backend.StatusBadRequest, "query exceeds 400000 output points; use fewer zones, fewer top series or a larger interval")
			}
			f.Meta.Notices = append([]data.Notice(nil), f.Meta.Notices...)
			for j := range f.Meta.Notices {
				f.Meta.Notices[j].Text = zones[i].Name + ": " + f.Meta.Notices[j].Text
			}
			if qm.Format == "total" { // one table across zones, with explicit zone columns
				names, ids := make([]string, f.Rows()), make([]string, f.Rows())
				for j := range names {
					names[j] = zones[i].Name
					ids[j] = zones[i].ID
				}
				f.Fields = append([]*data.Field{data.NewField("zone", nil, names), data.NewField("zoneId", nil, ids)}, f.Fields...)
			} else {
				for _, field := range f.Fields[1:] {
					if field.Labels == nil {
						field.Labels = data.Labels{}
					}
					field.Labels["zone"] = zones[i].Name
					field.Labels["zoneId"] = zones[i].ID
					if field.Config == nil {
						field.Config = &data.FieldConfig{}
					}
					labels := []string{zones[i].Name}
					for _, g := range p.query.GroupBy {
						labels = append(labels, g+"="+field.Labels[g])
					}
					field.Config.DisplayNameFromDS = strings.Join(labels, " · ")
				}
			}
			metadata := f.Meta.Custom.(map[string]any)
			metadata["zoneId"], metadata["zone"], metadata["zoneCount"] = zones[i].ID, zones[i].Name, len(zones)
			if qm.Format == "total" && len(out) > 0 {
				dst := out[0]
				for row := 0; row < f.Rows(); row++ {
					values := make([]any, len(f.Fields))
					for col, field := range f.Fields {
						values[col] = field.At(row)
					}
					dst.AppendRow(values...)
				}
				dst.Meta.Notices = append(dst.Meta.Notices, f.Meta.Notices...)
			} else {
				out = append(out, f)
			}
		}
	}
	for _, f := range out {
		meta := f.Meta.Custom.(map[string]any)
		meta["totalAPIRequests"], meta["totalRows"] = budget.calls, budget.rows
		if qm.Format == "total" {
			delete(meta, "zoneId")
			delete(meta, "zone")
			// The merged table has per-zone sampling and ranking, not the first
			// zone's statistics masquerading as the whole query.
			for _, key := range []string{"apiRequests", "rows", "maxSampleInterval", "samplingKnown", "totalSeries", "seriesCountIsLowerBound", "serverRanked"} {
				delete(meta, key)
			}
			meta["zones"] = zoneMetadata
		}
	}
	return backend.DataResponse{Frames: out}
}
