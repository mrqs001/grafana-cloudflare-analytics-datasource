# Cloudflare Analytics Datasource for Grafana

Query Cloudflare HTTP/CDN/WAF traffic directly from Grafana. No Prometheus, Mimir,
InfluxDB, or intermediate metric storage is required for Cloudflare data.

A Go backend keeps your token on the server. A typed Grafana query editor provides
zone discovery, metrics, grouping, filters, plan-aware intervals, and variables.
The included dashboards help investigate **Cloudflare edge requests vs Nginx
origin requests**.

**Status:** initial open-source release; unsigned, not yet in the Grafana catalog.
Requires Grafana **13.2.3+**. Apache-2.0 licensed. Independent community project,
not affiliated with Cloudflare or Grafana Labs.

## Try it locally

Install Docker with Compose v2.24.4+ (or Compose v5), then:

```sh
git clone https://github.com/mrqs001/grafana-cloudflare-analytics-datasource.git
cd grafana-cloudflare-analytics-datasource
cp .env.example .env
# Edit .env privately and set CF_API_TOKEN, or export it in your environment.
docker compose up --build -d
```

Open **http://localhost:3300**, sign in with local development credentials
`admin` / `admin`, then open **Dashboards → Cloudflare Analytics**. Change the
password with `GRAFANA_ADMIN_PASSWORD` in `.env`. The service binds only to loopback.

The build uses a multi-stage Dockerfile. `.dockerignore` excludes credentials;
the token is supplied only to the running Grafana process and provisioned into
`secureJsonData.apiToken`. Never commit `.env`, paste credentials into queries,
or run `docker compose config` into a public log (it expands environment values).
No Cloudflare configuration is changed. Local Grafana is disposable; use
`docker compose down` to remove it. Export local dashboard edits before recreating.

## Cloudflare token

Create a restricted **read-only** token for the accounts/zones you want to query:

