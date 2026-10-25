-- LIP initial schema. All timestamps are UTC timestamptz. IDs are
-- application-generated UUIDv4 strings so the same value can be used in
-- audit records and API responses without a second round trip.

CREATE TABLE users (
    id                   text PRIMARY KEY CHECK (id ~ '^[0-9a-f-]{36}$'),
    email                text NOT NULL,
    display_name         text NOT NULL DEFAULT '',
    password_hash        text NOT NULL,
    role                 text NOT NULL DEFAULT 'analyst' CHECK (role IN ('admin', 'analyst', 'viewer')),
    locale               text NOT NULL DEFAULT 'en' CHECK (locale IN ('en', 'ko')),
    must_change_password boolean NOT NULL DEFAULT false,
    disabled_at          timestamptz,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_lower_uidx ON users (lower(email));
CREATE INDEX users_role_idx ON users (role);

CREATE TABLE sessions (
    id           text PRIMARY KEY CHECK (id ~ '^[0-9a-f-]{36}$'),
    user_id      text NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash   text NOT NULL UNIQUE,
    ip           text NOT NULL DEFAULT '',
    user_agent   text NOT NULL DEFAULT '',
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    expires_at   timestamptz NOT NULL,
    revoked_at   timestamptz
);
CREATE INDEX sessions_user_idx ON sessions (user_id, created_at DESC);
CREATE INDEX sessions_expiry_idx ON sessions (expires_at) WHERE revoked_at IS NULL;

CREATE TABLE projects (
    id          text PRIMARY KEY CHECK (id ~ '^[0-9a-f-]{36}$'),
    name        text NOT NULL,
    description text NOT NULL DEFAULT '',
    created_by  text REFERENCES users (id) ON DELETE SET NULL,
    archived_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX projects_created_idx ON projects (created_at DESC);

CREATE TABLE conversations (
    id         text PRIMARY KEY CHECK (id ~ '^[0-9a-f-]{36}$'),
    project_id text NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    title      text NOT NULL,
    created_by text REFERENCES users (id) ON DELETE SET NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX conversations_project_idx ON conversations (project_id, updated_at DESC);

CREATE TABLE messages (
    id              text PRIMARY KEY CHECK (id ~ '^[0-9a-f-]{36}$'),
    conversation_id text NOT NULL REFERENCES conversations (id) ON DELETE CASCADE,
    role            text NOT NULL CHECK (role IN ('system', 'user', 'assistant', 'tool')),
    content         text NOT NULL,
    metadata        jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_by      text REFERENCES users (id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX messages_conversation_idx ON messages (conversation_id, created_at);

CREATE TABLE tasks (
    id              text PRIMARY KEY CHECK (id ~ '^[0-9a-f-]{36}$'),
    project_id      text NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    conversation_id text REFERENCES conversations (id) ON DELETE SET NULL,
    title           text NOT NULL,
    description     text NOT NULL DEFAULT '',
    tool            text NOT NULL,
    parameters      jsonb NOT NULL DEFAULT '{}'::jsonb,
    risk_level      text NOT NULL CHECK (risk_level IN ('passive', 'active', 'destructive')),
    status          text NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'awaiting_approval', 'running', 'succeeded', 'failed', 'cancelled', 'blocked')),
    attempts        integer NOT NULL DEFAULT 0,
    last_error      text NOT NULL DEFAULT '',
    result          jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_by      text REFERENCES users (id) ON DELETE SET NULL,
    approved_by     text REFERENCES users (id) ON DELETE SET NULL,
    started_at      timestamptz,
    finished_at     timestamptz,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX tasks_project_idx ON tasks (project_id, created_at DESC);
CREATE INDEX tasks_status_idx ON tasks (status, created_at);
CREATE INDEX tasks_conversation_idx ON tasks (conversation_id, created_at);
CREATE INDEX tasks_claim_idx ON tasks (status, risk_level, created_at) WHERE status = 'queued';

CREATE TABLE assets (
    id         text PRIMARY KEY CHECK (id ~ '^[0-9a-f-]{36}$'),
    project_id text NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    kind       text NOT NULL CHECK (kind IN ('host', 'domain', 'ip', 'cidr', 'url', 'repo', 'other')),
    value      text NOT NULL,
    source     text NOT NULL DEFAULT 'manual',
    attributes jsonb NOT NULL DEFAULT '{}'::jsonb,
    first_seen timestamptz NOT NULL DEFAULT now(),
    last_seen  timestamptz NOT NULL DEFAULT now(),
    created_by text REFERENCES users (id) ON DELETE SET NULL,
    UNIQUE (project_id, kind, value)
);
CREATE INDEX assets_project_idx ON assets (project_id, kind);
CREATE INDEX assets_value_idx ON assets (value);

CREATE TABLE findings (
    id          text PRIMARY KEY CHECK (id ~ '^[0-9a-f-]{36}$'),
    project_id  text NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    task_id     text REFERENCES tasks (id) ON DELETE SET NULL,
    asset_id    text REFERENCES assets (id) ON DELETE SET NULL,
    title       text NOT NULL,
    description text NOT NULL DEFAULT '',
    severity    text NOT NULL CHECK (severity IN ('info', 'low', 'medium', 'high', 'critical')),
    status      text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'triaged', 'accepted', 'fixed', 'false_positive')),
    source      text NOT NULL DEFAULT 'manual' CHECK (source IN ('manual', 'llm', 'import')),
    evidence    jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_by  text REFERENCES users (id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX findings_project_idx ON findings (project_id, created_at DESC);
CREATE INDEX findings_severity_idx ON findings (project_id, severity, status);
CREATE INDEX findings_task_idx ON findings (task_id);

CREATE TABLE approvals (
    id           text PRIMARY KEY CHECK (id ~ '^[0-9a-f-]{36}$'),
    project_id   text NOT NULL REFERENCES projects (id) ON DELETE CASCADE,
    task_id      text NOT NULL REFERENCES tasks (id) ON DELETE CASCADE,
    action       text NOT NULL,
    reason       text NOT NULL DEFAULT '',
    status       text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'denied', 'expired')),
    requested_by text REFERENCES users (id) ON DELETE SET NULL,
    decided_by   text REFERENCES users (id) ON DELETE SET NULL,
    expires_at   timestamptz NOT NULL,
    decided_at   timestamptz,
    created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX approvals_project_idx ON approvals (project_id, created_at DESC);
CREATE INDEX approvals_task_idx ON approvals (task_id);
CREATE UNIQUE INDEX approvals_one_pending_per_task ON approvals (task_id) WHERE status = 'pending';

CREATE TABLE audit_log (
    id          bigserial PRIMARY KEY,
    actor_id    text REFERENCES users (id) ON DELETE SET NULL,
    actor_email text NOT NULL DEFAULT 'system',
    action      text NOT NULL,
    target_type text NOT NULL DEFAULT '',
    target_id   text NOT NULL DEFAULT '',
    result      text NOT NULL CHECK (result IN ('success', 'failure', 'denied')),
    ip          text NOT NULL DEFAULT '',
    metadata    jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_log_created_idx ON audit_log (created_at DESC);
CREATE INDEX audit_log_actor_idx ON audit_log (actor_id, created_at DESC);
CREATE INDEX audit_log_action_idx ON audit_log (action, created_at DESC);

CREATE TABLE settings (
    key        text PRIMARY KEY,
    value      jsonb NOT NULL,
    updated_by text REFERENCES users (id) ON DELETE SET NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE llm_providers (
    id          text PRIMARY KEY CHECK (id ~ '^[0-9a-f-]{36}$'),
    name        text NOT NULL,
    kind        text NOT NULL CHECK (kind IN ('ollama', 'openai')),
    base_url    text NOT NULL DEFAULT '',
    model       text NOT NULL,
    api_key_env text NOT NULL DEFAULT '',
    enabled     boolean NOT NULL DEFAULT true,
    is_default  boolean NOT NULL DEFAULT false,
    config      jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX llm_providers_name_uidx ON llm_providers (lower(name));
CREATE UNIQUE INDEX llm_providers_one_default ON llm_providers (is_default) WHERE is_default;

INSERT INTO settings (key, value) VALUES
    ('app.name', '"LIP"'::jsonb),
    ('i18n.default_locale', '"en"'::jsonb),
    ('report.disclaimer', '"This report was produced by LIP from analyst-supplied and LLM-drafted evidence. Validate every finding before client delivery."'::jsonb)
ON CONFLICT (key) DO NOTHING;
