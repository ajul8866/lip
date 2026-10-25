"use client";

import Link from "next/link";
import { useApi } from "@/lib/hooks";
import type { Dashboard } from "@/lib/types";
import { useI18n } from "@/i18n";
import { useAuth } from "@/components/AuthProvider";
import {
  Card,
  EmptyState,
  ErrorBanner,
  Icon,
  PageHeader,
  SeverityBadge,
  Skeleton,
  Stat,
  StatusBadge,
} from "@/components/ui";

interface DashboardResponse {
  dashboard: Dashboard;
  safeLocalOnly: boolean;
  maxConcurrentTasks: number;
  autoRun: boolean;
}

export default function DashboardPage() {
  const { t, formatDate, formatNumber } = useI18n();
  const { safeLocalOnly, maxConcurrentTasks, autoRun } = useAuth();
  const { data, error, loading, reload } = useApi<DashboardResponse>("/dashboard");

  if (loading) {
    return (
      <div className="page-stack">
        <Skeleton lines={2} />
        <div className="stat-grid">
          {Array.from({ length: 4 }).map((_, index) => (
            <div className="stat" key={index}><Skeleton lines={2} /></div>
          ))}
        </div>
        <div className="two-col">
          <Card><Skeleton lines={5} /></Card>
          <Card><Skeleton lines={5} /></Card>
        </div>
      </div>
    );
  }
  if (error || !data) {
    return (
      <ErrorBanner message={t(`errors.${error?.code ?? "unknown"}`)} onRetry={reload} retryLabel={t("common.retry")} />
    );
  }
  const d = data.dashboard;

  return (
    <div className="page-stack page-enter">
      <PageHeader
        title={t("dashboard.title")}
        subtitle={t("dashboard.subtitle")}
        actions={
          <>
            <Link href="/conversations" className="btn btn-secondary btn-sm">
              <Icon name="chat" size={16} /> {t("conversations.title")}
            </Link>
            <Link href="/report" className="btn btn-primary btn-sm">
              <Icon name="report" size={16} /> {t("report.title")}
            </Link>
          </>
        }
      />

      <section className={`safe-banner ${safeLocalOnly ? "is-safe" : "is-open"}`}>
        <div className="safe-banner-icon">
          <Icon name={safeLocalOnly ? "lock" : "globe"} size={20} />
        </div>
        <div>
          <strong>{safeLocalOnly ? t("dashboard.safeModeOn") : t("dashboard.safeModeOff")}</strong>
          <p>{t("dashboard.safeModeExplain")}</p>
        </div>
        <div className="safe-banner-meta">
          <span>{t("dashboard.autonomyExplain", { count: maxConcurrentTasks })}</span>
          <span className={`badge ${autoRun ? "badge-success" : "badge-neutral"}`}>
            {autoRun ? t("common.enabled") : t("common.disabled")}
          </span>
        </div>
      </section>

      <div className="stat-grid">
        <Stat label={t("dashboard.projects")} value={formatNumber(d.projects)} icon="folder" tone="accent" />
        <Stat
          label={t("dashboard.tasks")}
          value={formatNumber(d.tasks)}
          hint={`${t("dashboard.running")}: ${d.runningTasks} · ${t("dashboard.awaitingApproval")}: ${d.awaitingTasks}`}
          icon="tasks"
        />
        <Stat
          label={t("dashboard.openFindings")}
          value={formatNumber(d.openFindings)}
          hint={`${t("dashboard.critical")}: ${d.criticalOpen} · ${t("dashboard.high")}: ${d.highOpen}`}
          icon="findings"
          tone={d.criticalOpen > 0 ? "danger" : d.highOpen > 0 ? "warning" : "success"}
        />
        <Stat label={t("dashboard.assets")} value={formatNumber(d.assets)} icon="assets" />
      </div>

      <div className="two-col">
        <Card
          title={t("dashboard.recentFindings")}
          actions={<Link href="/findings" className="text-link">{t("dashboard.viewAll")} <Icon name="arrowRight" size={14} /></Link>}
          padded={false}
        >
          {d.recentFindings.length === 0 ? (
            <EmptyState title={t("dashboard.emptyFindings")} icon="findings" />
          ) : (
            <ul className="feed">
              {d.recentFindings.map((finding) => (
                <li key={finding.id} className="feed-item">
                  <SeverityBadge severity={finding.severity} label={t(`severity.${finding.severity}`)} />
                  <div className="feed-copy">
                    <Link href="/findings" className="feed-title">{finding.title}</Link>
                    <span className="feed-meta">
                      {t(`status.${finding.status}`)} · {formatDate(finding.createdAt)}
                    </span>
                  </div>
                  <span className="badge badge-neutral">{t(`findings.source${finding.source === "llm" ? "Llm" : finding.source === "import" ? "Import" : "Manual"}`)}</span>
                </li>
              ))}
            </ul>
          )}
        </Card>

        <Card
          title={t("dashboard.recentTasks")}
          actions={<Link href="/tasks" className="text-link">{t("dashboard.viewAll")} <Icon name="arrowRight" size={14} /></Link>}
          padded={false}
        >
          {d.recentTasks.length === 0 ? (
            <EmptyState title={t("dashboard.emptyTasks")} icon="tasks" />
          ) : (
            <ul className="feed">
              {d.recentTasks.map((task) => (
                <li key={task.id} className="feed-item">
                  <StatusBadge status={task.status} label={t(`status.${task.status}`)} />
                  <div className="feed-copy">
                    <Link href="/tasks" className="feed-title">{task.title}</Link>
                    <span className="feed-meta">
                      {t(`risk.${task.riskLevel}`)} · {formatDate(task.createdAt)}
                    </span>
                  </div>
                  <code className="mono-chip">{task.tool}</code>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>

      <Card title={t("dashboard.quickStart")} subtitle={t("dashboard.quickStartDesc")} className="quick-start">
        <div className="quick-grid">
          <Link href="/projects" className="quick-step">
            <span className="quick-number">1</span>
            <Icon name="folder" size={20} />
            <strong>{t("nav.projects")}</strong>
            <span>{t("projects.subtitle")}</span>
          </Link>
          <Link href="/assets" className="quick-step">
            <span className="quick-number">2</span>
            <Icon name="assets" size={20} />
            <strong>{t("assets.title")}</strong>
            <span>{t("assets.subtitle")}</span>
          </Link>
          <Link href="/conversations" className="quick-step">
            <span className="quick-number">3</span>
            <Icon name="chat" size={20} />
            <strong>{t("conversations.title")}</strong>
            <span>{t("conversations.subtitle")}</span>
          </Link>
          <Link href="/report" className="quick-step">
            <span className="quick-number">4</span>
            <Icon name="report" size={20} />
            <strong>{t("report.title")}</strong>
            <span>{t("report.subtitle")}</span>
          </Link>
        </div>
      </Card>
    </div>
  );
}
