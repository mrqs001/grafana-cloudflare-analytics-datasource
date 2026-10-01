# Reproduce the dashboard previews

The README screenshots come from an isolated Grafana instance using its built-in
TestData datasource. All traffic is generated deterministically; the generator
does not read credentials, environment variables, or live analytics. Domains are
fictional `shop.example.com` and `api.example.com` names.

With Python 3 and Docker Compose installed, run from the repository root:

```sh
python3 scripts/generate_demo_dashboards.py
docker compose -f compose.demo.yaml up -d --wait
```

Open these dashboards, with no sign-in or Cloudflare token required:

- [HTTP overview](http://localhost:3301/d/demo-overview)
- [Simulated Nginx comparison](http://localhost:3301/d/demo-origin-comparison)

This loopback-only demo runs on port 3301, independently of the normal development
Grafana on port 3300. It has no Cloudflare datasource or token. The generated
provisioning files stay under ignored `.local/demo/`.

The fixed six-hour period shows a modeled lunchtime traffic peak. Each point is
a two-minute bucket. Nginx request rate is simulated at a fraction of edge traffic
to illustrate caching and origin comparisons; it is not a measurement or a claim
about typical cache efficiency. The zone selector is decorative in these static
fixtures and does not filter TestData CSV queries.

The screenshots illustrate the layouts in `src/dashboards/`, but replace their
queries with static TestData fixtures. For actual use, import those original JSON
dashboards and add your own Prometheus or Loki query for Nginx. The Cloudflare
plugin itself supplies only Cloudflare analytics.

Capture the rendered dashboard with the demo title and fictional series labels
visible. Keep a synthetic-data caption alongside any published image. Do not use
screenshots from a live account or copy private browser captures into `src/img/`.

Remove the disposable demo when finished:

```sh
docker compose -f compose.demo.yaml down
```
