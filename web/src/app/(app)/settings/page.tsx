"use client";

import { useEffect, useMemo, useState } from "react";
import { ApiError, api } from "@/lib/api";
import { apiErrorMessage } from "@/lib/errors";
import { useApi } from "@/lib/hooks";
import type {
  AuditEntry,
  LLMProvider,
  ListResponse,
  Locale,
  Role,
  Setting,
  SystemStatus,
  User,
} from "@/lib/types";
import { useI18n } from "@/i18n";
import { useAuth } from "@/components/AuthProvider";
import { useToast } from "@/components/ToastProvider";
import { Drawer, Modal } from "@/components/Panel";
import {
  Button,
  Card,
  CheckboxField,
  EmptyState,
  ErrorBanner,
  Icon,
  PageHeader,
  RoleBadge,
  SelectField,
  Skeleton,
  StatusBadge,
  Tabs,
  TextField,
} from "@/components/ui";

const localeSettingKey = ["i18n", "default_locale"].join(".");

const roles: Role[] = ["admin", "analyst", "viewer"];

export default function SettingsPage() {
  const { t, locale, setLocale, formatDate } = useI18n();
  const auth = useAuth();
  const { user, setUser } = auth;
  const { push } = useToast();
  const isAdmin = user?.role === "admin";
  const [tab, setTab] = useState("profile");

  const system = useApi<SystemStatus>("/system/status");
  const providers = useApi<{ items: LLMProvider[] }>(isAdmin ? "/llm-providers" : null);
  const users = useApi<{ items: User[] }>(isAdmin ? "/users" : null);
  const audit = useApi<ListResponse<AuditEntry>>(isAdmin ? "/audit?limit=50" : null);
  const settings = useApi<{ items: Setting[] }>(isAdmin ? "/settings" : null);

  const [displayName, setDisplayName] = useState(user?.displayName ?? "");
  const [profileSaving, setProfileSaving] = useState(false);
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [passwordSaving, setPasswordSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  const [providerOpen, setProviderOpen] = useState(false);
  const [editingProvider, setEditingProvider] = useState<LLMProvider | null>(null);
  const [providerForm, setProviderForm] = useState({
    name: "",
    kind: "ollama" as "ollama" | "openai",
    baseUrl: "http://127.0.0.1:11434",
    model: "llama3.1",
    apiKeyEnv: "",
    enabled: true,
    isDefault: false,
  });
  const [providerSaving, setProviderSaving] = useState(false);
  const [testingProvider, setTestingProvider] = useState<string | null>(null);
  const [deletingProvider, setDeletingProvider] = useState<LLMProvider | null>(null);

  const [userOpen, setUserOpen] = useState(false);
  const [editingUser, setEditingUser] = useState<User | null>(null);
  const [userForm, setUserForm] = useState({
    email: "",
    displayName: "",
    role: "analyst" as Role,
    locale: locale as Locale,
    password: "",
    disabled: false,
  });
  const [userSaving, setUserSaving] = useState(false);
  const [resettingUser, setResettingUser] = useState<User | null>(null);
  const [resetPassword, setResetPassword] = useState("");

  const [appName, setAppName] = useState("");
  const [disclaimer, setDisclaimer] = useState("");
  const [defaultLocale, setDefaultLocale] = useState<Locale>("en");
  const [settingsSaving, setSettingsSaving] = useState(false);

  useEffect(() => {
    if (user) setDisplayName(user.displayName);
  }, [user]);

  useEffect(() => {
    if (!settings.data) return;
    const map = new Map(settings.data.items.map((item) => [item.key, item.value]));
    if (typeof map.get("app.name") === "string") setAppName(map.get("app.name") as string);
    if (typeof map.get("report.disclaimer") === "string") setDisclaimer(map.get("report.disclaimer") as string);
    if (map.get(localeSettingKey) === "en" || map.get(localeSettingKey) === "ko") {
      setDefaultLocale(map.get(localeSettingKey) as Locale);
    }
  }, [settings.data]);

  useEffect(() => {
    if (window.location.hash === "#password") setTab("password");
  }, []);

  useEffect(() => {
    setFormError(null);
  }, [tab]);

  const tabItems: { id: string; label: string; count?: number }[] = useMemo(() => {
    const items: { id: string; label: string; count?: number }[] = [
      { id: "profile", label: t("settings.profile") },
      { id: "password", label: t("settings.password") },
      { id: "system", label: t("settings.system") },
    ];
    if (isAdmin) {
      items.push(
        { id: "providers", label: t("settings.providers"), count: providers.data?.items.length },
        { id: "users", label: t("settings.users"), count: users.data?.items.length },
        { id: "audit", label: t("settings.audit"), count: audit.data?.total },
      );
    }
    return items;
  }, [t, isAdmin, providers.data, users.data, audit.data]);

  async function saveProfile(event: React.FormEvent) {
    event.preventDefault();
    setProfileSaving(true);
    setFormError(null);
    try {
      const response = await api<{ user: User }>("/auth/me", {
        method: "PATCH",
        body: { displayName: displayName.trim(), locale },
      });
      setUser(response.user);
      setLocale(response.user.locale);
      push(t("common.success"), "success");
    } catch (err) {
      setFormError(apiErrorMessage(t, err));
    } finally {
      setProfileSaving(false);
    }
  }

  async function changePassword(event: React.FormEvent) {
    event.preventDefault();
    if (newPassword !== confirmPassword) {
      setFormError(t("setup.mismatch"));
      return;
    }
    setPasswordSaving(true);
    setFormError(null);
    try {
      await api("/auth/change-password", {
        method: "POST",
        body: { currentPassword, newPassword },
      });
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
      push(t("settings.passwordChanged"), "success");
      await auth.refresh();
    } catch (err) {
      setFormError(apiErrorMessage(t, err));
    } finally {
      setPasswordSaving(false);
    }
  }

  function openProvider(provider?: LLMProvider) {
    if (provider) {
      setEditingProvider(provider);
      setProviderForm({
        name: provider.name,
        kind: provider.kind,
        baseUrl: provider.baseUrl,
        model: provider.model,
        apiKeyEnv: provider.apiKeyEnv,
        enabled: provider.enabled,
        isDefault: provider.isDefault,
      });
    } else {
      setEditingProvider(null);
      setProviderForm({
        name: "",
        kind: "ollama",
        baseUrl: "http://127.0.0.1:11434",
        model: "llama3.1",
        apiKeyEnv: "",
        enabled: true,
        isDefault: false,
      });
    }
    setFormError(null);
    setProviderOpen(true);
  }

  async function saveProvider(event: React.FormEvent) {
    event.preventDefault();
    setProviderSaving(true);
    setFormError(null);
    try {
      const body = { ...providerForm, config: {} };
      if (editingProvider) {
        await api(`/llm-providers/${editingProvider.id}`, { method: "PATCH", body });
      } else {
        await api("/llm-providers", { method: "POST", body });
      }
      await providers.reload();
      push(t("common.success"), "success");
      setProviderOpen(false);
    } catch (err) {
      setFormError(apiErrorMessage(t, err));
    } finally {
      setProviderSaving(false);
    }
  }

  async function testProvider(provider: LLMProvider) {
    setTestingProvider(provider.id);
    try {
      await api(`/llm-providers/${provider.id}/test`, { method: "POST" });
      push(t("settings.testOk"), "success");
    } catch (err) {
      push(`${t("settings.testFail")} ${apiErrorMessage(t, err)}`, "error");
    } finally {
      setTestingProvider(null);
    }
  }

  async function deleteProvider() {
    if (!deletingProvider) return;
    try {
      await api(`/llm-providers/${deletingProvider.id}`, { method: "DELETE" });
      await providers.reload();
      push(t("common.success"), "success");
      setDeletingProvider(null);
    } catch (err) {
      push(apiErrorMessage(t, err), "error");
    }
  }

  function openUser(target?: User) {
    if (target) {
      setEditingUser(target);
      setUserForm({
        email: target.email,
        displayName: target.displayName,
        role: target.role,
        locale: target.locale,
        password: "",
        disabled: Boolean(target.disabledAt),
      });
    } else {
      setEditingUser(null);
      setUserForm({
        email: "",
        displayName: "",
        role: "analyst",
        locale: locale as Locale,
        password: "",
        disabled: false,
      });
    }
    setFormError(null);
    setUserOpen(true);
  }

  async function saveUser(event: React.FormEvent) {
    event.preventDefault();
    setUserSaving(true);
    setFormError(null);
    try {
      if (editingUser) {
        await api(`/users/${editingUser.id}`, {
          method: "PATCH",
          body: {
            displayName: userForm.displayName,
            role: userForm.role,
            locale: userForm.locale,
            disabled: userForm.disabled,
          },
        });
      } else {
        await api("/users", {
          method: "POST",
          body: {
            email: userForm.email,
            displayName: userForm.displayName,
            role: userForm.role,
            locale: userForm.locale,
            password: userForm.password,
          },
        });
      }
      await users.reload();
      push(t("common.success"), "success");
      setUserOpen(false);
    } catch (err) {
      setFormError(apiErrorMessage(t, err));
    } finally {
      setUserSaving(false);
    }
  }

  async function resetUserPassword() {
    if (!resettingUser) return;
    setUserSaving(true);
    setFormError(null);
    try {
      await api(`/users/${resettingUser.id}/reset-password`, {
        method: "POST",
        body: { newPassword: resetPassword },
      });
      push(t("settings.passwordReset"), "success");
      setResettingUser(null);
      setResetPassword("");
    } catch (err) {
      setFormError(apiErrorMessage(t, err));
    } finally {
      setUserSaving(false);
    }
  }

  async function saveSystemSettings(event: React.FormEvent) {
    event.preventDefault();
    setSettingsSaving(true);
    setFormError(null);
    try {
      await api("/settings", {
        method: "PATCH",
        body: {
          items: [
            { key: "app.name", value: appName.trim() },
            { key: localeSettingKey, value: defaultLocale },
            { key: "report.disclaimer", value: disclaimer },
          ],
        },
      });
      await settings.reload();
      await system.reload();
      push(t("common.success"), "success");
    } catch (err) {
      setFormError(apiErrorMessage(t, err));
    } finally {
      setSettingsSaving(false);
    }
  }

  if (!user) return null;

  return (
    <div className="page-stack page-enter">
      <PageHeader title={t("settings.title")} subtitle={t("settings.subtitle")} badge={<RoleBadge role={user.role} label={t(`roles.${user.role}`)} />} />

      <div className="settings-layout">
        <aside className="settings-nav">
          <Tabs items={tabItems} value={tab} onChange={setTab} />
        </aside>

        <div className="settings-panels">
          {tab === "profile" ? (
            <Card title={t("settings.profile")} subtitle={t("settings.profileDesc")}>
              <form className="form-stack" onSubmit={saveProfile}>
                <TextField label={t("settings.displayName")} value={displayName} onChange={(event) => setDisplayName(event.target.value)} maxLength={100} />
                <SelectField label={t("settings.locale")} value={locale} onChange={(event) => setLocale(event.target.value as Locale)}>
                  <option value="en">{t("language.en")}</option>
                  <option value="ko">{t("language.ko")}</option>
                </SelectField>
                <div className="profile-email">
                  <span className="detail-label">{t("settings.email")}</span>
                  <code>{user.email}</code>
                </div>
                {formError ? <ErrorBanner message={formError} /> : null}
                <div className="form-actions">
                  <Button variant="primary" type="submit" loading={profileSaving}>{t("settings.saveProfile")}</Button>
                </div>
              </form>
            </Card>
          ) : null}

          {tab === "password" ? (
            <Card title={t("settings.password")} subtitle={t("settings.passwordDesc")}>
              <form className="form-stack" onSubmit={changePassword}>
                <TextField label={t("auth.currentPassword")} type="password" autoComplete="current-password" value={currentPassword} onChange={(event) => setCurrentPassword(event.target.value)} required />
                <TextField label={t("auth.newPassword")} type="password" autoComplete="new-password" value={newPassword} onChange={(event) => setNewPassword(event.target.value)} hint={t("setup.passwordHint")} required />
                <TextField label={t("auth.confirmNewPassword")} type="password" autoComplete="new-password" value={confirmPassword} onChange={(event) => setConfirmPassword(event.target.value)} required />
                {formError ? <ErrorBanner message={formError} /> : null}
                <div className="form-actions">
                  <Button variant="primary" type="submit" loading={passwordSaving} disabled={!currentPassword || !newPassword}>{t("settings.changePassword")}</Button>
                </div>
              </form>
            </Card>
          ) : null}

          {tab === "system" ? (
            <div className="settings-stack">
              <Card title={t("settings.system")} subtitle={t("settings.systemDesc")}>
                <dl className="definition-list">
                  <div><dt>{t("settings.safeLocalOnly")}</dt><dd><StatusBadge status={auth.safeLocalOnly ? "succeeded" : "awaiting_approval"} label={auth.safeLocalOnly ? t("common.enabled") : t("common.disabled")} /></dd></div>
                  <div><dt>{t("settings.safeModeSource")}</dt><dd><code>{auth.safeModeSource}</code></dd></div>
                  <div><dt>{t("settings.egressRules")}</dt><dd>{system.data?.egressRuleCount ?? 0}</dd></div>
                  <div><dt>{t("settings.maxConcurrentTasks")}</dt><dd>{auth.maxConcurrentTasks}</dd></div>
                  <div><dt>{t("settings.autoRun")}</dt><dd>{auth.autoRun ? t("common.enabled") : t("common.disabled")}</dd></div>
                  <div><dt>{t("nav.language")}</dt><dd>{(system.data?.supportedLocales ?? ["en", "ko"]).join(", ")}</dd></div>
                  <div><dt>{t("settings.appName")}</dt><dd>{system.data?.appName}</dd></div>
                  <div><dt>{t("common.details")}</dt><dd>{system.data?.version}</dd></div>
                </dl>
                <div className="banner banner-info">
                  <Icon name="shield" size={18} />
                  <span>{t("dashboard.safeModeExplain")}</span>
                </div>
              </Card>

              {isAdmin ? (
                <Card title={t("settings.instance")} subtitle={t("settings.instanceDesc")}>
                  <form className="form-stack" onSubmit={saveSystemSettings}>
                    <TextField label={t("settings.appName")} value={appName} onChange={(event) => setAppName(event.target.value)} maxLength={80} />
                    <SelectField label={t("settings.defaultLocale")} value={defaultLocale} onChange={(event) => setDefaultLocale(event.target.value as Locale)}>
                      <option value="en">{t("language.en")}</option>
                      <option value="ko">{t("language.ko")}</option>
                    </SelectField>
                    <TextField label={t("settings.reportDisclaimer")} value={disclaimer} onChange={(event) => setDisclaimer(event.target.value)} maxLength={2000} />
                    {formError ? <ErrorBanner message={formError} /> : null}
                    <div className="form-actions">
                      <Button variant="primary" type="submit" loading={settingsSaving}>{t("common.save")}</Button>
                    </div>
                  </form>
                </Card>
              ) : null}
            </div>
          ) : null}

          {tab === "providers" && isAdmin ? (
            <Card
              title={t("settings.providers")}
              subtitle={t("settings.providersDesc")}
              actions={<Button variant="primary" icon="plus" onClick={() => openProvider()}>{t("settings.addProvider")}</Button>}
              padded={false}
            >
              {providers.loading ? <div className="card-padded"><Skeleton lines={4} /></div> : null}
              {providers.error ? <div className="card-padded"><ErrorBanner message={t(`errors.${providers.error.code}`)} onRetry={providers.reload} retryLabel={t("common.retry")} /></div> : null}
              {!providers.loading && providers.data?.items.length === 0 ? (
                <EmptyState icon="server" title={t("settings.noProviders")} action={<Button variant="primary" icon="plus" onClick={() => openProvider()}>{t("settings.addProvider")}</Button>} />
              ) : (
                <div className="provider-list">
                  {(providers.data?.items ?? []).map((provider) => (
                    <div key={provider.id} className="provider-row">
                      <div className={`provider-icon provider-${provider.kind}`}>
                        <Icon name={provider.kind === "ollama" ? "server" : "globe"} size={19} />
                      </div>
                      <div className="provider-copy">
                        <div className="provider-title">
                          <strong>{provider.name}</strong>
                          {provider.isDefault ? <span className="badge badge-accent">{t("settings.defaultProvider")}</span> : null}
                          <span className={`badge ${provider.enabled ? "badge-success" : "badge-neutral"}`}>{provider.enabled ? t("common.enabled") : t("common.disabled")}</span>
                        </div>
                        <span className="provider-meta">
                          {provider.kind} · {provider.model} · {provider.baseUrl || "—"}
                        </span>
                        {provider.apiKeyEnv ? <code className="mono-chip">{provider.apiKeyEnv}</code> : null}
                      </div>
                      <div className="provider-actions">
                        <Button size="sm" variant="ghost" icon="activity" loading={testingProvider === provider.id} onClick={() => void testProvider(provider)}>
                          {t("settings.test")}
                        </Button>
                        <button type="button" className="icon-btn" aria-label={t("settings.editProvider")} onClick={() => openProvider(provider)}>
                          <Icon name="edit" size={16} />
                        </button>
                        <button type="button" className="icon-btn icon-btn-danger" aria-label={t("settings.deleteProvider")} onClick={() => setDeletingProvider(provider)} disabled={provider.isDefault}>
                          <Icon name="trash" size={16} />
                        </button>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </Card>
          ) : null}

          {tab === "users" && isAdmin ? (
            <Card
              title={t("settings.users")}
              subtitle={t("settings.usersDesc")}
              actions={<Button variant="primary" icon="plus" onClick={() => openUser()}>{t("settings.addUser")}</Button>}
              padded={false}
            >
              <div className="table-wrap">
                <table className="data-table">
                  <thead>
                    <tr>
                      <th>{t("settings.email")}</th>
                      <th>{t("settings.role")}</th>
                      <th>{t("settings.locale")}</th>
                      <th>{t("settings.disabled")}</th>
                      <th>{t("common.created")}</th>
                      <th aria-label={t("common.actions")} />
                    </tr>
                  </thead>
                  <tbody>
                    {(users.data?.items ?? []).map((item) => (
                      <tr key={item.id}>
                        <td>
                          <span className="table-title">{item.displayName}</span>
                          <span className="table-sub">{item.email}</span>
                        </td>
                        <td><RoleBadge role={item.role} label={t(`roles.${item.role}`)} /></td>
                        <td>{item.locale}</td>
                        <td>{item.disabledAt ? <span className="badge badge-neutral">{t("common.yes")}</span> : <span className="muted">{t("common.no")}</span>}</td>
                        <td className="muted">{formatDate(item.createdAt)}</td>
                        <td className="row-actions">
                          <button type="button" className="icon-btn" aria-label={t("common.edit")} onClick={() => openUser(item)}>
                            <Icon name="edit" size={15} />
                          </button>
                          <button type="button" className="icon-btn" aria-label={t("settings.resetPassword")} onClick={() => { setResettingUser(item); setResetPassword(""); setFormError(null); }}>
                            <Icon name="key" size={15} />
                          </button>
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <p className="card-footnote">{t("settings.lastAdminHint")}</p>
            </Card>
          ) : null}

          {tab === "audit" && isAdmin ? (
            <Card
              title={t("settings.audit")}
              subtitle={t("settings.auditDesc")}
              actions={<Button variant="ghost" size="sm" icon="refresh" onClick={audit.reload}>{t("common.refresh")}</Button>}
              padded={false}
            >
              {audit.loading ? <div className="card-padded"><Skeleton lines={6} /></div> : null}
              {!audit.loading && audit.data?.items.length === 0 ? <EmptyState icon="file" title={t("settings.noAudit")} /> : null}
              <div className="table-wrap">
                <table className="data-table">
                  <thead>
                    <tr>
                      <th>{t("settings.time")}</th>
                      <th>{t("settings.actor")}</th>
                      <th>{t("settings.action")}</th>
                      <th>{t("settings.target")}</th>
                      <th>{t("settings.result")}</th>
                    </tr>
                  </thead>
                  <tbody>
                    {(audit.data?.items ?? []).map((entry) => (
                      <tr key={entry.id}>
                        <td className="muted">{formatDate(entry.createdAt)}</td>
                        <td>{entry.actorEmail}</td>
                        <td><code className="mono-chip">{entry.action}</code></td>
                        <td className="muted">{entry.targetType}:{entry.targetId.slice(0, 8)}</td>
                        <td><StatusBadge status={entry.result === "success" ? "succeeded" : entry.result === "denied" ? "denied" : "failed"} label={t(`auditResult.${entry.result}`)} /></td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            </Card>
          ) : null}
        </div>
      </div>

      {providerOpen ? (
        <Drawer
          title={editingProvider ? t("settings.editProvider") : t("settings.addProvider")}
          description={t("settings.providersDesc")}
          onClose={() => setProviderOpen(false)}
          footer={
            <>
              <Button variant="ghost" onClick={() => setProviderOpen(false)}>{t("common.cancel")}</Button>
              <Button variant="primary" type="submit" form="provider-form" loading={providerSaving}>{t("common.save")}</Button>
            </>
          }
        >
          <form id="provider-form" className="form-stack" onSubmit={saveProvider}>
            <TextField label={t("settings.providerName")} value={providerForm.name} onChange={(event) => setProviderForm({ ...providerForm, name: event.target.value })} data-autofocus required />
            <SelectField
              label={t("settings.kind")}
              value={providerForm.kind}
              onChange={(event) => {
                const kind = event.target.value as "ollama" | "openai";
                setProviderForm({
                  ...providerForm,
                  kind,
                  baseUrl: kind === "ollama" ? "http://127.0.0.1:11434" : "https://api.openai.com/v1",
                  model: kind === "ollama" ? "llama3.1" : "gpt-4o-mini",
                  apiKeyEnv: kind === "openai" ? "LIP_LLM_API_KEY" : "",
                });
              }}
            >
              <option value="ollama">{t("settings.kindOllama")}</option>
              <option value="openai">{t("settings.kindOpenai")}</option>
            </SelectField>
            <TextField label={t("settings.baseUrl")} value={providerForm.baseUrl} onChange={(event) => setProviderForm({ ...providerForm, baseUrl: event.target.value })} hint={providerForm.kind === "ollama" ? t("settings.ollamaHint") : t("settings.openaiHint")} required />
            <TextField label={t("settings.model")} value={providerForm.model} onChange={(event) => setProviderForm({ ...providerForm, model: event.target.value })} required />
            {providerForm.kind === "openai" ? (
              <TextField label={t("settings.apiKeyEnv")} value={providerForm.apiKeyEnv} onChange={(event) => setProviderForm({ ...providerForm, apiKeyEnv: event.target.value })} hint={t("settings.apiKeyEnvHint")} required />
            ) : null}
            <CheckboxField label={t("settings.enabled")} checked={providerForm.enabled} onChange={(checked) => setProviderForm({ ...providerForm, enabled: checked })} />
            <CheckboxField label={t("settings.isDefault")} checked={providerForm.isDefault} onChange={(checked) => setProviderForm({ ...providerForm, isDefault: checked })} />
            {providerForm.kind === "openai" && auth.safeLocalOnly ? (
              <div className="banner banner-warning"><Icon name="lock" size={18} /><span>{t("errors.safe_mode_denied")}</span></div>
            ) : null}
            {formError ? <ErrorBanner message={formError} /> : null}
          </form>
        </Drawer>
      ) : null}

      {userOpen ? (
        <Drawer
          title={editingUser ? t("settings.editUser") : t("settings.addUser")}
          description={t("settings.usersDesc")}
          onClose={() => setUserOpen(false)}
          footer={
            <>
              <Button variant="ghost" onClick={() => setUserOpen(false)}>{t("common.cancel")}</Button>
              <Button variant="primary" type="submit" form="user-form" loading={userSaving}>{t("common.save")}</Button>
            </>
          }
        >
          <form id="user-form" className="form-stack" onSubmit={saveUser}>
            <TextField label={t("settings.email")} type="email" value={userForm.email} onChange={(event) => setUserForm({ ...userForm, email: event.target.value })} disabled={Boolean(editingUser)} data-autofocus required />
            <TextField label={t("settings.displayName")} value={userForm.displayName} onChange={(event) => setUserForm({ ...userForm, displayName: event.target.value })} maxLength={100} />
            <SelectField label={t("settings.role")} value={userForm.role} onChange={(event) => setUserForm({ ...userForm, role: event.target.value as Role })}>
              {roles.map((role) => <option key={role} value={role}>{t(`roles.${role}`)}</option>)}
            </SelectField>
            <SelectField label={t("settings.locale")} value={userForm.locale} onChange={(event) => setUserForm({ ...userForm, locale: event.target.value as Locale })}>
              <option value="en">{t("language.en")}</option>
              <option value="ko">{t("language.ko")}</option>
            </SelectField>
            {!editingUser ? (
              <TextField label={t("settings.tempPassword")} type="password" value={userForm.password} onChange={(event) => setUserForm({ ...userForm, password: event.target.value })} hint={t("setup.passwordHint")} required />
            ) : (
              <CheckboxField label={t("settings.disabled")} checked={userForm.disabled} onChange={(checked) => setUserForm({ ...userForm, disabled: checked })} hint={t("settings.lastAdminHint")} />
            )}
            {formError ? <ErrorBanner message={formError} /> : null}
          </form>
        </Drawer>
      ) : null}

      {resettingUser ? (
        <Modal
          title={t("settings.resetPassword")}
          description={t("settings.resetPasswordDesc")}
          onClose={() => setResettingUser(null)}
          footer={
            <>
              <Button variant="ghost" onClick={() => setResettingUser(null)}>{t("common.cancel")}</Button>
              <Button variant="primary" onClick={() => void resetUserPassword()} loading={userSaving} disabled={resetPassword.length < 12}>{t("common.save")}</Button>
            </>
          }
        >
          <TextField label={t("settings.newTempPassword")} type="password" value={resetPassword} onChange={(event) => setResetPassword(event.target.value)} hint={t("setup.passwordHint")} data-autofocus />
          {formError ? <ErrorBanner message={formError} /> : null}
        </Modal>
      ) : null}

      {deletingProvider ? (
        <Modal
          title={t("settings.deleteProvider")}
          description={t("common.confirmDelete")}
          onClose={() => setDeletingProvider(null)}
          footer={
            <>
              <Button variant="ghost" onClick={() => setDeletingProvider(null)}>{t("common.cancel")}</Button>
              <Button variant="danger" onClick={() => void deleteProvider()}>{t("common.delete")}</Button>
            </>
          }
        >
          <p className="modal-copy">{deletingProvider.name}</p>
        </Modal>
      ) : null}
    </div>
  );
}
