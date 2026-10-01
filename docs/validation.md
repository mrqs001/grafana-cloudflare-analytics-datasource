# Validation evidence

Validation performed on 2026-10-01 against a real restricted read-only Cloudflare
token, two accessible zones, and disposable local Grafana. Private zone names,
IDs, token values, raw responses and screenshots stay in ignored `.local/`.

## Live API and Grafana

`python3 scripts/live_test.py` passed:

- Real Grafana datasource health query; zone/settings/account metadata discovery.
- 15m, 1h, 6h, 24h, 3d, 7d and 30d count, rate and total queries. Equivalent
  direct API bucket selections had 0% aggregate count difference in the recorded
  run. Integrated rate values agreed with counts. This is evidence for those
  queries, not a guarantee that sampled selections always match.
- Status, hostname, cache, country, colo, method, path totals, request source,
  origin status, security action and security source grouping.
- Bytes/bandwidth; in/not-in/not-equals filters; genuinely empty hostname results.
- Multiple refIds; invalid and unknown zones; invalid status filters; retention
  rejection; a real invalid-token datasource health check; variable lookup.
- Grafana's public datasource response contained no secure token value.
- Real adaptive sampling was observed and surfaced in metadata.

The tested zone exposed 31-day retention, 30-day maximum duration and 10,000 rows
per page. Other plans are discovered dynamically. Live path time series on the
busier zone were too expensive; range totals were optimized using Cloudflare's
global ranking and then passed. High-cardinality time series remain bounded and
can return an explicit limit/deadline error; choose totals or narrower filters.

## Deterministic checks

Go unit/race tests cover retries, metadata pagination/cache, token redaction,
HTTP authentication/rate/server errors, GraphQL errors under HTTP 200, malformed
responses, cancellation, query validation, UTC interval selection, retention,
partial rates, already-scaled sampling, split-page reconstruction, chunk boundaries,
no partial success, top-series selection, null buckets and empty frames.
Frontend tests cover exact-value multi-variable expansion and unresolved variables.
Browser tests use `@grafana/plugin-e2e` for configuration, missing credentials,
query editor controls, secure configured state and a live dashboard/zone variable.

Insufficient permission responses, upstream 429/5xx, validation errors, saturated
pages and stricter duration limits are deterministic fixtures. The provided valid
read-only token was not altered to manufacture missing permissions or expiry;
invalid-token behavior was tested live. No production Cloudflare writes occurred.
No real Nginx datasource was supplied, so the Mixed panel is a documented template;
end-to-end origin reconciliation remains a deployment-specific validation.

## Dependency audit

The scaffold's high-severity frontend dependency findings were removed by moving
to Grafana 13.2.3/React 19. npm still reports moderate React Router advisories in
Grafana UI's compatibility dependencies. The plugin does not bundle React Router
or perform SSR hydration; Grafana provides UI/runtime dependencies. Do not force
a router-major override that breaks Grafana's peer API. Keep the host patched and
review `npm audit` before releases. Current audit findings are not silently waived.

## Re-run

Use the README commands. Live tests are optional and consume API quota. The local
report prints pass/fail summaries without credentials or zone identities. Archive
validation, CI status, and browser tests must be rerun for release candidates.

## Packaging and catalog checks

A clean multi-stage Docker build succeeded. The release archive includes backend
binaries for Linux amd64/arm64/arm, Windows amd64 and macOS amd64/arm64. Typecheck,
ESLint, frontend tests, Go race tests and vet passed. Browser checks confirmed
configuration, query controls and real dashboard responses.

The official full plugin validator identified README relative links and a generic
license placeholder; these were corrected, and a secret-free configuration
screenshot was added. Remaining catalog prerequisites are an approved Grafana
Cloud publisher organization and plugin signing. The validator reported the
`mrqs001` organization as unregistered. This repository does not claim catalog
approval. Metadata validation is run separately in CI.
