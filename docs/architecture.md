# Architecture

```text
Grafana React query editor / dashboard variables
                    │ typed query / resource request (no stored token)
                    ▼
Grafana backend → Go datasource instance → Cloudflare HTTPS API
                  │                       GET /zones (discovery)
                  │                       POST /graphql (queries only)
                  └─ Grafana DataFrames + notices + query metadata
```

- `src/`: typed query/config editors, safe variable interpolation and backend resources.
- `pkg/cloudflare/`: fixed-origin HTTPS client, read-only endpoints, redacted errors,
  pagination, bounded HTTP retries, five-minute metadata and 30-second analytics
  caches, per-key request deduplication, and a concurrency limit.
- `pkg/plugin/`: query validation, allowlisted dimensions, plan discovery, interval
  selection, bounded complete retrieval, DataFrame conversion, health and resources.
- `provisioning/`: credential references and two runnable dashboards.
- `scripts/live_test.py`: opt-in real API/Grafana cross-checks; no production writes.

Each datasource instance owns its client/cache. Credential updates dispose the
old client, canceling pending cache work and preventing data sharing across tokens. Redirects are not followed.
The backend never logs request bodies or Authorization headers. It rejects unsupported
fields and unresolved variables; filter values go through GraphQL variables.

All HTTP analytics metrics share one dataset planner. A future dataset should add
its own supported fields, limits and aggregation semantics, reuse the client and
frame patterns, and include live/fixture tests. Do not force DNS, Workers, or WAF
raw event datasets into HTTP count semantics. Current security dimensions describe
HTTP traffic; this is not a full firewall-events or bot-analytics implementation.

Alerting works through QueryData. Use literal query fields, no dashboard variables,
and configure an intentional no-data policy for sparse/sampled series. The
backend does not persist metrics or schedule background polling. Cached responses
are bounded to 128 entries and 32 MiB per instance. Health checks bypass the cache.
