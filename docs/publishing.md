# Releases and Grafana catalog publication

The plugin ID is **mrqs001-cloudflareanalytics-datasource**. It follows
`publisher-name-datasource`; it is deliberately distinct from Grafana's official
`grafana-cloudflare-datasource`. `mrqs001` is the current GitHub owner, **not a
verified Grafana Cloud publisher identity**. Before applying for catalog signing,
confirm that you control the matching Grafana Cloud organization slug.

If the slug differs, make a deliberate pre-publication rename: replace the exact
plugin ID in `src/plugin.json`, `pkg/main.go`, Docker/Compose, dashboards,
provisioning, tests and documentation. `git grep mrqs001-cloudflareanalytics-datasource`
shows references. Update `.cprc.json`'s organization metadata for future scaffolding
updates. Generated `.config/` is managed by create-plugin; our Compose files
stand alone and do not use its generated container name. Rebuild/restart Grafana
and re-import dashboards after a rename. Existing datasources reference their
plugin ID, so a post-release rename is a migration, not a cosmetic change.

## Reproducible build and archive

The npm lockfile and Go checksums pin dependencies. Use Node 24, the Go version
from `go.mod`, and Mage 1.15.0. Builds include SDK build timestamp metadata, so
archives are repeatable from pinned source but are not byte-identical by default.

```sh
npm ci
npm run typecheck && npm run lint && npm run test:ci
go test -race ./pkg/...
go vet ./pkg/...
npm run build
mage -v buildAll
python3 scripts/package.py
```

The ZIP has exactly one root directory named after the plugin ID, containing
`plugin.json`, frontend assets, docs, license and executable backend binaries.
SHA-256 checksums accompany the ZIP. `mage build:linux` only builds the native
Linux architecture; use `buildAll` for published multi-platform archives.

Run the official validator on the archive:

```sh
npx --yes @grafana/plugin-validator@latest -jsonOutput \
  artifacts/mrqs001-cloudflareanalytics-datasource-0.1.0.zip
```

Local unsigned builds can report signature/catalog-related checks. Those are
publication requirements, not a claim that a build has been accepted by Grafana.
CI additionally validates metadata and archives build outputs.

## Signing and release

1. Check tests, live validation, dependency advisories and compatibility. Never
   put the development Cloudflare token in CI secrets or release artifacts.
2. Set the version in `package.json`/lockfile and update the changelog.
3. Follow [Grafana signing](https://grafana.com/developers/plugin-tools/publish-a-plugin/sign-a-plugin)
   and [publication](https://grafana.com/developers/plugin-tools/publish-a-plugin) instructions.
   Apply for the appropriate community signature; private signatures are not
   equivalent to public catalog approval. The public plugin ID must match your
   approved publisher slug.
4. After approval, add `GRAFANA_ACCESS_POLICY_TOKEN` as a GitHub Actions secret.
   This is a Grafana signing credential, **never the Cloudflare API token**.
5. Push a version tag (`v0.1.0` for this version). The release workflow builds,
   optionally signs, packages, and creates a **draft** GitHub release. Inspect it
   before publishing. Without a signing token it is explicitly unsigned.
6. Submit the signed archive and checksum to Grafana's catalog process. Confirm
   plugin validation, source availability, docs, license, logo and compatibility.

Metadata currently requires Grafana 13.2.3+ (React 19 APIs). The Docker environment
and browser tests validate that minimum. The separate compatibility workflow
checks current Grafana API compatibility for pull requests. A package archive is
not evidence of catalog approval or a license to use vendor trademarks.
