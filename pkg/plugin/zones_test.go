package plugin

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/mrqs001/grafana-cloudflare-analytics-datasource/pkg/cloudflare"
)

const zoneA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const zoneB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

type zoneClient struct {
	fakeClient
	discoveryError bool
	failZone       string
	smallChunks    bool
}

func (c *zoneClient) Zones(context.Context) ([]cloudflare.Zone, error) {
	if c.discoveryError {
		return nil, errors.New("discovery denied")
	}
	return []cloudflare.Zone{{ID: zoneB, Name: "beta.example"}, {ID: zoneA, Name: "alpha.example"}}, nil
}
func (c *zoneClient) Settings(_ context.Context, id string) (cloudflare.Settings, error) {
	s := testSettings()
	if c.smallChunks {
		s.MaxDuration = 60
	}
	return s, nil
}
func (c *zoneClient) Rows(_ context.Context, id string, filter map[string]any, fields []string, _ int, _ bool, _ bool, _ bool) ([]cloudflare.Row, bool, error) {
	c.calls++
	if id == c.failZone {
		return nil, false, errors.New("analytics denied")
	}
	a, _ := time.Parse(time.RFC3339Nano, filter["datetime_geq"].(string))
	r := cloudflare.Row{Count: 3, Dimensions: map[string]any{}}
	if id == zoneB {
		r.Count = 7
	}
	for _, f := range fields {
		if strings.HasPrefix(f, "datetime") {
			r.Dimensions[f] = a.Truncate(time.Minute).Format(time.RFC3339)
		} else {
			r.Dimensions[f] = "200"
		}
	}
	return []cloudflare.Row{r}, false, nil
}
func TestZoneSelectionModesAndLegacy(t *testing.T) {
	d := Datasource{client: &zoneClient{}, settings: settings{DefaultZoneMode: "selected", DefaultZoneIDs: []string{zoneB}}}
	for _, tc := range []struct {
		name  string
		q     Query
		want  int
		first string
	}{
		{"default", Query{}, 1, zoneB}, {"explicit default", Query{ZoneMode: "default", ZoneID: zoneA}, 1, zoneB},
		{"legacy", Query{ZoneID: zoneA}, 1, zoneA}, {"multiple deduplicate", Query{ZoneMode: "selected", ZoneIDs: []string{zoneB, zoneA, strings.ToUpper(zoneA)}}, 2, zoneA},
		{"all ignores stale IDs", Query{ZoneMode: "all", ZoneIDs: []string{"stale"}}, 2, zoneA},
	} {
		t.Run(tc.name, func(t *testing.T) {
			zs, err := d.resolveZones(context.Background(), tc.q)
			if err != nil || len(zs) != tc.want || zs[0].ID != tc.first {
				t.Fatalf("selection: %v %v", zs, err)
			}
		})
	}
	d.settings = settings{DefaultZoneID: zoneA}
	zs, err := d.resolveZones(context.Background(), Query{})
	if err != nil || len(zs) != 1 || zs[0].ID != zoneA {
		t.Fatal("legacy default", err)
	}
	d.settings = settings{}
	zs, err = d.resolveZones(context.Background(), Query{})
	if err != nil || len(zs) != 2 {
		t.Fatal("blank default should discover all", err)
	}
	for _, q := range []Query{{ZoneMode: "selected"}, {ZoneMode: "invalid"}, {ZoneIDs: []string{"bad"}}} {
		if _, err = d.resolveZones(context.Background(), q); err == nil {
			t.Fatal("accepted invalid selection")
		}
	}
	d.client = &zoneClient{discoveryError: true}
	zs, err = d.resolveZones(context.Background(), Query{ZoneID: zoneA})
	if err != nil || zs[0].Name != zoneA {
		t.Fatal("manual ID without discovery", err)
	}
	if _, err = d.resolveZones(context.Background(), Query{ZoneMode: "all"}); err == nil {
		t.Fatal("all must require discovery")
	}
}
func TestMultiZoneFramesAndTotals(t *testing.T) {
	d := Datasource{client: &zoneClient{}}
	end := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Minute)
	dq := backend.DataQuery{RefID: "A", TimeRange: backend.TimeRange{From: end.Add(-time.Minute), To: end}}
	for _, format := range []string{"timeSeries", "total"} {
		r := d.queryZones(context.Background(), Query{ZoneMode: "all", Format: format}, dq)
		if r.Error != nil {
			t.Fatal(r.Error)
		}
		if format == "timeSeries" {
			if len(r.Frames) != 2 {
				t.Fatal("missing zone")
			}
			for i, f := range r.Frames {
				if f.Fields[1].Labels["zone"] == "" || f.Fields[1].Labels["zoneId"] == "" || f.Fields[1].Config.DisplayNameFromDS == "" {
					t.Fatal("missing zone identity")
				}
				want := 3.
				if i == 1 {
					want = 7
				}
				if *f.Fields[1].At(0).(*float64) != want {
					t.Fatal("cross-zone aggregation")
				}
				if f.Meta.Custom.(map[string]any)["totalAPIRequests"] != 2 {
					t.Fatal("incorrect shared request count")
				}
			}
		} else {
			if len(r.Frames) != 1 || r.Frames[0].Rows() != 2 {
				t.Fatal("totals must be one two-zone table")
			}
			f := r.Frames[0]
			if f.Fields[0].Name != "zone" || f.Fields[2].At(0).(float64) != 3 || f.Fields[2].At(1).(float64) != 7 {
				t.Fatal("incorrect totals")
			}
			if len(f.Meta.Custom.(map[string]any)["zones"].([]map[string]any)) != 2 {
				t.Fatal("missing per-zone metadata")
			}
		}
	}
	d.client = &zoneClient{failZone: zoneB}
	r := d.queryZones(context.Background(), Query{ZoneMode: "all"}, dq)
	if r.Error == nil || len(r.Frames) > 0 || !strings.Contains(r.Error.Error(), "beta.example") {
		t.Fatal("must not return partial zone results")
	}
}
func TestMultiZoneSharesRequestBudget(t *testing.T) {
	c := &zoneClient{smallChunks: true}
	d := Datasource{client: c}
	end := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Minute)
	r := d.queryZones(context.Background(), Query{ZoneMode: "all"}, backend.DataQuery{RefID: "A", TimeRange: backend.TimeRange{From: end.Add(-30 * time.Minute), To: end}})
	if r.Error == nil || len(r.Frames) > 0 || c.calls != 48 {
		t.Fatalf("shared request cap: calls=%d error=%v", c.calls, r.Error)
	}
}

func TestSingleZoneTotalsOmitZoneColumns(t *testing.T) {
	d := Datasource{client: &zoneClient{}}
	end := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Minute)
	for _, groups := range [][]string{nil, {"status"}} {
		r := d.queryZones(context.Background(), Query{ZoneID: zoneA, Format: "total", GroupBy: groups}, backend.DataQuery{RefID: "A", TimeRange: backend.TimeRange{From: end.Add(-time.Minute), To: end}})
		if r.Error != nil {
			t.Fatal(r.Error)
		}
		f := r.Frames[0]
		if len(f.Fields) != len(groups)+1 || f.Fields[len(groups)].Name != "requests" || f.Fields[len(groups)].At(0).(float64) != 3 {
			t.Fatal("unexpected single-zone table", f.Fields)
		}
		for _, field := range f.Fields {
			if field.Name == "zone" || field.Name == "zoneId" || len(field.Labels) > 0 {
				t.Fatal("zone identity leaked into single-zone columns")
			}
		}
		if len(f.Meta.Custom.(map[string]any)["zones"].([]map[string]any)) != 1 {
			t.Fatal("inspector lost zone metadata")
		}
	}
}
