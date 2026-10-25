"use client";

import { useRouter } from "next/navigation";
import { useEffect, useState } from "react";
import { ApiError, api } from "@/lib/api";
import { apiErrorMessage } from "@/lib/errors";
import { useApi } from "@/lib/hooks";
import type { SetupStatus } from "@/lib/types";
import { useI18n } from "@/i18n";
import { useAuth } from "@/components/AuthProvider";
import { useToast } from "@/components/ToastProvider";
import { Button, ErrorBanner, Icon, SelectField, TextField } from "@/components/ui";

export default function SetupPage() {
  const { t, locale, setLocale } = useI18n();
  const auth = useAuth();
  const { push } = useToast();
  const router = useRouter();
  const status = useApi<SetupStatus>("/setup/status");
  const [step, setStep] = useState(1);
  const [email, setEmail] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [projectName, setProjectName] = useState("");
  const [setupToken, setSetupToken] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [submitting, setSubmitting] = useState(false);

  useEffect(() => {
    if (!status.loading && status.data && !status.data.setupRequired) {
      router.replace("/login");
    }
  }, [status.loading, status.data, router]);

  useEffect(() => {
    if (!projectName) setProjectName(t("setup.defaultProject"));
  }, [projectName, t]);

  function validateAccount(): boolean {
    const errors: Record<string, string> = {};
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim())) errors.email = t("setup.errorEmail");
    if (password.length < 12) errors.password = t("setup.errorPassword");
    if (password !== confirm) errors.confirm = t("setup.errorMismatch");
    setFieldErrors(errors);
    return Object.keys(errors).length === 0;
  }

  function nextStep() {
    setError(null);
    if (validateAccount()) setStep(2);
  }

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    setError(null);
    if (!projectName.trim()) {
      setFieldErrors({ project: t("setup.errorProject") });
      return;
    }
    if (!validateAccount()) {
      setStep(1);
      return;
    }
    setSubmitting(true);
    try {
      await api("/setup", {
        method: "POST",
        headers: setupToken ? { "X-LIP-Setup-Token": setupToken } : undefined,
        body: {
          email: email.trim(),
          displayName: displayName.trim(),
          password,
          projectName: projectName.trim(),
          locale,
        },
      });
      await auth.refresh();
      push(t("setup.success"), "success");
      router.replace("/dashboard");
    } catch (err) {
      if (err instanceof ApiError && err.code === "setup_closed") {
        router.replace("/login");
        return;
      }
      setError(apiErrorMessage(t, err));
    } finally {
      setSubmitting(false);
    }
  }

  if (status.loading) {
    return (
      <div className="boot-screen">
        <span className="spinner" />
      </div>
    );
  }
  if (status.error) {
    return (
      <div className="boot-screen">
        <ErrorBanner message={t(`errors.${status.error.code}`)} onRetry={status.reload} retryLabel={t("common.retry")} />
      </div>
    );
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
            <strong>{status.data?.appName ?? t("app.name")}</strong>
            <span>{t("app.tagline")}</span>
          </div>
        </div>
        <div className="auth-aside-copy">
          <h1>{t("setup.title")}</h1>
          <p>{t("setup.subtitle")}</p>
          <ul className="auth-points">
            <li><Icon name="check" size={16} />{t("setup.bullet1")}</li>
            <li><Icon name="check" size={16} />{t("setup.bullet2")}</li>
            <li><Icon name="check" size={16} />{t("setup.bullet3")}</li>
          </ul>
        </div>
        <div className="auth-aside-foot">
          <Icon name="lock" size={16} /> {t("setup.securityNote")}
        </div>
      </section>

      <section className="auth-main">
        <div className="auth-card">
          <div className="wizard-steps" aria-label={t("setup.stepOf", { current: step, total: 2 })}>
            <span className={`wizard-step ${step >= 1 ? "is-active" : ""}`}>1</span>
            <span className="wizard-line" />
            <span className={`wizard-step ${step >= 2 ? "is-active" : ""}`}>2</span>
            <span className="wizard-caption">{t("setup.stepOf", { current: step, total: 2 })}</span>
          </div>

          <form onSubmit={submit} className="auth-form" noValidate>
            {step === 1 ? (
              <>
                <header className="auth-form-head">
                  <h2>{t("setup.stepAccount")}</h2>
                  <p>{t("setup.accountHint")}</p>
                </header>
                <TextField
                  label={t("setup.email")}
                  type="email"
                  autoComplete="email"
                  value={email}
                  onChange={(event) => setEmail(event.target.value)}
                  error={fieldErrors.email}
                  data-autofocus
                  required
                />
                <TextField
                  label={t("setup.displayName")}
                  autoComplete="name"
                  value={displayName}
                  onChange={(event) => setDisplayName(event.target.value)}
                  hint={t("common.optional")}
                />
                <TextField
                  label={t("setup.password")}
                  type="password"
                  autoComplete="new-password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  hint={t("setup.passwordHint")}
                  error={fieldErrors.password}
                  required
                />
                <TextField
                  label={t("setup.confirmPassword")}
                  type="password"
                  autoComplete="new-password"
                  value={confirm}
                  onChange={(event) => setConfirm(event.target.value)}
                  error={fieldErrors.confirm}
                  required
                />
                <TextField
                  label={t("setup.setupToken")}
                  type="password"
                  value={setupToken}
                  onChange={(event) => setSetupToken(event.target.value)}
                  hint={t("setup.setupTokenHint")}
                />
                {error ? <ErrorBanner message={error} /> : null}
                <div className="auth-actions">
                  <Button variant="primary" onClick={nextStep} type="button">
                    {t("setup.next")}
                  </Button>
                </div>
              </>
            ) : (
              <>
                <header className="auth-form-head">
                  <h2>{t("setup.stepProject")}</h2>
                  <p>{t("setup.projectHint")}</p>
                </header>
                <TextField
                  label={t("setup.projectName")}
                  value={projectName}
                  onChange={(event) => setProjectName(event.target.value)}
                  error={fieldErrors.project}
                  data-autofocus
                  required
                />
                <SelectField
                  label={t("setup.locale")}
                  value={locale}
                  onChange={(event) => setLocale(event.target.value as "en" | "ko")}
                >
                  <option value="en">{t("language.en")}</option>
                  <option value="ko">{t("language.ko")}</option>
                </SelectField>
                <div className="review-box">
                  <span className="review-label">{t("setup.reviewAccount")}</span>
                  <strong>{email}</strong>
                  <span className="review-label">{t("setup.reviewProject")}</span>
                  <strong>{projectName}</strong>
                </div>
                {error ? <ErrorBanner message={error} /> : null}
                <div className="auth-actions split">
                  <Button variant="ghost" onClick={() => setStep(1)} type="button" icon="arrowRight">
                    {t("common.back")}
                  </Button>
                  <Button variant="primary" type="submit" loading={submitting}>
                    {t("setup.submit")}
                  </Button>
                </div>
              </>
            )}
          </form>
        </div>
      </section>
    </div>
  );
}
