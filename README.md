# dawnlink

A self-hosted Go service inspired by [nightly.link](https://nightly.link), with Russian and English interface localization.

The service provides permanent public links to GitHub Actions artifacts without requiring visitors to sign in to GitHub.

## Features

- Home page: paste a GitHub link to redirect to a dawnl.ink URL
- Latest artifact for a workflow and branch: `/{owner}/{repo}/workflows/{workflow}/{branch}/{artifact}`
- Direct downloads: append `.zip` to the URL
- Artifact list for a run: `/{owner}/{repo}/actions/runs/{run_id}`
- Specific artifact: `/{owner}/{repo}/actions/artifacts/{id}`
- GitHub URL import through the home page form (workflow, run, artifact, and job log URLs)
- OAuth dashboard for private repositories
- GitHub App installation tokens and SQLite repository cache
- Localization: English fallback and Russian for users with a Russian locale; explicit selection through the `lang` cookie or `?lang=` query parameter

## Requirements

- Go 1.26+

## Quick Start

```bash
cp .env.example .env
# Fill in the required variables and add the app's PEM key.
# Go does not load .env automatically:
set -a
source .env
set +a

go run ./cmd/dawnlink
```

Open http://localhost:8080/

## GitHub App

The public app is [dawnl.ink](https://github.com/apps/dawnl-ink) (`dawnl-ink`).

For self-hosting, create an app with **Read-only** permissions for:

- Actions (workflows, runs, artifacts)
- Metadata

No webhook is required. Configure these GitHub App settings:

- Homepage URL: the value of `URL` (first URL when several are configured)
- Setup URL: `{URL}setup` for each configured public URL
- Callback URL: `{URL}dashboard` for each configured public URL, when using the OAuth dashboard

After installation, GitHub calls the Setup URL with the `installation_id` parameter, and the service updates its list of available repositories.

To access public repositories where the app is not installed, set `FALLBACK_INSTALLATION_ID` to the app installation ID for any repository, as in the upstream service.

## Configuration

| Variable | Required | Description |
|----------|----------|-------------|
| `GITHUB_APP_ID` | Yes | GitHub App ID |
| `GITHUB_PEM_FILENAME` | Yes | Path to the GitHub App private PEM key |
| `APP_SECRET` | Yes | Random secret of at least 32 characters for private `?h=` links |
| `GITHUB_APP_NAME` | No | App slug; defaults to `dawnl-ink` |
| `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET` | Together | OAuth dashboard for private repositories |
| `FALLBACK_INSTALLATION_ID` | No | Installation token used for public repositories |
| `PORT` | No | HTTP port; defaults to `8080` |
| `URL` | No | Public absolute service URL, or comma-separated list of URLs; defaults to `http://localhost:{PORT}/`. The first URL is primary; links and OAuth redirects use the request host when it matches a configured URL |
| `DATABASE_FILE` | No | SQLite database path; defaults to `./db.sqlite` |
| `DEFAULT_LOCALE` | No | Fallback language, `en` or `ru`; defaults to `en` |

Generate a secret:

```bash
openssl rand -hex 32
```

## URL Examples

| Purpose | URL |
|---------|-----|
| Latest successful artifact | `/quassel/quassel/workflows/main/master/Windows` |
| Any completed run | `?status=completed` |
| Run artifact list | `/owner/repo/actions/runs/12345678` |
| Download a ZIP archive | `.../artifact-name.zip` |
| Job logs | `/owner/repo/runs/{job_id}.txt` |

Workflow names without an extension automatically receive the `.yml` extension. The `status` parameter accepts only `success` and `completed`. Private links contain the `h` secret; the service marks these pages as `noindex` and does not send a referrer to external sites.

## Docker

The SQLite database and PEM key must be available inside the container:

```bash
docker build -t dawnlink .
docker run --rm -p 8080:8080 \
  --env-file .env \
  -e GITHUB_PEM_FILENAME=/run/secrets/github-app.pem \
  -e DATABASE_FILE=/data/db.sqlite \
  -v "$PWD/private-key.pem:/run/secrets/github-app.pem:ro" \
  -v dawnlink-data:/data \
  dawnlink
```

Multi-architecture builds use Go cross-compilation, so QEMU is not required for the build stage:

```bash
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t registry.example/dawnlink:latest \
  --push .
```

BuildKit caches modules and the Go build cache between builds. The production image is based on `distroless/static-debian12:nonroot` and runs the process without root privileges.

For production, set `URL` to a public HTTPS URL, or several comma-separated URLs if the service is reachable on multiple domains. Register each `{URL}setup` and `{URL}dashboard` in the GitHub App settings. Do not publish the `.env` file, PEM key, SQLite database, or links containing `h`.

## Verification

```bash
make test
go vet ./...
go test -race ./...
```

## GitHub Actions

- `CI`: formatting, module checks, `go vet`, regular and race tests, build, and `govulncheck`
- `Binaries`: Linux, macOS, and Windows archives (`amd64`/`arm64`), GitHub Releases, and SHA-256 checksums for `v*` tags
- `Container`: multi-architecture `linux/amd64,linux/arm64` image in GHCR, with SBOM and provenance; pull requests build without publishing
- `CodeQL`: Go analysis on pushes, pull requests, and weekly
- Dependabot: weekly updates for Go modules, Docker images, and GitHub Actions

A push to `main` publishes the `ghcr.io/{owner}/{repo}:latest` container and binary artifacts. A tag such as `v1.2.3` also publishes semantic-version container tags and a GitHub Release.

## License

AGPL-3.0
