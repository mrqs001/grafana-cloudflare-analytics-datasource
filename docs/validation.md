# Validation evidence

Validation performed on 2026-10-01 against a real restricted read-only Cloudflare
token, two accessible zones, and disposable local Grafana. Private zone names,
IDs, token values, raw responses and screenshots stay in ignored `.local/`.

## Live API and Grafana

`python3 scripts/live_test.py` passed:

- Real Grafana datasource health query; zone/settings/account metadata discovery.
- 15m, 1h, 6h, 24h, 3d, 7d, 30d and 30.5d count, rate and total queries. Equivalent
  direct API bucket selections had 0% aggregate count difference in the recorded
  run. Integrated rate values agreed with counts. This is evidence for those
  queries, not a guarantee that sampled selections always match. The 30.5-day
  case exercised live splitting across the 30-day per-request limit.
- Status, hostname, cache, country, colo, method, path totals, request source,
  origin status, security action and security source grouping.
- Bytes/bandwidth; in/not-in/not-equals filters; genuinely empty hostname results.
- Grafana server-side reduce expressions, supporting the alerting data path.
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

## 0.3.1 maintenance validation (2026-10-30)

- Clean npm install, TypeScript, ESLint, six frontend tests, 25 Go race tests and vet pass.
- Eight credential-free browser checks pass on each of Grafana 13.2.3 and 13.3.0;
  the three opt-in live specs are skipped. This update does not claim a new live API run.
- All six backend targets build, the Docker build passes, and `govulncheck` reports
  no vulnerabilities in the Go packages.
- The full official validator completes 40 analyzers without failure, reports two
  existing warnings (unsigned plugin and unregistered publisher), and one organization
  scan error because `mrqs001` is not a registered Grafana Cloud publisher.
  Catalog signing remains a separate prerequisite; metadata checks still pass.

## Dependency audit

The 2026-10-30 dependency refresh uses Grafana 13.3.0/React 19.3.1. The npm audit
reports no high or critical findings. Remaining findings are moderate React Router
advisories in Grafana UI's compatibility dependencies and a low-severity deepmerge
advisory in build/test tools. No patched deepmerge release is available. The plugin does not bundle React Router
or perform SSR hydration; Grafana provides UI/runtime dependencies. Do not force
a router-major override that breaks Grafana's peer API. Keep the host patched and
review `npm audit` before releases. Current audit findings are not silently waived.

Compatibility constraints retained during the update:

- ESLint 9: `eslint-plugin-react@7.37.5` does not support ESLint 10.
- Grafana lint-config 9: the managed scaffold imports `@grafana/eslint-config/flat.js`,
  which v10 no longer exports. The protected `.config` directory is unchanged.
- TypeScript 6.0.3: the existing ts-node/type-checker tooling uses the JavaScript
  compiler API; TypeScript 7's native distribution is not a drop-in replacement.
- `@types/node` follows the Node 24 build runtime; prerelease integrity-plugin
  versions are not used in production builds.

CI exercises both the minimum Grafana 13.2.3 and current Grafana 13.3.0. The default
Docker development image uses 13.3.0. For an existing 13.2.3 instance, installing this
plugin update does not require a Grafana upgrade.

## Re-run

Use the README commands. Live tests are optional and consume API quota. The local
report prints pass/fail summaries without credentials or zone identities. Archive
validation, CI status, and browser tests must be rerun for release candidates.

## Packaging and catalog checks

A clean multi-stage Docker build succeeded. The release archive includes backend
binaries for Linux amd64/arm64/arm, Windows amd64 and macOS amd64/arm64. Typecheck,
ESLint, frontend tests, Go race tests and vet passed. Browser checks confirmed
configuration, query controls and real dashboard responses. All seven browser checks passed
against the final clean Docker image. Secret scans covered the Git index, entire
committed history, built assets, image configuration and build history.

The official full plugin validator identified README relative links and a generic
license placeholder; these were corrected, and a secret-free configuration
screenshot was added. Remaining catalog prerequisites are an approved Grafana
Cloud publisher organization and plugin signing. The validator reported the
`mrqs001` organization as unregistered. This repository does not claim catalog
approval. Metadata validation is run separately in CI.
