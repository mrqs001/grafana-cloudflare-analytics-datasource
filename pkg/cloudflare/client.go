// Package cloudflare implements a read-only client for Cloudflare's current APIs.
package cloudflare

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

const APIURL = "https://api.cloudflare.com/client/v4"
const Dataset = "httpRequestsAdaptiveGroups"

type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string { return e.Message }

type cacheEntry struct {
	value   any
	expires time.Time
}
type Client struct {
	http           *http.Client
	baseURL, token string
	mu             sync.Mutex
	cache          map[string]cacheEntry
	slots          chan struct{}
}

func New(token string) *Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 4
	return newClient(token, APIURL, &http.Client{Timeout: 30 * time.Second, Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }})
}
func newClient(token, base string, h *http.Client) *Client {
	return &Client{http: h, baseURL: base, token: token, cache: map[string]cacheEntry{}, slots: make(chan struct{}, 4)}
}
func (c *Client) Close() { c.http.CloseIdleConnections() }
func (c *Client) clean(s string) string {
	if c.token != "" {
		s = strings.ReplaceAll(s, c.token, "[REDACTED]")
	}
	if len(s) > 600 {
		s = s[:600] + "…"
	}
	return s
}

// Only GET metadata and POST GraphQL queries are exposed. No mutation API exists.
func (c *Client) request(ctx context.Context, path string, payload any, out any) error {
	if c.token == "" {
		return &APIError{401, "API token is missing. Configure it in secure datasource settings."}
	}
	select {
	case c.slots <- struct{}{}:
		defer func() { <-c.slots }()
	case <-ctx.Done():
		return ctx.Err()
	}
	var body []byte
	var err error
	method := http.MethodGet
	if payload != nil {
		body, err = json.Marshal(payload)
		if err != nil {
			return err
		}
		method = http.MethodPost
	}
	for attempt := 0; attempt < 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bytes.NewReader(body))
		if err != nil {
			return errors.New("could not create Cloudflare request")
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("User-Agent", "cloudflare-analytics-grafana/0.1")
		resp, err := c.http.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return errors.New("Cloudflare connection failed or timed out; check backend network access")
		}
		b, readErr := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024+1))
		_ = resp.Body.Close()
		if readErr != nil {
			return errors.New("could not read Cloudflare response")
		}
		if len(b) > 16*1024*1024 {
			return errors.New("Cloudflare response exceeded 16 MiB; narrow the query")
		}
		if (resp.StatusCode == 429 || resp.StatusCode >= 500) && attempt < 2 {
			delay := time.Duration(1<<attempt) * time.Second
			if h := resp.Header.Get("Retry-After"); h != "" {
				if n, e := strconv.Atoi(h); e == nil {
					delay = time.Duration(n) * time.Second
				} else if t, e := http.ParseTime(h); e == nil {
					delay = time.Until(t)
				}
			}
			if delay > 5*time.Second {
				return &APIError{resp.StatusCode, "Cloudflare rate limit or temporary outage; retry later or reduce dashboard refresh frequency"}
			}
			if delay < 0 {
				delay = 0
			}
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
				continue
			case <-ctx.Done():
				timer.Stop()
				return ctx.Err()
			}
		}
		switch resp.StatusCode {
		case 401, 403:
			return &APIError{resp.StatusCode, "Cloudflare denied access. Check token validity, Analytics Read permission, and zone resource scope."}
		case 429:
			return &APIError{429, "Cloudflare rate limit reached. Reduce dashboard refresh frequency and concurrent queries."}
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return &APIError{resp.StatusCode, fmt.Sprintf("Cloudflare HTTP %d; retry later or check API availability", resp.StatusCode)}
		}
		if err = json.Unmarshal(b, out); err != nil {
			return errors.New("Cloudflare returned an invalid JSON response")
		}
		return nil
	}
	return errors.New("Cloudflare request failed")
}

