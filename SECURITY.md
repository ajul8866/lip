# Security policy

LIP is intended for authorized security work. Do not point it at systems you do
not own or have written permission to test.

## Reporting

Report vulnerabilities privately to the maintainers of your deployment. Do not
open a public issue containing target data, credentials, or exploit details.

## Deployment baseline

- Terminate TLS before exposing the UI outside loopback.
- Set `LIP_SETUP_TOKEN` until the first administrator exists.
- Keep `LIP_SAFE_LOCAL_ONLY=true` unless egress has been reviewed.
- Use a dedicated PostgreSQL role/database and back it up.
- Rotate LLM API keys in the process environment; they are not stored in the database.
- Treat audit logs and reports as sensitive engagement data.
- The app container runs read-only, non-root, with all capabilities dropped and
  no privilege escalation.

## Known MVP limitations

- Local passwords only; no MFA or SSO.
- All authenticated team members can read all projects (single-team model).
- Audit retention, export, and purge are manual.
- In-process rate limiting is per replica.
- LLM output is untrusted draft material and must be validated.
