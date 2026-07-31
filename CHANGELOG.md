# Changelog

## v0.1.3 - 2026-08-01
### What's Changed
#### 🔒 Security
- Bump `golang.org/x/text` to v0.39.0 (fixes CVE-2026-56852).
- Set `HttpOnly`, `Secure` and `SameSite` on the session-logout cookie.
- Add explicit least-scope `permissions` blocks to the PR and Release workflows.

## v0.1.2 - 2026-07-31
### What's Changed
- Restructure the README: clearer quick-start, configuration, publishing and storage guides, plus a Kubernetes deployment example.
- Bump `modernc.org/sqlite` to v1.55.0 and `github.com/coreos/go-oidc/v3` to v3.20.0.

## v0.1.1 - 2026-07-31
### What's Changed
- Add screenshots to the README: the composed profile and the configuration UI.

## v0.1.0 - 2026-07-31
### What's Changed
- Initial release: composes a GitHub profile README from styled SVG sections — banner, positioning, what-I-do, technology (grouped), and open-source contributions.
- Web UI with per-section configuration, live preview, manual refresh/publish and a built-in scheduler.
- Contributions collector over the GitHub GraphQL API: external repositories with commit/PR/issue counts and per-item links.
- Publishes via pull request or direct commit; idempotent writes.
- Storage on SQLite (default) or PostgreSQL.
- Optional web UI authentication: HTTP Basic or OIDC (Keycloak).
