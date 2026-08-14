# Changelog

## v0.3.0 - 2026-08-14
### What's Changed
- Write one commit per run instead of one per file. Publishing went through the contents API, which commits each path separately, so every run left two commits in the target repository — one for the README, one for the contributions asset. Fifty of the last fifty-four commits in the profile repository were these refreshes, twice a day on several days, which makes a contribution graph read as a scheduler rather than as work. Commits now go through the git data API as a single tree, commit and ref update, with force disabled so a branch that moved underneath the run fails rather than being clobbered.
- Make a run that changes nothing write nothing. Previously an unchanged section still produced commits. The output is compared against the branch before any write, and the commit path checks again by tree SHA — trees are content-addressed, so an identical set of files resolves to the branch's own tree.
- Add `skip_star_only_changes` (default off) for the remaining source of churn: star counts on other people's repositories move on their own, so a plain comparison still commits when only a number changed. Suppressing that means published counts fall behind, which is why it is opt-in rather than the default. It recognises the star counts a format marks with `★`, so it has no effect on the plain table format, whose star column is a bare number.
- Add `group_merged` (default off) to render contributions as merged code first and reported issues second, each sorted by stars. Sorting a single list by total activity put a 41-star repository second and a 23.9k-star one thirteenth; sorting it by stars instead would bury actual merged work under repositories that only ever received a bug report. Nothing is dropped or hidden — both groups render in full, and a rejected or long-open pull request still appears, just not among the merged work.
- Collect `PRsMerged` separately, because the existing pull-request count includes every state. Without it a rejected pull request and one open for two years are indistinguishable from a merged feature, which is precisely the distinction the grouping depends on. Existing snapshots carry no merged count, so a refresh is needed before enabling grouping — until then repositories qualify on commits alone.
- Change the default schedule to weekly. Contributions change on the order of weeks, and a daily run only produces the no-op commits above. A running instance keeps its schedule in the database and is unaffected.
- Stop attempting to open a pull request in PR mode when nothing was committed and no pull request exists, which the API rejected with a 422.

## v0.2.4 - 2026-08-04
### What's Changed
- Keep the database DSN out of the Deployment in the Kubernetes example. It carries the database password, and an env value is readable in any dump of the object — the same inconsistency the example already avoided for `GITHUB_TOKEN`. It moves into the Secret, and the note about guarding the UI now says where the session and OIDC client secrets belong too.

## v0.2.3 - 2026-08-03
### What's Changed
- Describe the skipped contributions accurately. v0.2.2 blamed a fine-grained PAT's repository scope, which was wrong — the token in question is a classic PAT. Measured instead: for pull requests in one year that token received 82 nodes, 26 of them null, where a token carrying the `repo` scope received only the remaining 56 and resolved them identically. A null slot is therefore a contribution to a repository *no* available token can read — private, or already deleted, which is what v0.2.1 said. The user's own repositories are not involved; they all resolve.

## v0.2.2 - 2026-08-03
### What's Changed
- Stop the image from overriding its own configuration. `CMD` passed `--addr` and `--db`, and since the `RS_*` variables only supply each flag's default, those flags won: a deployment configured with `RS_DATABASE_DSN=postgres://…` silently kept its state in a container-local SQLite file and lost every configuration change with the next container. The defaults now come from `ENV`, which a caller can override.
- Report which contributions were skipped, broken down by year, activity kind and whether it was the repository or the item that the token could not see. A bare total said something had been dropped but gave no handle on what.
- Correct what v0.2.1 claimed about those skipped contributions. They are not necessarily private or deleted repositories: a fine-grained PAT is scoped to selected repositories, so activity anywhere else — the user's own repositories included — comes back as null. Own-repository activity is filtered out regardless, so a skip is not by itself missing data. A classic PAT resolves them, as the README already recommends.

## v0.2.1 - 2026-08-03
### What's Changed
- Drop contributions the GitHub token cannot resolve. A repository that has since gone private or been deleted comes back as JSON `null`, which unmarshalled into a zero-valued struct and aggregated under the empty repository name — rendering as a nameless `<details>` block whose entries were bare, unlinked bullets. Such nodes are now skipped, without inflating counts, and the number dropped is logged.

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
