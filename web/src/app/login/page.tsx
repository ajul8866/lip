"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { ApiError, api } from "@/lib/api";
import { apiErrorMessage } from "@/lib/errors";
import { useApi } from "@/lib/hooks";
import type { SetupStatus } from "@/lib/types";
import { useI18n } from "@/i18n";
import { useAuth } from "@/components/AuthProvider";
import { Button, ErrorBanner, Icon, TextField } from "@/components/ui";

export default function LoginPage() {
  const { t } = useI18n();
  const auth = useAuth();
  const router = useRouter();
  const system = useApi<SetupStatus>("/setup/status");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (system.data?.setupRequired) {
      router.replace("/setup");
      return;
    }
    if (auth.status === "authenticated") {
      router.replace("/dashboard");
    }
  }, [system.data?.setupRequired, auth.status, router]);

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      await api("/auth/login", { method: "POST", body: { email: email.trim(), password } });
      await auth.refresh();
      router.replace("/dashboard");
    } catch (err) {
      if (err instanceof ApiError && (err.code === "invalid_credentials" || err.status === 401)) {
        setError(t("login.invalid"));
      } else {
        setError(apiErrorMessage(t, err));
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="auth-layout">
      <section className="auth-aside">
        <div className="auth-aside-grid" aria-hidden="true" />
        <div className="auth-brand">
          <span className="brand-mark brand-mark-lg">
            <Icon name="shield" size={28} />
          </span>
          <div>
            <strong>{system.data?.appName ?? t("app.name")}</strong>
            <span>{t("app.tagline")}</span>
          </div>
        </div>
        <div className="auth-aside-copy">
          <h1>{t("login.title")}</h1>
          <p>{t("login.subtitle")}</p>
          <div className="auth-signal">
            <div className="signal-row">
              <span className="signal-dot signal-ok" />
              <span>{t("setup.bullet1")}</span>
            </div>
            <div className="signal-row">
              <span className="signal-dot signal-ok" />
              <span>{t("setup.bullet2")}</span>
            </div>
            <div className="signal-row">
              <span className="signal-dot" />
              <span>{t("setup.bullet3")}</span>
            </div>
          </div>
        </div>
        <div className="auth-aside-foot">
          <Icon name="lock" size={16} /> {t("setup.securityNote")}
        </div>
      </section>

      <section className="auth-main">
        <div className="auth-card">
          <form onSubmit={submit} className="auth-form" noValidate>
            <header className="auth-form-head">
              <h2>{t("login.title")}</h2>
              <p>{t("login.subtitle")}</p>
            </header>
            <TextField
              label={t("login.email")}
              type="email"
              autoComplete="username"
              value={email}
              onChange={(event) => setEmail(event.target.value)}
              data-autofocus
              required
            />
            <TextField
              label={t("login.password")}
              type="password"
              autoComplete="current-password"
              value={password}
              onChange={(event) => setPassword(event.target.value)}
              required
            />
            {error ? <ErrorBanner message={error} /> : null}
            <div className="auth-actions">
              <Button variant="primary" type="submit" loading={submitting} disabled={!email || !password}>
                {t("login.submit")}
              </Button>
            </div>
          </form>
        </div>
      </section>
    </div>
  );
}
