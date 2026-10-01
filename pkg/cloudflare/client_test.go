package cloudflare

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestGraphQLErrorsAndRedaction(t *testing.T) {
	const secret = "test-only-not-a-real-credential"
	for _, status := range []int{200, 401, 403, 429, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("Authorization") != "Bearer "+secret {
					t.Error("missing authorization")
				}
				w.Header().Set("Retry-After", "10")
				w.WriteHeader(status)
				_ = json.NewEncoder(w).Encode(map[string]any{"errors": []any{map[string]string{"message": "bad token " + secret}}})
			}))
			defer server.Close()
			c := newClient(secret, server.URL, server.Client())
			var out any
			err := c.graphql(context.Background(), "query{viewer{__typename}}", nil, &out)
			if err == nil || strings.Contains(err.Error(), secret) {
				t.Fatalf("error missing or leaks secret: %v", err)
			}
		})
	}
}
func TestPaginationAndMetadataCache(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		page := r.URL.Query().Get("page")
		_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": []Zone{{ID: page, Name: "example.test", Account: Account{ID: "account"}}}, "result_info": map[string]int{"total_pages": 2}})
	}))
	defer server.Close()
	c := newClient("fixture", server.URL, server.Client())
	zones, err := c.Zones(context.Background())
	if err != nil || len(zones) != 2 {
		t.Fatalf("zones: %v %v", zones, err)
	}
	accounts, err := c.Accounts(context.Background())
	if err != nil || len(accounts) != 1 || calls.Load() != 2 {
		t.Fatalf("cache/dedup failed: %v %d", err, calls.Load())
	}
}
func TestRetryAndCancellation(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(429)
			return
		}
		_, _ = w.Write([]byte(`{"data":{"ok":true}}`))
	}))
	defer server.Close()
	c := newClient("fixture", server.URL, server.Client())
	var out any
	if err := c.graphql(context.Background(), "query{viewer{__typename}}", nil, &out); err != nil || calls.Load() != 2 {
		t.Fatalf("retry: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.graphql(ctx, "query{viewer{__typename}}", nil, &out); err == nil {
		t.Fatal("expected cancellation")
	}
}
func TestNoDataAndMalformedResponse(t *testing.T) {
	for _, body := range []string{`{"data":null}`, `not json`, `{"data":{},"errors":[{"message":"unknown field"}]}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(body)) }))
		c := newClient("fixture", server.URL, server.Client())
		var out any
		if err := c.graphql(context.Background(), "query{viewer{__typename}}", nil, &out); err == nil {
			t.Fatal("expected failure")
		}
		server.Close()
	}
}

func TestRankedQueriesKeepValuesInVariables(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Query     string         `json:"query"`
			Variables map[string]any `json:"variables"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(payload.Query, "orderBy:[sum_edgeResponseBytes_DESC]") || strings.Contains(payload.Query, "literal\"host") {
			t.Error("ranking or variable isolation failed")
		}
		if payload.Variables["filter"].(map[string]any)["clientRequestHTTPHost"] != "literal\"host" {
			t.Error("literal filter changed")
		}
		_, _ = w.Write([]byte(`{"data":{"viewer":{"zones":[{"rows":[]}]}}}`))
	}))
	defer server.Close()
	c := newClient("fixture", server.URL, server.Client())
	if _, err := c.Rows(context.Background(), "zone", map[string]any{"clientRequestHTTPHost": "literal\"host"}, []string{"clientRequestPath"}, 21, true, true, true); err != nil {
		t.Fatal(err)
	}
}
