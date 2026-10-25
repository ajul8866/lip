"use client";

import Link from "next/link";
import { usePathname, useRouter } from "next/navigation";
import { useEffect, useMemo, useState } from "react";
import { api, ApiError } from "@/lib/api";
import type { SystemStatus } from "@/lib/types";
import { useI18n } from "@/i18n";
import { useApi } from "@/lib/hooks";
import { useAuth } from "./AuthProvider";
import { useProjects } from "./ProjectProvider";
import { useTheme } from "./ThemeProvider";
import { useToast } from "./ToastProvider";
import { ErrorBanner, Icon, RoleBadge, Skeleton, type IconName } from "./ui";

const navItems: { href: string; key: string; icon: IconName }[] = [
  { href: "/dashboard", key: "nav.dashboard", icon: "dashboard" },
  { href: "/projects", key: "nav.projects", icon: "folder" },
  { href: "/conversations", key: "nav.conversations", icon: "chat" },
  { href: "/tasks", key: "nav.tasks", icon: "tasks" },
  { href: "/findings", key: "nav.findings", icon: "findings" },
  { href: "/assets", key: "nav.assets", icon: "assets" },
  { href: "/report", key: "nav.report", icon: "report" },
  { href: "/settings", key: "nav.settings", icon: "settings" },
];

function initials(name: string): string {
  const parts = name.trim().split(/\s+/).filter(Boolean);
  if (parts.length === 0) return "L";
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase();
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
}

