# MVP acceptance map

Scope: single-team, authorized security workflow management with a working
vertical slice: auth → conversation → bounded task → approval → finding →
report. Active scanning, SSO/MFA, PDF reports, multi-tenant isolation, and
arbitrary tool plugins are explicitly out of scope.

| Requirement | Where satisfied | Evidence |
|---|---|---|
| First-run admin password setup | `POST /api/v1/setup`, `web/src/app/setup/page.tsx` | advisory-locked transaction; setup closes after first user; integration test |
| Auth/session | `internal/auth`, `internal/httpapi/handlers_auth.go`, `middleware.go` | opaque tokens, SHA-256 at rest, HttpOnly/SameSite cookie, bearer for API clients, origin/Sec-Fetch-Site checks |
| CRUD: projects, conversations, tasks, findings, assets, providers, users | `internal/httpapi/handlers_*.go`, `internal/store` | list/create/read/update/archived-delete; messages are append-only chat records; audit is append-only |
| LLM providers (cloud + Ollama) | `internal/llm`, `handlers_providers.go` | Ollama and OpenAI-compatible clients; env-referenced keys; mock provider removed |
| Safe local-only mode | `internal/config`, `internal/egress`, `handlers_assets.go`, `handlers_providers.go` | missing/invalid config fails closed; allowlist; DNS + dial IP checks; no proxy env; cloud/remote imports blocked |
| Asset import sync | `internal/assets`, `handlers_assets.go` | inline/file/allowlisted URL; root confinement; normalization/dedupe; insert/update/skip counts |
| Approval records | `internal/store/approvals.go`, `handlers_tasks.go`, `runner` | one pending per task; 24h expiry; self-approval denied for analysts; task blocked on expiry; audited |
| Structured logging/audit | `internal/audit`, `middleware.go`, all handlers | actor, action, target, result, IP, metadata, timestamp; same transaction as mutation |
| Report generation | `internal/report`, `handlers_settings.go` | deterministic Markdown; HTML escaped; assets/findings/tasks/approvals/disclaimer |
| i18n EN/KO | `internal/i18n/locales`, `web/src/i18n`, `/api/v1/i18n/{locale}` | one source of truth; profile + cookie persistence; no hardcoded UI copy |
| Same codebase: Compose + embedded binary | `Dockerfile`, `docker-compose.yml`, `internal/webui` | multi-stage build copies Next export into `go:embed`; SPA fallback; single binary tested |
| Autonomy bounds | `config.MaxConcurrent`, `toolcatalog`, `runner` | max 8, default 2; max 3 planned tasks; active tools immutable and approval-gated; no shell/exploit tools |
| Tests | `internal/**/*_test.go` | unit + Postgres integration covering auth, safe mode, approvals, runner, report, UI |
| Packaging/env | `Dockerfile`, `docker-compose.yml`, `.env.example`, `Makefile` | non-root/read-only container, healthchecks, required secret env; compose smoke tested |
| Documented runbook | `README.md` | Compose and binary commands, first-run, seed, tests, operations |
| No hardcoded secrets | config/env, provider `apiKeyEnv` | no key material in DB or source; `.env` gitignored |
| Parameterized SQL | `internal/store` | pgx bind parameters; no user-controlled SQL fragments |

## Definition of Done

- [x] `go test ./...` passes with a disposable `LIP_TEST_DATABASE_URL`.
- [x] `go vet ./...` passes.
- [x] `npx tsc --noEmit` passes.
- [x] `npm run build` exports the Next.js app.
- [x] `docker compose up --build -d` reaches healthy and `/readyz` returns ready.
- [x] First-run setup creates admin + project and closes subsequent setup.
- [x] Login/logout/change-password work; temp-password users are write-blocked.
- [x] Safe mode denies remote imports and cloud providers; allowlisted local HEAD succeeds.
- [x] Plan creates passive tasks and an approval-gated active task.
- [x] Analyst self-approval is denied; admin approval queues the task.
- [x] Runner executes the approved HEAD against the allowlisted local target.
- [x] Finding synthesis persists an LLM-sourced draft.
- [x] Report contains assets, findings, tasks, approvals, and escaped content.
- [x] Audit contains setup, import, plan, approval, execution, and export actions.
- [x] EN/KO switch persists to profile and cookie; UI has no raw i18n keys.
- [x] Single binary serves the embedded SPA and API.
