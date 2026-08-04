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

## Quick start

You need a GitHub token (a classic PAT with `public_repo`, `read:org` and
`read:user`, or a fine-grained token with equivalent access). It is read from
the environment and never written to the database.

```sh
export GITHUB_TOKEN=<your token>
go run ./cmd/readme-spotlight          # web UI + scheduler on http://localhost:8080
```

Then, in the UI:

1. Set the **target repository** (your profile repo, e.g. `you/you`).
2. **Refresh** — collects your contributions (nothing is published yet).
3. Review the live **preview** of each section.
4. **Publish** — writes the sections into the target README.

Prefer just the contributions block, printed to stdout? Skip the UI:

```sh
go run ./cmd/readme-spotlight --print --format hybrid
```

## Configuration

Everything is edited in the web UI and persisted in the database: which sections
are shown, their content and accent colours, the contributions format
(`table` / `details` / `svg` / `hybrid`), sorting, the target repository and
markers, the publish mode, and the refresh schedule.

The section content can also be updated by a script or an agent over the
[machine API](#machine-api).

## Publishing

- **Pull request** (default) — commits to a head branch and opens a PR, so the
  profile's default branch only changes when you merge it.
- **Direct commit** — commits straight to the target branch.

Both are idempotent — files that have not changed are not rewritten. A built-in
cron refreshes and republishes on the configured schedule; **Refresh** and
**Publish** can also be triggered manually from the UI.

## Authentication

The UI is open by default. Set `RS_AUTH_MODE` to guard it when it is exposed:

| Mode | Variables |
|------|-----------|
| `basic` | `RS_BASIC_USER`, `RS_BASIC_PASSWORD` |
| `oidc` | `RS_OIDC_ISSUER_URL`, `RS_OIDC_CLIENT_ID`, `RS_OIDC_CLIENT_SECRET`, `RS_OIDC_REDIRECT_URL` (`https://<host>/auth/callback`), `RS_SESSION_SECRET` |

`/healthz`, `/favicon.svg` and the OIDC login routes stay public; everything else
is guarded.

## Machine API

For keeping the wording in sync with an external source — a CV, a bio, whatever
you already maintain elsewhere — without clicking through the UI. Disabled unless
`RS_API_TOKEN` is set; the prefix returns `404` otherwise.

```sh
export RS_API_TOKEN=$(openssl rand -hex 32)   # 32 characters minimum, or startup fails
```

| Endpoint | Purpose |
|----------|---------|
| `GET /api/content` | read the editable content |
| `PATCH /api/content` | replace some content fields; omitted fields keep their value |
| `POST /api/publish` | publish now, **always as a pull request** |

```sh
curl -sf -X PATCH https://<host>/api/content \
  -H "Authorization: Bearer $RS_API_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"positioning":{"enabled":true,"text":"Platform & LLMOps engineer","accent":"#3fb950"}}'

curl -sf -X POST https://<host>/api/publish -H "Authorization: Bearer $RS_API_TOKEN"
```

The API is a second front door to a service that holds a repository-write GitHub
token, so its reach is capped by design:

- **Content only.** `banner`, `positioning`, `focus`, `tech`, `title`, `format`,
  `sort_by` and `limit` are writable. The fields deciding *where* and *how* the
  service writes — target repository and branch, README path, markers, publish
  mode, PR branch, schedule — are reachable only from the authenticated UI. An
  unknown or out-of-reach field is rejected with `400`, never ignored.
- **Pull requests only.** `POST /api/publish` opens a PR even when the stored
  publish mode is `commit`, so an automated caller cannot land an unreviewed
  commit. The stored mode is left untouched.
- **Validated content.** Accents must be hex (they are interpolated into SVG),
  text is length-capped and control characters are refused; bodies are capped at
  64 KiB.
- **Rate-limited.** Globally rather than per-IP, since a client-supplied
  `X-Forwarded-For` cannot be trusted for budgeting. Authorised requests get
  60/minute (burst 20); rejected ones get their own 10/minute (burst 5), so an
  anonymous flood cannot starve the real caller. Ten consecutive authentication
  failures lock the API for five minutes.
- **Audited.** Every accepted write is logged with the fields it changed, and the
  log is visible in the UI.

Keep the network layer regardless: expose `/api` only to the networks that need
it, and treat the token like the GitHub token it fronts.

## Storage

SQLite by default (pure-Go, no CGO) at `./data/spotlight.db`, or `/data/spotlight.db`
in the container. Point `--db` (or `RS_DATABASE_DSN`) at PostgreSQL to use that
instead:

```sh
go run ./cmd/readme-spotlight --db 'postgres://user:pass@host:5432/readme_spotlight?sslmode=disable'
```

Every `RS_*` variable supplies the **default** for its flag, so a flag on the
command line wins. Configure containers through the environment and leave the
command alone — a `--db` in the command line would quietly ignore
`RS_DATABASE_DSN` and keep writing to the container's own filesystem, which is
gone with the next container.

## Deployment

### Docker

```sh
docker run -p 8080:8080 \
  -e GITHUB_TOKEN=<your token> \
  -v rs-data:/data \
  ghcr.io/obervinov/readme-spotlight:latest
```

Images are multi-arch (`linux/amd64`, `linux/arm64`) and published to GHCR on
every release.

### Kubernetes

Put everything sensitive in a `Secret`, then apply a `Deployment` + `Service`.
This example uses PostgreSQL, so no volume is needed:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: readme-spotlight
type: Opaque
stringData:
  GITHUB_TOKEN: <your token>
  # The DSN carries the database password, so it belongs here rather than in the
  # Deployment: env values are readable in any dump of the object.
  RS_DATABASE_DSN: postgres://user:pass@postgres:5432/readme_spotlight?sslmode=disable
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: readme-spotlight
spec:
  replicas: 1
  selector:
    matchLabels: { app: readme-spotlight }
  template:
    metadata:
      labels: { app: readme-spotlight }
    spec:
      containers:
        - name: readme-spotlight
          image: ghcr.io/obervinov/readme-spotlight:latest
          ports:
            - containerPort: 8080
          env:
            - name: GITHUB_TOKEN
              valueFrom:
                secretKeyRef: { name: readme-spotlight, key: GITHUB_TOKEN }
            - name: RS_DATABASE_DSN
              valueFrom:
                secretKeyRef: { name: readme-spotlight, key: RS_DATABASE_DSN }
            # Guard the UI when exposing it (see Authentication). RS_SESSION_SECRET
            # and RS_OIDC_CLIENT_SECRET belong in the Secret above, alongside
            # RS_API_TOKEN if the machine API is enabled:
            # - name: RS_AUTH_MODE
            #   value: oidc
          readinessProbe:
            httpGet: { path: /healthz, port: 8080 }
          livenessProbe:
            httpGet: { path: /healthz, port: 8080 }
---
apiVersion: v1
kind: Service
metadata:
  name: readme-spotlight
spec:
  selector: { app: readme-spotlight }
  ports:
    - port: 80
      targetPort: 8080
```

To use SQLite instead of PostgreSQL, drop `RS_DATABASE_DSN` and mount a
`PersistentVolumeClaim` at `/data`.

## License

MIT