export function AppShell({ children }: { children: React.ReactNode }) {
  const { t, locale, setLocale, formatDate } = useI18n();
  const { theme, toggleTheme } = useTheme();
  const { status, user, sessionId, safeLocalOnly, maxConcurrentTasks, autoRun, error, refresh, logout, setUser } = useAuth();
  const { projects, selectedId, select, loading: projectsLoading, error: projectsError, reload: reloadProjects } = useProjects();
  const { push } = useToast();
  const router = useRouter();
  const pathname = usePathname();
  const [mobileOpen, setMobileOpen] = useState(false);
  const [switchingLocale, setSwitchingLocale] = useState(false);
  const system = useApi<SystemStatus>("/system/status");

  useEffect(() => {
    if (status === "anonymous") router.replace("/login");
  }, [status, router]);

  useEffect(() => {
    setMobileOpen(false);
  }, [pathname]);

  useEffect(() => {
    const key = navItems.find((item) => pathname === item.href || pathname.startsWith(`${item.href}/`))?.key ?? "app.name";
    const appName = system.data?.appName ?? t("app.name");
    document.title = `${t(key)} · ${appName}`;
  }, [pathname, system.data?.appName, t]);

  const pageTitle = useMemo(() => {
    const key = navItems.find((item) => pathname === item.href || pathname.startsWith(`${item.href}/`))?.key ?? "app.name";
    return t(key);
  }, [pathname, t]);

  async function changeLocale(next: "en" | "ko") {
    if (next === locale) return;
    setSwitchingLocale(true);
    setLocale(next);
    try {
      if (user) {
        const response = await api<{ user: typeof user }>("/auth/me", {
          method: "PATCH",
          body: { locale: next },
        });
        setUser(response.user);
      }

    } catch (err) {
      push(err instanceof ApiError ? t(`errors.${err.code}`) : t("errors.unknown"), "error");
      await refresh();
    } finally {
      setSwitchingLocale(false);
    }
  }

  async function signOut() {
    await logout();
    router.replace("/login");
  }

  if (status === "loading") {
    return (
      <div className="boot-screen">
        <div className="brand-mark brand-mark-lg">
          <Icon name="shield" size={28} />
        </div>
        <p>{t("boot.loading")}</p>
        <Skeleton lines={2} className="boot-skeleton" />
      </div>
    );
  }
  if (status === "error") {
    return (
      <div className="boot-screen">
        <div className="brand-mark brand-mark-lg">
          <Icon name="alert" size={28} />
        </div>
        <p>{t("boot.error")}</p>
        <ErrorBanner message={error?.code ? t(`errors.${error.code}`) : t("errors.unknown")} onRetry={() => void refresh()} retryLabel={t("common.retry")} />
      </div>
    );
  }
  if (!user || status === "anonymous") {
    return <div className="boot-screen"><span className="spinner" /></div>;
  }

  return (
    <div className="app-shell">
      <aside className={`sidebar ${mobileOpen ? "is-open" : ""}`}>
        <div className="sidebar-brand">
          <span className="brand-mark">
            <Icon name="shield" size={20} />
          </span>
          <div>
            <span className="brand-name">{system.data?.appName ?? t("app.name")}</span>
            <span className="brand-tagline">{t("app.tagline")}</span>
          </div>
        </div>
        <nav className="sidebar-nav" aria-label={t("common.actions")}>
          <span className="nav-label">{t("nav.workspace")}</span>
          {navItems.map((item) => {
            const active = pathname === item.href || pathname.startsWith(`${item.href}/`);
            return (
              <Link key={item.href} href={item.href} className={`nav-item ${active ? "is-active" : ""}`} aria-current={active ? "page" : undefined}>
                <Icon name={item.icon} size={18} />
                <span>{t(item.key)}</span>
              </Link>
            );
          })}
        </nav>
        <div className="sidebar-foot">
          <div className={`safe-chip ${safeLocalOnly ? "is-on" : "is-off"}`}>
            <Icon name={safeLocalOnly ? "lock" : "globe"} size={15} />
            <span>{safeLocalOnly ? t("nav.safeMode") : t("nav.standardMode")}</span>
          </div>
          <div className="sidebar-meta">
            <span>{system.data?.version ? (system.data.version === "dev" ? "dev" : `v${system.data.version}`) : ""}</span>
            <span>{system.data?.startedAt ? formatDate(system.data.startedAt, { dateStyle: "medium" }) : ""}</span>
          </div>
        </div>
      </aside>
      {mobileOpen ? <button type="button" className="sidebar-scrim" aria-label={t("common.close")} onClick={() => setMobileOpen(false)} /> : null}

      <div className="app-content">
        <header className="topbar">
          <div className="topbar-left">
            <button type="button" className="icon-btn mobile-only" onClick={() => setMobileOpen(true)} aria-label={t("common.open")}>
              <Icon name="menu" size={20} />
            </button>
            <div className="topbar-title">
              <span className="topbar-eyebrow">{t("app.tagline")}</span>
              <strong>{pageTitle}</strong>
            </div>
          </div>
          <div className="topbar-right">
            <label className="project-switch">
              <span className="sr-only">{t("projects.select")}</span>
              <select
                value={selectedId ?? ""}
                onChange={(event) => select(event.target.value)}
                disabled={projectsLoading || projects.length === 0}
                aria-label={t("projects.select")}
              >
                {projects.length === 0 ? <option value="">{t("projects.empty")}</option> : null}
                {projects.map((project) => (
                  <option key={project.id} value={project.id}>
                    {project.name}
                  </option>
                ))}
              </select>
              <Icon name="chevronDown" size={15} />
            </label>
            <div className="segmented" role="group" aria-label={t("nav.language")}>
              <button type="button" className={locale === "en" ? "is-active" : ""} onClick={() => void changeLocale("en")} disabled={switchingLocale}>
                EN
              </button>
              <button type="button" className={locale === "ko" ? "is-active" : ""} onClick={() => void changeLocale("ko")} disabled={switchingLocale}>
                KO
              </button>
            </div>
            <button type="button" className="icon-btn" onClick={toggleTheme} aria-label={theme === "dark" ? t("theme.light") : t("theme.dark")}>
              <Icon name={theme === "dark" ? "sun" : "moon"} size={18} />
            </button>
            <details className="user-menu">
              <summary>
                <span className="avatar">{initials(user.displayName || user.email)}</span>
                <span className="user-menu-copy">
                  <span className="user-name">{user.displayName}</span>
                  <span className="user-role">{t(`roles.${user.role}`)}</span>
                </span>
                <Icon name="chevronDown" size={14} />
              </summary>
              <div className="user-popover">
                <div className="user-popover-head">
                  <span className="avatar avatar-lg">{initials(user.displayName || user.email)}</span>
                  <div>
                    <strong>{user.displayName}</strong>
                    <span>{user.email}</span>
                    <RoleBadge role={user.role} label={t(`roles.${user.role}`)} />
                  </div>
                </div>
                <Link href="/settings" className="popover-item">
                  <Icon name="settings" size={16} /> {t("nav.settings")}
                </Link>
                <button type="button" className="popover-item" onClick={() => void signOut()}>
                  <Icon name="logout" size={16} /> {t("nav.logout")}
                </button>
                <div className="popover-meta">
                  <span>{t("settings.session")}</span>
                  <code>{sessionId?.slice(0, 8)}</code>
                </div>
              </div>
            </details>
          </div>
        </header>

        {projectsError ? (
          <div className="content-banner">
            <ErrorBanner
              message={t(`errors.${projectsError.code}`)}
              onRetry={() => void reloadProjects()}
              retryLabel={t("common.retry")}
            />
          </div>
        ) : null}
        {user.mustChangePassword ? (
          <div className="content-banner">
            <div className="banner banner-warning">
              <Icon name="key" size={18} />
              <span>{t("auth.mustChangeBanner")}</span>
              <Link href="/settings#password" className="banner-action">
                {t("auth.changePassword")}
              </Link>
            </div>
          </div>
        ) : null}

        <main className="main">{children}</main>
        <footer className="app-footer">
          <span>{t("app.description")}</span>
          <span className="app-footer-meta">
            {safeLocalOnly ? t("nav.safeMode") : t("nav.standardMode")} · {maxConcurrentTasks} · {autoRun ? t("common.enabled") : t("common.disabled")}
          </span>
        </footer>
      </div>
    </div>
  );
}
