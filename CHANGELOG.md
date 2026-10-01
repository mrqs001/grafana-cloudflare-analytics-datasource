# Changelog

## 0.3.0 (2026-10-01)

- Add status range filters and URI path patterns; accept literal dollar signs in paths.
- Omit zone columns from single-zone totals, retaining zone metadata in the inspector.
- Cache identical analytics requests for 30 seconds with bounded memory; coalesce
  concurrent lookups per key without holding locks during network calls.
- Keep health checks uncached and derive the User-Agent version from build metadata.

## 0.2.1 (2026-10-01)

- Keep query controls and filters on fewer rows in narrow Explore and panel editors.

## 0.2.0 (2026-10-01)

- Single, multiple and all-zone defaults/queries, zone-labeled graphs, and multi-value zone variables.
- Compact query editor with expandable options and domain-only zone dropdowns.

- Versioned build for the multi-zone editor; restart Grafana and hard-refresh after upgrading.

## 0.1.0

- Go backend for Cloudflare HTTP adaptive analytics, real health checks and discovery.
- Requests, request rate, response bytes and bandwidth; eleven dimensions/filters.
- UTC buckets, partial-bucket rates, plan-aware intervals, sampling notices and
  bounded complete retrieval; efficient server-ranked range totals.
- Secure token configuration, exact-value template variables and dashboard examples.
- Disposable Docker development, tests, packaging and CI.
- Initial build is unsigned; Grafana catalog publication requires publisher setup.
