# LIP

LIP is an LLM-driven security workflow manager for authorized, in-house red
teams. It keeps the workflow auditable: one project contains declared assets,
conversations, bounded tasks, approvals, findings, and an exported report. The
LLM can plan and draft; it cannot execute unbounded tools.

**MVP scope:** local accounts, projects, declared assets, conversations,
bounded tasks, human approvals, findings, Markdown reports, safe local-only
egress, local Ollama and OpenAI-compatible providers. **Out of scope:** SSO,
MFA, PDF reports, exploit tooling, arbitrary plugins, multi-tenant isolation,
and an active scanning engine. The two built-in tools demonstrate the policy:
`inventory_summary` (passive) and `http_head` (active, approval-gated).

## Architecture

```text
Browser (Next.js static export, EN/KO)
        |
        | same-origin /api/v1, HttpOnly session cookie
        v
Go HTTP server  ──────────────  embedded UI (go:embed)
  │ auth/session + CSRF origin checks
  │ RBAC + audit (same transaction as mutation)
  │ bounded runner (max 2 by default)
  │ LLM registry ── egress guard ── Ollama / OpenAI-compatible
  │ asset import (inline / confined file / allowlisted URL)
  v
PostgreSQL 17  (users, sessions, projects, conversations, messages,
                tasks, findings, assets, approvals, audit_log,
                settings, llm_providers, schema_migrations)
```

The frontend is exported statically and embedded into the Go binary. Docker
Compose runs the same image plus PostgreSQL. A standalone binary only needs a
`DATABASE_URL`; migrations run on startup.

## Quick start (Docker Compose)

```sh
cp .env.example .env
# Required: set POSTGRES_PASSWORD in .env
# Recommended: set LIP_SETUP_TOKEN in .env
docker compose up --build -d
```

Open <http://127.0.0.1:8080>. If `LIP_SETUP_TOKEN` is set, enter it in the setup
wizard. The first-run form creates the administrator and first project; it is
closed as soon as one user exists.

The app binds to `127.0.0.1` by default. Set `LIP_BIND_ADDR=0.0.0.0` only behind
a reviewed reverse proxy with TLS and `LIP_SECURE_COOKIES=true`.

## Run as a single binary

```sh
export DATABASE_URL="postgres://lip:${POSTGRES_PASSWORD}@127.0.0.1:5432/lip?sslmode=disable"
export LIP_IMPORT_ROOT='./data/imports'
export LIP_SETUP_TOKEN="$(openssl rand -hex 24)"
make build          # builds the Next.js export, embeds it, then builds bin/lip
./bin/lip
```

`make build-go` skips the frontend rebuild and embeds whatever is currently in
`internal/webui/dist`. `make migrate` applies migrations and exits.

## LLM providers

LIP stores provider *configuration*, never key material. An OpenAI-compatible
provider references an environment variable name matching
`LIP_LLM_[A-Z0-9_]+`; the value is read from the process environment.

Example: local Ollama

```sh
ollama serve
ollama pull llama3.1
```

Set `LIP_EGRESS_ALLOWLIST=127.0.0.1:11434`, restart, then add an Ollama provider
in **Settings → LLM providers** with base URL `http://127.0.0.1:11434` and model
`llama3.1`. In safe local-only mode the host must be loopback/private **and**
explicitly allowlisted.

Example: OpenAI-compatible cloud

```sh
LIP_SAFE_LOCAL_ONLY=false
LIP_EGRESS_ALLOWLIST=api.openai.com:443
LIP_LLM_API_KEY=... # referenced by name as apiKeyEnv in the provider form
```

Cloud providers are blocked while safe local-only mode is on. The egress guard
still applies in standard mode; the destination must be allowlisted.

## Safe local-only mode (fail closed)

When `LIP_SAFE_LOCAL_ONLY` is missing, LIP enables it. When the value is invalid,
LIP enables it **and clears the allowlist**, so a typo cannot open egress.
`/api/v1/system/status` reports the resolved mode and the number of rules.

