# Contributing

Open an issue describing the problem and a reproducible query with secrets, zone
IDs, domains and private paths removed. Never attach `.env`, Grafana databases,
raw production traces, or Authorization headers.

Use Node 24 and Go 1.26.5+. Follow the development commands in README. Changes to
query semantics need deterministic tests for values, time boundaries, sampling,
limits and errors. UI changes should have a focused `@grafana/plugin-e2e` test.
Run formatting, typecheck, lint, unit tests, Go race tests/vet, and a production
build. Keep generated `.config/` files unchanged; extend the root configs instead.

Live integration tests are opt-in and require your own restricted read-only token.
They may consume Cloudflare query quota. Do not run them on every keystroke or
increase permissions to make a test pass. Never alter Cloudflare configuration.

Pull requests should explain the user-visible behavior, validation, and any plan
limitations. New datasets must document request definitions and sampling semantics.
By contributing, you agree your contribution is under Apache-2.0.
