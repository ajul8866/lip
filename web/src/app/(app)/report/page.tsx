"use client";

import { useState } from "react";
import { reportUrl } from "@/lib/api";
import { useApi } from "@/lib/hooks";
import type { Asset, Finding, ListResponse, Task } from "@/lib/types";
import { useI18n } from "@/i18n";
import { useAuth } from "@/components/AuthProvider";
import { useProjects } from "@/components/ProjectProvider";
import { useToast } from "@/components/ToastProvider";
import {
  Button,
  Card,
  EmptyState,
  Icon,
  PageHeader,
  Skeleton,
  Stat,
} from "@/components/ui";

export default function ReportPage() {
  const { t, formatDate } = useI18n();
  const { safeLocalOnly } = useAuth();
  const { selected, loading: projectLoading } = useProjects();
  const { push } = useToast();
  const [downloading, setDownloading] = useState(false);
  const findings = useApi<ListResponse<Finding>>(selected ? `/projects/${selected.id}/findings?limit=1` : null);
  const assets = useApi<ListResponse<Asset>>(selected ? `/projects/${selected.id}/assets?limit=1` : null);
  const tasks = useApi<ListResponse<Task>>(selected ? `/projects/${selected.id}/tasks?limit=1` : null);

  if (projectLoading) return <Skeleton lines={5} />;
  if (!selected) {
    return <Card><EmptyState icon="folder" title={t("projects.empty")} body={t("projects.emptyDesc")} /></Card>;
  }

  return (
    <div className="page-stack page-enter">
      <PageHeader
        title={t("report.title")}
        subtitle={t("report.subtitle")}
        badge={<span className="badge badge-neutral">{selected.name}</span>}
        actions={
          <a
            className="btn btn-primary"
            href={reportUrl(selected.id)}
            onClick={() => {
              setDownloading(true);
              push(t("report.download"), "info");
              window.setTimeout(() => setDownloading(false), 1200);
            }}
          >
            <Icon name={downloading ? "clock" : "download"} size={16} />
            <span>{t("report.download")}</span>
          </a>
        }
      />

      <div className="stat-grid">
        <Stat label={t("report.findingsCount")} value={findings.data?.total ?? 0} icon="findings" tone="accent" />
        <Stat label={t("report.assetsCount")} value={assets.data?.total ?? 0} icon="assets" />
        <Stat label={t("dashboard.tasks")} value={tasks.data?.total ?? 0} icon="tasks" />
        <Stat
          label={t("settings.safeLocalOnly")}
          value={safeLocalOnly ? t("common.enabled") : t("common.disabled")}
          icon={safeLocalOnly ? "lock" : "globe"}
          tone={safeLocalOnly ? "success" : "warning"}
        />
      </div>

      <div className="two-col">
        <Card title={t("report.contents")}>
          <ul className="check-list">
            <li><Icon name="check" size={16} />{t("report.assetList")}</li>
            <li><Icon name="check" size={16} />{t("report.findingList")}</li>
            <li><Icon name="check" size={16} />{t("report.taskList")}</li>
            <li><Icon name="check" size={16} />{t("report.approvalList")}</li>
            <li><Icon name="check" size={16} />{t("report.limitations")}</li>
          </ul>
          <p className="muted report-hint">{t("report.hint")}</p>
        </Card>
        <Card title={t("report.generated")}>
          <div className="report-preview" aria-hidden="true">
            <div className="report-line report-line-title" />
            <div className="report-line" />
            <div className="report-line short" />
            <div className="report-line" />
            <div className="report-line mid" />
            <div className="report-line short" />
            <div className="report-line" />
          </div>
          <div className="report-meta">
            <span>{t("report.ready")}</span>
            <strong>{formatDate(new Date(), { dateStyle: "long" })}</strong>
          </div>
          <Button variant="primary" icon="download" onClick={() => window.location.assign(reportUrl(selected.id))}>
            {t("report.download")}
          </Button>
        </Card>
      </div>
    </div>
  );
}
