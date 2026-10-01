# Query semantics and edge/origin comparison

The HTTP dataset is `httpRequestsAdaptiveGroups`, the current successor to the
legacy colo-group datasets. Cloudflare supplies estimates of `count` and
`sum(edgeResponseBytes)`. `avg(sampleInterval)` describes sampling; it is not a
multiplier. The maximum returned group average is exposed as `maxSampleInterval`.
It is neither a statistical error bound nor the raw maximum sampling factor.
No sampling metadata means unknown sampling, not an exact census.

Sources: [migration](https://developers.cloudflare.com/analytics/graphql-api/migration-guides/graphql-api-analytics/),
[sampling](https://developers.cloudflare.com/analytics/graphql-api/sampling/),
[settings](https://developers.cloudflare.com/analytics/graphql-api/features/discovery/settings/),
[limits](https://developers.cloudflare.com/analytics/graphql-api/limits/).

## Time

Every query filters the exact UTC range with `datetime_geq` and `datetime_lt`.
Buckets align to the Unix epoch. API native dimensions are minute, five minutes,
fifteen minutes, hour and date. Six-hour output sums hour groups. When a native
field is unavailable, the planner chooses a finer supported divisor.

For a request from 12:00:30 to 12:01:30, one-minute buckets each cover 30 seconds.
If their counts are 60 and 30, the rates are 2 and 1 request/s. Timestamps are
12:00:30 and 12:01:00. Complete buckets retain their UTC starts. We never widen the
query range to make buckets look complete. Daily buckets remain UTC even across
local daylight-saving changes.

Missing buckets default to null. Zero fill is explicit and only fills missing
buckets within observed series. Entirely empty results remain empty. Sampling,
late data, and absent events cannot always be distinguished. A narrow query and
a wide query can have different estimates because Cloudflare adapts sampling.

## Completeness and top values

For ordinary time series, a response exactly at the API row limit is considered
potentially truncated. Its rows are discarded and the time range is split into
two nonoverlapping parts. Split counts/bytes are summed into output buckets.
This repeats down to a second, bounded by row/request/time budgets. If a complete
result cannot be obtained, the entire query fails. Other refIds remain independent.

Top series are selected by their whole-range sum, consistently across all buckets.
For range totals within `maxDuration`, Cloudflare can rank globally before applying
the limit. The backend asks for top N + 1 to detect omission; the reported series
count is a lower bound when that response is full. Range totals spanning multiple
API windows use full retrieval instead: merging local top-N lists would lose
potentially important global groups. Frame metadata identifies server ranking.

Do not sum a top-series panel to obtain a zone-wide total. Use an ungrouped
Requests query with Range totals. Totals and differently grouped queries can
still differ within sampling uncertainty.

## Multiple zones

Each query resolves datasource defaults, selected IDs, or all discovered zones.
Discovery names do not include account details; manual IDs fall back to ID labels
when discovery is unavailable. All zones is dynamic and capped at 20 zones. Defaults
are a convenience, not a permission boundary. Legacy single-zone queries still work.

The planner checks every zone's retention and fields before fetching data. All zones
use the same output interval and exact time range. It never sums different zones
implicitly: time series carry zone and zoneId labels, and range-total rows include
zone columns only for multi-zone queries. Single-zone totals contain just the
requested dimensions and metric. Top N applies independently per zone. Merged table metadata contains
per-zone sampling/ranking statistics plus shared request/row totals.

The 48-request, 100,000-row and 90-second budgets are shared across all zones in one
query; output is capped at 400,000 points. A failed zone or exhausted budget returns
an error for the whole refId, with no partial frames. Other refIds remain independent.

## Filters and caching

Status ranges use `gt`, `geq`, `lt`, or `leq`. For 5xx, combine status `geq 500`
and `lt 600`; use `geq 400` for 4xx and 5xx together. URI path `like` and `notLike`
use Cloudflare's `%` wildcard (for example `/api/%`). Clauses combine with AND.
Values remain GraphQL variables. Literal dollar signs in paths are accepted;
whole unresolved variable references such as `$hostname` still fail validation.

Identical upstream analytics requests share a 30-second cache. Keys include zone,
exact time bounds, filters, dimensions, metric selection, ranking, and row limit.
No time rounding is introduced; moving relative ranges can miss the cache. Cache
hits and shared requests appear as `cacheHits` / `totalCacheHits` in query metadata.
`apiRequests` / `totalAPIRequests` exclude them (and exclude metadata calls/retries).
The 48-request planning budget still counts cache lookups to bound query work.
Errors are not cached. Canceling one waiter does not cancel other waiters; when
all waiters leave, pending upstream work is canceled. Health checks bypass caches.

## Nginx

Start with end-user traffic (`requestSource=eyeball`), identical hosts and methods,
then choose whether to include subrequests. Compare completed, matching UTC
buckets. Edge analytics and origin logs can have different delivery delays.

If Nginx is already in Prometheus, a starting overlay is:

```promql
sum(rate(nginx_http_requests_total[$__rate_interval]))
```

Exporter names and labels differ. Prometheus `rate` estimates a rolling counter
rate and handles resets; it is not the same estimator as a nonoverlapping edge
bucket average. `increase` extrapolates and may produce non-integer counts. Do not
claim exact equality from that overlay.

For Nginx access logs already in Loki:

```logql
sum(count_over_time({job="nginx"}[$__interval])) / $__interval_s
```

Filter by the relevant service/host. Loki windows end at the sample timestamp;
Cloudflare output is labeled at the bucket start. For exact reconciliation,
rebucket raw origin logs with the same `[start,end)` boundaries and UTC timestamps.
The comparison dashboard explains these choices instead of assuming an origin
backend, metric name, or label scheme.

Cache hits, WAF blocks/challenges, Cloudflare redirects, Workers responses and
edge errors can avoid Nginx entirely. Retries, direct origin access, Workers
subrequests, load balancing, and multiple origin hops can produce extra origin
requests. Tunnel changes connectivity, not those request definitions. Inspect
cache status, origin status, edge status, security action/source and request source
together. Origin status 0 does not uniquely identify a WAF block.
