# Changelog

## v0.2.0 - 2026-08-03
### What's Changed
- Add a machine API for keeping the section content in sync with an external source (e.g. a CV) without using the UI: `GET`/`PATCH /api/content` and `POST /api/publish`. Disabled unless `RS_API_TOKEN` is set.
- Add a favicon, served publicly so it also shows on the OIDC login redirect.
#### 🔒 Security
- The machine API can write section content only — target repository, branch, README path, markers, publish mode, PR branch and schedule stay UI-only, and an out-of-reach field is rejected rather than ignored.
- `POST /api/publish` always publishes through a pull request, even when the stored publish mode is `commit`.
- Validate accent colours as hex before they are interpolated into SVG, cap content lengths and reject control characters; request bodies are capped at 64 KiB.
- Bearer tokens are compared in constant time over a SHA-256 digest, accepted from the `Authorization` header only, and must be at least 32 characters or startup fails.
- Rate-limit the API globally, with separate budgets for authorised (60/min) and rejected (10/min) requests so an anonymous flood cannot starve the real caller, and lock the API for five minutes after ten consecutive authentication failures.

## v0.1.3 - 2026-07-31
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