type graphResponse struct {
	Data   json.RawMessage `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

func (c *Client) graphql(ctx context.Context, query string, variables map[string]any, out any) error {
	var r graphResponse
	if err := c.request(ctx, "/graphql", map[string]any{"query": query, "variables": variables}, &r); err != nil {
		return err
	}
	if len(r.Errors) > 0 {
		return &APIError{400, "Cloudflare GraphQL: " + c.clean(r.Errors[0].Message) + ". Check selected fields, plan limits, token permissions and time range."}
	}
	if len(r.Data) == 0 || string(r.Data) == "null" {
		return errors.New("Cloudflare returned no GraphQL data")
	}
	if err := json.Unmarshal(r.Data, out); err != nil {
		return errors.New("unexpected Cloudflare GraphQL response shape")
	}
	return nil
}

type Account struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type Zone struct {
	ID      string  `json:"id"`
	Name    string  `json:"name"`
	Account Account `json:"account"`
}

// Metadata is cached per datasource (and therefore per token), never metric data.
func (c *Client) metadata(ctx context.Context, key string, fetch func() (any, error)) (any, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if e, ok := c.cache[key]; ok && time.Now().Before(e.expires) {
		return e.value, nil
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	v, err := fetch()
	if err == nil {
		c.cache[key] = cacheEntry{v, time.Now().Add(5 * time.Minute)}
	}
	return v, err
}
func (c *Client) Zones(ctx context.Context) ([]Zone, error) {
	v, err := c.metadata(ctx, "zones", func() (any, error) {
		zones := []Zone{}
		for page := 1; page <= 200; page++ {
			var r struct {
				Success    bool   `json:"success"`
				Result     []Zone `json:"result"`
				ResultInfo struct {
					TotalPages int `json:"total_pages"`
				} `json:"result_info"`
			}
			if e := c.request(ctx, fmt.Sprintf("/zones?per_page=50&page=%d", page), nil, &r); e != nil {
				return nil, e
			}
			if !r.Success {
				return nil, errors.New("zone discovery denied; add Zone Read permission or enter a zone ID manually")
			}
			zones = append(zones, r.Result...)
			if page >= r.ResultInfo.TotalPages {
				return zones, nil
			}
		}
		return nil, errors.New("zone discovery exceeded 10000 zones; use a token scoped to fewer zones")
	})
	if err != nil {
		return nil, err
	}
	return v.([]Zone), nil
}
func (c *Client) Accounts(ctx context.Context) ([]Account, error) {
	zones, err := c.Zones(ctx)
	if err != nil {
		return nil, err
	}
	out := []Account{}
	seen := map[string]bool{}
	for _, z := range zones {
		if !seen[z.Account.ID] {
			out = append(out, z.Account)
			seen[z.Account.ID] = true
		}
	}
	return out, nil
}

type Settings struct {
	Enabled           bool     `json:"enabled"`
	AvailableFields   []string `json:"availableFields"`
	MaxDuration       int64    `json:"maxDuration"`
	MaxPageSize       int      `json:"maxPageSize"`
	NotOlderThan      int64    `json:"notOlderThan"`
	MaxNumberOfFields int      `json:"maxNumberOfFields"`
}

func (c *Client) Settings(ctx context.Context, zone string) (Settings, error) {
	v, err := c.metadata(ctx, "settings:"+zone, func() (any, error) {
		var r struct {
			Viewer struct {
				Zones []struct {
					Settings struct {
						HTTP Settings `json:"httpRequestsAdaptiveGroups"`
					} `json:"settings"`
				} `json:"zones"`
			} `json:"viewer"`
		}
		q := `query($zone:string!){viewer{zones(filter:{zoneTag:$zone}){settings{httpRequestsAdaptiveGroups{enabled availableFields maxDuration maxPageSize notOlderThan maxNumberOfFields}}}}}`
		if e := c.graphql(ctx, q, map[string]any{"zone": zone}, &r); e != nil {
			return nil, e
		}
		if len(r.Viewer.Zones) != 1 {
			return nil, errors.New("zone was not found or is outside the token's scope")
		}
		s := r.Viewer.Zones[0].Settings.HTTP
		if !s.Enabled {
			return nil, errors.New("HTTP analytics is unavailable for this zone or token; check Analytics Read permissions and plan")
		}
		if s.MaxDuration <= 0 || s.MaxPageSize <= 0 || s.NotOlderThan <= 0 {
			return nil, errors.New("Cloudflare returned invalid dataset limits")
		}
		return s, nil
	})
	if err != nil {
		return Settings{}, err
	}
	return v.(Settings), nil
}
func (s Settings) Has(field string) bool {
	for _, f := range s.AvailableFields {
		if f == field {
			return true
		}
	}
	return false
}

type Row struct {
	Count float64 `json:"count"`
	Sum   struct {
		Bytes float64 `json:"edgeResponseBytes"`
	} `json:"sum"`
	Avg struct {
		SampleInterval float64 `json:"sampleInterval"`
	} `json:"avg"`
	Dimensions map[string]any `json:"dimensions"`
}

// fields and time dimension must come from the planner's allowlist, never raw user input.
func (c *Client) Rows(ctx context.Context, zone string, filter map[string]any, dimensions []string, limit int, bytesMetric, sampling, ranked bool) ([]Row, error) {
	selection := "count"
	if bytesMetric {
		selection += " sum{edgeResponseBytes}"
	}
	if sampling {
		selection += " avg{sampleInterval}"
	}
	if len(dimensions) > 0 {
		selection += " dimensions{" + strings.Join(dimensions, " ") + "}"
	}
	order := ""
	if ranked {
		order = ",orderBy:[count_DESC]"
		if bytesMetric {
			order = ",orderBy:[sum_edgeResponseBytes_DESC]"
		}
	}
	q := `query($zone:string!,$filter:ZoneHttpRequestsAdaptiveGroupsFilter_InputObject!,$limit:uint64!){viewer{zones(filter:{zoneTag:$zone}){rows:httpRequestsAdaptiveGroups(limit:$limit,filter:$filter` + order + `){` + selection + `}}}}`
	var r struct {
		Viewer struct {
			Zones []struct {
				Rows []Row `json:"rows"`
			} `json:"zones"`
		} `json:"viewer"`
	}
	if err := c.graphql(ctx, q, map[string]any{"zone": zone, "filter": filter, "limit": limit}, &r); err != nil {
		return nil, err
	}
	if len(r.Viewer.Zones) != 1 {
		return nil, errors.New("zone was not found or is outside the token's scope")
	}
	return r.Viewer.Zones[0].Rows, nil
}
