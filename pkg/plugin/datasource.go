package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"sync"
	"time"

	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/instancemgmt"
	"github.com/mrqs001/grafana-cloudflare-analytics-datasource/pkg/cloudflare"
)

var (
	_ backend.QueryDataHandler      = (*Datasource)(nil)
	_ backend.CheckHealthHandler    = (*Datasource)(nil)
	_ backend.CallResourceHandler   = (*Datasource)(nil)
	_ instancemgmt.InstanceDisposer = (*Datasource)(nil)
)

type settings struct {
	DefaultZoneIDs  []string `json:"defaultZoneIds"`
	DefaultZoneMode string   `json:"defaultZoneMode"`
	DefaultZoneID   string   `json:"defaultZoneId"`
}
type analyticsClient interface {
	Close()
	Zones(context.Context) ([]cloudflare.Zone, error)
	Accounts(context.Context) ([]cloudflare.Account, error)
	Settings(context.Context, string) (cloudflare.Settings, error)
	Rows(context.Context, string, map[string]any, []string, int, bool, bool, bool) ([]cloudflare.Row, bool, error)
}
type Datasource struct {
	client   analyticsClient
	settings settings
}

func NewDatasource(_ context.Context, s backend.DataSourceInstanceSettings) (instancemgmt.Instance, error) {
	d := &Datasource{client: cloudflare.New(s.DecryptedSecureJSONData["apiToken"])}
	if len(s.JSONData) > 0 {
		if err := json.Unmarshal(s.JSONData, &d.settings); err != nil {
			return nil, errors.New("invalid datasource settings")
		}
	}
	return d, nil
}
func (d *Datasource) Dispose() { d.client.Close() }
func (d *Datasource) QueryData(ctx context.Context, req *backend.QueryDataRequest) (*backend.QueryDataResponse, error) {
	response := backend.NewQueryDataResponse()
	var mu sync.Mutex
	var wg sync.WaitGroup
	// Bound parallel queries and preserve independent errors by refId.
	workers := make(chan struct{}, 4)
	for _, q := range req.Queries {
		select {
		case workers <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		wg.Add(1)
		go func(q backend.DataQuery) {
			defer wg.Done()
			defer func() { <-workers }()
			r := d.query(ctx, q)
			mu.Lock()
			response.Responses[q.RefID] = r
			mu.Unlock()
		}(q)
	}
	wg.Wait()
	return response, nil
}
func (d *Datasource) query(ctx context.Context, q backend.DataQuery) backend.DataResponse {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	var qm Query
	if err := json.Unmarshal(q.JSON, &qm); err != nil {
		return backend.ErrDataResponse(backend.StatusBadRequest, "invalid query JSON")
	}
	return d.queryZones(ctx, qm, q)

}
func queryError(err error) backend.DataResponse {
	if errors.Is(err, context.DeadlineExceeded) {
		return backend.ErrDataResponse(backend.StatusTimeout, "query deadline exceeded; use range totals, fewer dimensions or a narrower time range")
	}
	status := backend.StatusBadGateway
	var e *cloudflare.APIError
	if errors.As(err, &e) {
		switch e.Status {
		case 401, 403:
			status = backend.StatusUnauthorized
		case 400:
			status = backend.StatusBadRequest
		case 429:
			status = backend.StatusTooManyRequests
		}
	}
	return backend.ErrDataResponse(status, err.Error())
}
func (d *Datasource) CheckHealth(ctx context.Context, _ *backend.CheckHealthRequest) (*backend.CheckHealthResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	zones, err := d.resolveZones(ctx, Query{ZoneMode: "default"})
	if err != nil {
		return unhealthy(err)
	}
	end := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Minute)
	for _, zone := range zones {
		s, err := d.client.Settings(ctx, zone.ID)
		if err != nil {
			return unhealthy(fmt.Errorf("zone %s: %w", zone.Name, err))
		}
		_, _, err = d.client.Rows(cloudflare.WithoutCache(ctx), zone.ID, map[string]any{"datetime_geq": end.Add(-time.Minute).Format(time.RFC3339), "datetime_lt": end.Format(time.RFC3339)}, nil, 1, false, s.Has("avg_sampleInterval"), false)
		if err != nil {
			return unhealthy(fmt.Errorf("zone %s: %w", zone.Name, err))
		}
	}

	return &backend.CheckHealthResult{Status: backend.HealthStatusOk, Message: fmt.Sprintf("Cloudflare HTTP analytics query succeeded for %d zone(s).", len(zones))}, nil
}
func unhealthy(err error) (*backend.CheckHealthResult, error) {
	return &backend.CheckHealthResult{Status: backend.HealthStatusError, Message: err.Error()}, nil
}
func (d *Datasource) CallResource(ctx context.Context, req *backend.CallResourceRequest, sender backend.CallResourceResponseSender) error {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if req.Method != "GET" {
		return sendJSON(sender, http.StatusMethodNotAllowed, map[string]string{"error": "only GET is supported"})
	}
	u, err := url.Parse(req.URL)
	if err != nil {
		return sendJSON(sender, 400, map[string]string{"error": "invalid resource URL"})
	}
	var value any
	switch req.Path {
	case "zones":
		var zones []cloudflare.Zone
		zones, err = d.client.Zones(ctx)
		// Zone pickers need only zone identity, never account names/emails.
		type zoneOption struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		options := []zoneOption{}
		for _, z := range zones {
			options = append(options, zoneOption{z.ID, z.Name})
		}
		value = options
	case "accounts":
		value, err = d.client.Accounts(ctx)
	case "settings":
		zone := u.Query().Get("zoneId")
		if zone == "" {
			zone = d.settings.DefaultZoneID
		}
		if !zonePattern.MatchString(zone) {
			return sendJSON(sender, 400, map[string]string{"error": "invalid zone ID"})
		}
		value, err = d.client.Settings(ctx, zone)
	case "values":
		zone, field := u.Query().Get("zoneId"), u.Query().Get("field")
		if zone == "" {
			zone = d.settings.DefaultZoneID
		}
		q := Query{ZoneID: zone, Metric: "requests", Format: "total", GroupBy: []string{field}, MaxSeries: 200}
		if err = validate(&q, d.settings.DefaultZoneID); err == nil {
			var s cloudflare.Settings
			s, err = d.client.Settings(ctx, zone)
			if err == nil {
				end := time.Now().UTC().Add(-2 * time.Minute).Truncate(time.Minute)
				var p plan
				p, err = makePlan(q, backend.DataQuery{TimeRange: backend.TimeRange{From: end.Add(-time.Hour), To: end}}, s, time.Now())
				if err == nil {
					var rows []cloudflare.Row
					rows, _, err = d.fetch(ctx, p)
					if err == nil {
						set := map[string]bool{}
						for _, r := range rows {
							if v, ok := r.Dimensions[dimensions[field]]; ok {
								set[fmt.Sprint(v)] = true
							}
						}
						values := []string{}
						for v := range set {
							values = append(values, v)
						}
						sort.Strings(values)
						value = values
					}
				}
			}
		}
	default:
		return sendJSON(sender, 404, map[string]string{"error": "unknown resource"})
	}
	if err != nil {
		return sendJSON(sender, 400, map[string]string{"error": err.Error()})
	}
	return sendJSON(sender, 200, value)
}
func sendJSON(sender backend.CallResourceResponseSender, status int, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return sender.Send(&backend.CallResourceResponse{Status: status, Headers: map[string][]string{"Content-Type": {"application/json"}, "Cache-Control": {"no-store"}}, Body: body})
}
