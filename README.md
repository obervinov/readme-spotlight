# readme-spotlight

[![PR](https://github.com/obervinov/readme-spotlight/actions/workflows/pr.yaml/badge.svg?branch=main&event=pull_request)](https://github.com/obervinov/readme-spotlight/actions/workflows/pr.yaml)
[![Release](https://github.com/obervinov/readme-spotlight/actions/workflows/release.yaml/badge.svg)](https://github.com/obervinov/readme-spotlight/actions/workflows/release.yaml)

![GitHub release (latest SemVer)](https://img.shields.io/github/v/release/obervinov/readme-spotlight?style=for-the-badge)
![GitHub last commit](https://img.shields.io/github/last-commit/obervinov/readme-spotlight?style=for-the-badge)
![GitHub Release Date](https://img.shields.io/github/release-date/obervinov/readme-spotlight?style=for-the-badge)
![GitHub issues](https://img.shields.io/github/issues/obervinov/readme-spotlight?style=for-the-badge)
![GitHub repo size](https://img.shields.io/github/repo-size/obervinov/readme-spotlight?style=for-the-badge)
[![Go version](https://img.shields.io/badge/go-1.25-blue.svg?style=for-the-badge)](https://go.dev/dl/)
[![License](https://img.shields.io/badge/license-MIT-green.svg?style=for-the-badge)](https://opensource.org/licenses/MIT)

Self-hosted tool that composes a GitHub profile README from styled SVG sections
and keeps it up to date on a schedule.

GitHub sanitizes HTML in READMEs — no CSS, no classes, no inline styles — so
hand-built profiles tend to look plain or inconsistent. `readme-spotlight`
sidesteps that by rendering each section as an **SVG image** (which can carry
real colours, type and layout) and writing them into a single managed region of
your README. The result is a cohesive, good-looking profile that refreshes
itself.

It started as an open-source contributions widget and grew into a small profile
builder. The contributions section still does the thing GitHub's own profile
can't: it surfaces every external repository you have contributed to — including
personal-account repos and buried org work — with per-item links.

## Screenshots

The composed profile it publishes, and the configuration UI:

| Composed profile | Configuration UI |
|:---:|:---:|
| [![Composed profile](docs/screenshots/profile.png)](docs/screenshots/profile.png) | [![Configuration UI](docs/screenshots/webui.png)](docs/screenshots/webui.png) |

## Sections

Rendered, in order, into the region between `<!--SPOTLIGHT:START-->` and
`<!--SPOTLIGHT:END-->`:

1. **Banner** — name, role and a short hook.
2. **Positioning** — a one-line specialisation statement.
3. **What I Do** — focus areas as a titled card.
4. **Technology & Tools** — your stack, grouped by domain.
5. **Open-Source Contributions** — external repos with commit/PR/issue counts,
   as a styled SVG card, an expandable linked list, or both (hybrid).

Each section is configurable and can be toggled off. All state is stored in the
database, so nothing has to be reconfigured between runs.

## Running

```sh
export GITHUB_TOKEN=<PAT with public_repo, read:org, read:user>
go run ./cmd/readme-spotlight            # web UI + scheduler on :8080
```

Open <http://localhost:8080>, set the target repository, **Refresh** to collect
data, preview the sections, then **Publish**. The GitHub token is read from the
environment and never stored.

There is also a one-shot mode for the contributions block:

```sh
go run ./cmd/readme-spotlight --print --format hybrid
```

### Docker

```sh
docker run -p 8080:8080 -e GITHUB_TOKEN=... -v rs-data:/data \
  ghcr.io/obervinov/readme-spotlight:latest
```

## Publishing

- **Pull request** (default) — commits to a head branch and opens a PR, so the
  profile's default branch only changes when you merge.
- **Direct commit** — commits straight to the target branch.

Both are idempotent: files that have not changed are not rewritten. A built-in
cron refreshes and republishes on a configurable schedule; **Refresh** and
**Publish** can also be triggered manually from the UI.

## Authentication

Off by default (the UI is open). Set `RS_AUTH_MODE` to guard it when exposed:

- `basic` — HTTP Basic auth against `RS_BASIC_USER` / `RS_BASIC_PASSWORD`.
- `oidc` — OpenID Connect login (e.g. Keycloak). Requires `RS_OIDC_ISSUER_URL`,
  `RS_OIDC_CLIENT_ID`, `RS_OIDC_CLIENT_SECRET`, `RS_OIDC_REDIRECT_URL`
  (`https://<host>/auth/callback`) and `RS_SESSION_SECRET`.

`/healthz` and the OIDC login routes stay public; everything else is guarded.

## Storage

SQLite by default (pure-Go, no CGO). Point `--db` at PostgreSQL to use that
instead:

```sh
go run ./cmd/readme-spotlight --db 'postgres://user:pass@host:5432/spotlight'
```

## License

MIT