- **Account → Account Analytics → Read**, scoped to the appropriate account,
  following [Cloudflare's Analytics token guide](https://developers.cloudflare.com/analytics/graphql-api/getting-started/authentication/api-token-auth/).
- **Zone → Zone → Read** for zone names and account discovery.
- Ensure the token's zone resources include each requested zone. Zone analytics
  permissions may also be needed depending on your token type and scope; a health
  check runs a real HTTP analytics query to verify actual access.

No write permission, global API key, account email, or IP access is needed.
Without Zone Read, enter a **Default zone ID** in datasource settings and use IDs
in queries; `zones()` and `accounts()` discovery will be unavailable. The token
must still have access to that zone's Analytics dataset. Both account-owned and
user-owned tokens are supported without relying on a user-token verification API.

## Install in an existing Grafana

1. Build or download a release ZIP. Unzip its single
   `mrqs001-cloudflareanalytics-datasource/` directory into Grafana's plugin directory.
2. For this unsigned community build, explicitly allow
   `mrqs001-cloudflareanalytics-datasource` in
   `GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS` (or the corresponding Grafana setting).
3. Restart Grafana. Add a datasource named **Cloudflare Analytics**, set its API
   token under the password field, optionally enter a default zone ID, and click
   **Save & test**. The saved token is not returned to the browser.
4. Import the dashboards from the datasource's Dashboards tab or `src/dashboards/`.
   Choose your Cloudflare datasource and zone.

Do not enable general development mode in production. Catalog signing is a
separate step; see [publishing](https://github.com/mrqs001/grafana-cloudflare-analytics-datasource/blob/main/docs/publishing.md). Backend binaries are needed
for your Grafana server's OS/architecture, not your browser's platform.

Provisioning example (supply the variable through your secret manager):

```yaml
apiVersion: 1
datasources:
  - name: Cloudflare Analytics
    uid: cloudflare-analytics
    type: mrqs001-cloudflareanalytics-datasource
    access: proxy
    jsonData:
      defaultZoneId: '<optional-zone-id>'
    secureJsonData:
      apiToken: $CF_API_TOKEN
```

![Secure configuration and successful Cloudflare health check](https://raw.githubusercontent.com/mrqs001/grafana-cloudflare-analytics-datasource/main/src/img/configuration.png)

## Query editor

| Metric         | Meaning                                              | Unit       |
| -------------- | ---------------------------------------------------- | ---------- |
| Requests       | Estimated requests in each bucket, or selected range | requests   |
| Request rate   | Requests divided by covered duration                 | requests/s |
| Response bytes | Sum of edge response bytes                           | bytes      |
| Bandwidth      | Edge response bytes divided by covered duration      | bytes/s    |

Group/filter by edge status, origin status, hostname, cache status, country, colo,
HTTP method, request source, URI path, security action, or security source when
available on your plan. Select up to three grouping dimensions. Filters combine
with AND; each supports equals, not equals, in, and not in. Multiple values use
one line per value. Country codes and colo codes must match Cloudflare's values.

New queries default to `requestSource = eyeball`, representing end-user traffic.
Remove that filter to inspect all request sources. The backend respects your
explicit filters and never silently adds a hidden source filter.

Examples:

- **Origin comparison:** Request rate, your zone, 5m interval, source = eyeball.
- **Errors:** Requests, group by edge status, status in 500 / 502 / 503.
- **Cache:** Request rate, group by cache status, hostname = your application.
- **Top paths:** Requests, **Range totals / top values**, group by URI path.
- **Security:** Requests, group by security action and edge status.

For high-cardinality dimensions such as path, prefer range totals. Within a
single supported API range the backend requests a server-ranked top list.
Time series require complete retrieval before selecting top series, so they can
be more expensive. Results warn when lower-ranked series are omitted.

### Dashboard variables

Create a Grafana Query variable using this datasource:

| Variable query               | Result                                                            |
| ---------------------------- | ----------------------------------------------------------------- |
| `zones()`                    | Zone names as labels, IDs as values                               |
| `accounts()`                 | Accounts represented in accessible zones                          |
| `values(hostname, $zone)`    | Hostnames seen in the last complete hour (ending two minutes ago) |
| `values(cacheStatus, $zone)` | Other supported dimension values use the same syntax              |

Use a **single-value** `$zone` in the Zone selector. In filter values, a whole
`$hostname` or `${hostname}` expands to exact values, preserving commas/quotes.
Use **is one of / is not one of** with multi-value variables. For Include All,
leave Grafana's custom All value empty so Grafana expands actual values; wildcard
`*` is treated literally. At most 100 filter values are accepted. Grafana alert
rules cannot evaluate dashboard variables; use literal zone IDs and filters.

## Correctness and limits

Cloudflare's `httpRequestsAdaptiveGroups` returns **already scaled estimates**.
We never multiply counts or bytes by `sampleInterval`. Sampling metadata and
notices appear in Grafana's Query inspector. Different grouping/time selections
may produce different estimates; sparse/missing buckets do not prove zero traffic.

- UTC, epoch-aligned bucket starts. Exact half-open `[from, to)` filtering.
- Requests/bytes are bucket sums; rates divide by the actual covered seconds,
  including partial first/last buckets. The first timestamp is clipped to the
  requested start if necessary. Range-total rates are whole-range averages.
- Auto interval uses panel max data points and Grafana's interval, from 1m, 5m,
  15m, 1h, 6h, and 24h. Coarser output is aggregated from compatible native buckets.
- Settings discovery determines retention, fields, maximum duration and page size.
  Requests beyond retention fail explicitly; the start time is never silently moved.
- Long ranges split into disjoint windows. Full result pages split recursively;
  partial/truncated time series are never presented as complete totals.
- Limits: 2,000 buckets/series, 200 displayed series, 100,000 retrieved rows,
  48 API requests/query, 90-second query deadline, 30-second HTTP timeout.
  A budget failure returns an actionable error instead of partial metrics.
- Up to four in-flight API calls per datasource, at most two bounded retries on
  HTTP 429/5xx. Cloudflare GraphQL errors are handled even with HTTP 200.
- Only metadata is cached (five minutes, per datasource/token). No historical
  metric storage, background scraping, raw GraphQL, or write API is exposed.

Detailed semantics, operational caveats, and Nginx examples:
[query semantics](https://github.com/mrqs001/grafana-cloudflare-analytics-datasource/blob/main/docs/query-semantics.md) · [architecture](https://github.com/mrqs001/grafana-cloudflare-analytics-datasource/blob/main/docs/architecture.md).

## Dashboards

**HTTP overview** covers totals, requests/sec, bandwidth, status, cache, hosts,
colos and paths. **Edge vs Nginx origin** contains a Mixed panel where you add a
query from your existing Nginx datasource, plus cache/security/origin-status views.
No origin metrics or matching origin datasource are fabricated or provisioned.

The default range excludes the most recent two minutes to reduce ingestion lag.
Edge requests include cache hits, challenges, blocks, redirects, and other responses
that may never reach Nginx. A cache miss or origin status 0 is not an exact origin
request counter. Match population, buckets, timestamps and units before comparing.

## Development and validation

Use Node 24 LTS, Go 1.26.5+, Docker, and Mage:

```sh
npm ci
go install github.com/magefile/mage@v1.15.0
npm run typecheck
npm run lint
npm run test:ci
go test -race ./pkg/...
go vet ./pkg/...
npm run build
mage -v build:linux
docker compose -f docker-compose.yaml -f compose.dev.yaml up -d
npm exec playwright install chromium
npm run e2e
# Opt-in live tests, using only your locally supplied read-only token:
python3 scripts/live_test.py
RUN_LIVE_TESTS=1 npm run e2e
```

Rebuild and restart Grafana after backend or `plugin.json` changes. `npm run dev`
watches frontend code. Standard CI requires no real Cloudflare credential; mock
backend tests exercise error/retry/limit paths. Live tests compare Grafana frames
to equivalent direct GraphQL selections, validate rates, grouping, filters,
variables, invalid credentials, and retention. See [validation](https://github.com/mrqs001/grafana-cloudflare-analytics-datasource/blob/main/docs/validation.md)
for what was actually exercised and what remains plan-dependent.

[Contributing](https://github.com/mrqs001/grafana-cloudflare-analytics-datasource/blob/main/CONTRIBUTING.md) · [Security reporting](https://github.com/mrqs001/grafana-cloudflare-analytics-datasource/blob/main/SECURITY.md) ·
[Changelog](https://github.com/mrqs001/grafana-cloudflare-analytics-datasource/blob/main/CHANGELOG.md) · [Release and catalog checklist](https://github.com/mrqs001/grafana-cloudflare-analytics-datasource/blob/main/docs/publishing.md)
