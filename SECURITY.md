# Security

Do not include credentials or private account data in public issues. Report a
suspected vulnerability through GitHub's private vulnerability reporting for this
repository. If that is unavailable, open a public issue requesting a private
contact without vulnerability details or sensitive data.

The plugin requires only read access. Tokens belong in Grafana secure settings.
The API origin is fixed to Cloudflare, redirects are refused, queries are
allowlisted and parameterized, and Cloudflare mutation APIs are not exposed.
Grafana administrators remain responsible for datasource access controls and
protecting Grafana's database/encryption key, container environment and backups.

Use supported Grafana releases and keep the host patched: React and Grafana UI
libraries are provided by the Grafana host at runtime. The generated frontend
bundle does not embed the Cloudflare development token.
