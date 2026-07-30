# Changelog

## v0.1.0 - 2026-07-30
### What's Changed
- Initial release: composes a GitHub profile README from styled SVG sections — banner, positioning, what-I-do, technology (grouped), and open-source contributions.
- Web UI with per-section configuration, live preview, manual refresh/publish and a built-in scheduler.
- Contributions collector over the GitHub GraphQL API: external repositories with commit/PR/issue counts and per-item links.
- Publishes via pull request or direct commit; idempotent writes.
- Storage on SQLite (default) or PostgreSQL.
- Optional web UI authentication: HTTP Basic or OIDC (Keycloak).