Safe mode blocks:

- outbound HTTP to any host/port not explicitly allowlisted;
- public IP destinations, even if an allowlisted name resolves to one;
- metadata/link-local addresses (`169.254.0.0/16`, `fd00:ec2::254`) always;
- cloud/OpenAI-compatible providers;
- remote (`url`) asset imports;
- redirect targets that are not allowlisted (HTTP HEAD does not follow redirects).

Local file imports are confined to `LIP_IMPORT_ROOT`; symlinks escaping that root
are rejected. The HTTP client ignores proxy environment variables and revalidates
every redirect. DNS is resolved before dialing and the actual connected IP is
checked again (`net.Dialer.Control`).

Allowlist entries are exact `host:port` values. Wildcards are accepted only
outside safe mode. Invalid entries fail startup.

## Autonomy boundary

- `LIP_MAX_CONCURRENT_TASKS` (default 2, max 8) bounds the in-process runner.
- The planner may propose at most 3 tasks.
- Passive tools (`inventory_summary`, `synthesize_findings`) may run automatically.
- Active tools (`http_head`) are created in `awaiting_approval`, must target a
  URL asset declared in the project, and cannot change parameters after creation.
- Analysts cannot approve their own active task; admins can. Every decision is audited.
- Approvals expire after 24 hours and the task is blocked.
- `LIP_AUTORUN=false` disables automatic claiming; approvals and records still work.
- There is no shell execution, arbitrary plugin loading, or exploit tool.

## Data model and migrations

DDL lives in [`internal/db/migrations/0001_init.sql`](internal/db/migrations/0001_init.sql).
Migrations are embedded, forward-only, checksum-verified, and applied under a
PostgreSQL advisory lock. Never edit an applied migration; add a new file.

All access goes through `internal/store` using bind parameters. IDs are
application-generated UUID strings. Mutations and their audit rows share a
transaction, so a mutation cannot commit without its audit entry.

## API

The OpenAPI document is [`docs/openapi.yaml`](docs/openapi.yaml). All routes are
under `/api/v1`. Session auth uses the `lip_session` HttpOnly cookie; API clients
may use `Authorization: Bearer <session-token>` only if they obtain a token
through a trusted out-of-band channel (the UI uses cookies). State-changing
cookie requests are origin/Sec-Fetch-Site checked and JSON content is required.

## Seed data

```sh
export LIP_SEED_PASSWORD='a long unique passphrase'
export LIP_SEED_SETUP_TOKEN='the value from .env, if configured'
scripts/seed.sh
```

The script uses only the public API: it sets up or logs in, creates/selects a
project, imports [`examples/assets.txt`](examples/assets.txt), opens a
conversation, sends a scoped message, and queues a local inventory task. It does
not bypass approvals or audit.

## Tests

```sh
make test
LIP_TEST_DATABASE_URL="postgres://lip:${POSTGRES_PASSWORD}@127.0.0.1:5432/lip_test?sslmode=disable" make test-integration
make lint
```

The integration test resets the `public` schema of the supplied test database.
Use a dedicated database; never point it at production. The suite covers setup,
login, CSRF origin checks, provider policy, safe-mode import denial, plan
generation, passive execution, self-approval denial, admin approval, HEAD
execution against a local allowlisted server, report export, and audit coverage.

## Operations

- Back up the PostgreSQL volume; audit retention is not automated in the MVP.
- Keep `LIP_SETUP_TOKEN` set until the first admin exists, then remove it.
- Terminate TLS at a reviewed proxy and set `LIP_SECURE_COOKIES=true` plus
  `LIP_TRUST_PROXY=true` only when the proxy strips client-supplied forwarding headers.
- The app container is read-only, drops all capabilities, and runs as non-root.
- Logs are JSON on stdout; audit rows are in PostgreSQL.
- `GET /api/v1/readyz` checks the database; `GET /api/v1/healthz` is liveness.

## License

Apache-2.0. See [`LICENSE`](LICENSE).
