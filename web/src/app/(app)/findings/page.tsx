"use client";

import { useMemo, useState } from "react";
import { api } from "@/lib/api";
import { apiErrorMessage } from "@/lib/errors";
import { useApi } from "@/lib/hooks";
import type { Finding, FindingStatus, ListResponse, Severity } from "@/lib/types";
import { useI18n } from "@/i18n";
import { useAuth } from "@/components/AuthProvider";
import { useProjects } from "@/components/ProjectProvider";
import { useToast } from "@/components/ToastProvider";
import { Drawer, Modal } from "@/components/Panel";
import {
  Button,
  Card,
  EmptyState,
  ErrorBanner,
  FindingStatusBadge,
  Icon,
  PageHeader,
  SelectField,
  SeverityBadge,
  Skeleton,
  TextAreaField,
  TextField,
} from "@/components/ui";

const severities: Severity[] = ["critical", "high", "medium", "low", "info"];
const statuses: FindingStatus[] = ["open", "triaged", "accepted", "fixed", "false_positive"];

interface FindingForm {
  title: string;
  description: string;
  severity: Severity;
  status: FindingStatus;
  evidence: string;
}

const emptyForm: FindingForm = {
  title: "",
  description: "",
  severity: "medium",
  status: "open",
  evidence: "{}",
};

export default function FindingsPage() {
  const { t, formatDate } = useI18n();
  const { user } = useAuth();
  const { selected, loading: projectLoading } = useProjects();
  const { push } = useToast();
  const [severityFilter, setSeverityFilter] = useState("");
  const [statusFilter, setStatusFilter] = useState("");
  const path = selected
    ? `/projects/${selected.id}/findings?limit=200${severityFilter ? `&severity=${severityFilter}` : ""}${statusFilter ? `&status=${statusFilter}` : ""}`
    : null;
  const findings = useApi<ListResponse<Finding>>(path);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<Finding | null>(null);
  const [form, setForm] = useState<FindingForm>(emptyForm);
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<Finding | null>(null);

  const canWrite = (user?.role === "admin" || user?.role === "analyst") && !user.mustChangePassword;
  const grouped = useMemo(() => {
    const map = new Map<Severity, number>();
    for (const finding of findings.data?.items ?? []) {
      map.set(finding.severity, (map.get(finding.severity) ?? 0) + 1);
    }
    return map;
  }, [findings.data]);

  function openCreate() {
    setForm(emptyForm);
    setFormError(null);
    setCreating(true);
  }

  function openEdit(finding: Finding) {
    setForm({
      title: finding.title,
      description: finding.description,
      severity: finding.severity,
      status: finding.status,
      evidence: JSON.stringify(finding.evidence ?? {}, null, 2),
    });
    setFormError(null);
    setEditing(finding);
  }

  async function submit(event: React.FormEvent) {
    event.preventDefault();
    if (!selected) return;
    let evidence: unknown;
    try {
      evidence = JSON.parse(form.evidence || "{}");
    } catch {
      setFormError(t("common.invalidJson"));
      return;
    }
    setSaving(true);
    setFormError(null);
    try {
      if (editing) {
        await api(`/findings/${editing.id}`, {
          method: "PATCH",
          body: { ...form, evidence },
        });
        push(t("findings.updated"), "success");
      } else {
        await api(`/projects/${selected.id}/findings`, {
          method: "POST",
          body: { ...form, evidence },
        });
        push(t("common.success"), "success");
      }
      await findings.reload();
      setCreating(false);
      setEditing(null);
      setForm(emptyForm);
    } catch (err) {
      setFormError(apiErrorMessage(t, err));
    } finally {
      setSaving(false);
    }
  }

  async function deleteFinding() {
    if (!deleting) return;
    try {
      await api(`/findings/${deleting.id}`, { method: "DELETE" });
      await findings.reload();
      push(t("common.success"), "success");
      setDeleting(null);
    } catch (err) {
      push(apiErrorMessage(t, err), "error");
    }
  }

  if (projectLoading) return <Skeleton lines={5} />;
  if (!selected) {
    return <Card><EmptyState icon="folder" title={t("projects.empty")} body={t("projects.emptyDesc")} /></Card>;
  }


  return (
    <div className="page-stack page-enter">
      <PageHeader
        title={t("findings.title")}
        subtitle={t("findings.subtitle")}
        badge={<span className="badge badge-neutral">{selected.name}</span>}
        actions={canWrite ? <Button variant="primary" icon="plus" onClick={openCreate}>{t("findings.new")}</Button> : undefined}
      />

      <div className="severity-strip">
        {severities.map((severity) => (
          <div key={severity} className={`severity-cell severity-${severity}`}>
            <span>{t(`severity.${severity}`)}</span>
            <strong>{grouped.get(severity) ?? 0}</strong>
          </div>
        ))}
      </div>

      <div className="filter-row">
        <div className="filter-label"><Icon name="filter" size={15} /> {t("tasks.filters")}</div>
        <SelectField label={t("findings.filterSeverity")} value={severityFilter} onChange={(event) => setSeverityFilter(event.target.value)}>
          <option value="">{t("common.all")}</option>
          {severities.map((severity) => <option key={severity} value={severity}>{t(`severity.${severity}`)}</option>)}
        </SelectField>
        <SelectField label={t("findings.filterStatus")} value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)}>
          <option value="">{t("common.all")}</option>
          {statuses.map((status) => <option key={status} value={status}>{t(`status.${status}`)}</option>)}
        </SelectField>
        <span className="filter-count">{t("common.total")}: {findings.data?.total ?? 0}</span>
        <Button size="sm" variant="ghost" icon="refresh" onClick={findings.reload}>{t("common.refresh")}</Button>
      </div>

      {findings.error ? <ErrorBanner message={t(`errors.${findings.error.code}`)} onRetry={findings.reload} retryLabel={t("common.retry")} /> : null}

      <Card padded={false}>
        {findings.loading ? <div className="card-padded"><Skeleton lines={6} /></div> : null}
        {!findings.loading && findings.data?.items.length === 0 ? (
          <EmptyState
            icon="findings"
            title={t("findings.empty")}
            body={t("findings.emptyDesc")}
            action={canWrite ? <Button variant="primary" icon="plus" onClick={openCreate}>{t("findings.new")}</Button> : undefined}
          />
        ) : (
          !findings.loading && (
            <div className="table-wrap">
              <table className="data-table">
                <thead>
                  <tr>
                    <th>{t("findings.severity")}</th>
                    <th>{t("findings.titleField")}</th>
                    <th>{t("findings.status")}</th>
                    <th>{t("findings.source")}</th>
                    <th>{t("common.updated")}</th>
                    <th aria-label={t("common.actions")} />
                  </tr>
                </thead>
                <tbody>
                  {(findings.data?.items ?? []).map((finding) => (
                    <tr key={finding.id}>
                      <td><SeverityBadge severity={finding.severity} label={t(`severity.${finding.severity}`)} /></td>
                      <td>
                        <button type="button" className="table-title" onClick={() => openEdit(finding)}>{finding.title}</button>
                        {finding.description ? <span className="table-sub">{finding.description.slice(0, 120)}</span> : null}
                      </td>
                      <td><span className="badge badge-neutral">{t(finding.source === "llm" ? "findings.sourceLlm" : finding.source === "import" ? "findings.sourceImport" : "findings.sourceManual")}</span></td>
                      <td className="muted">{formatDate(finding.updatedAt)}</td>
                      <td className="row-actions">
                        <button type="button" className="icon-btn" aria-label={t("common.edit")} onClick={() => openEdit(finding)}>
                          <Icon name="edit" size={15} />
                        </button>
                        {canWrite ? (
                          <button type="button" className="icon-btn icon-btn-danger" aria-label={t("common.delete")} onClick={() => setDeleting(finding)}>
                            <Icon name="trash" size={15} />
                          </button>
                        ) : null}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )
        )}
      </Card>

      {(creating || editing) ? (
        <Drawer
          title={editing ? t("findings.editTitle") : t("findings.newTitle")}
          description={t("findings.subtitle")}
          size="lg"
          onClose={() => { setCreating(false); setEditing(null); }}
          footer={
            <>
              <Button variant="ghost" onClick={() => { setCreating(false); setEditing(null); }}>{t("common.cancel")}</Button>
              <Button variant="primary" type="submit" form="finding-form" loading={saving} disabled={!form.title.trim()}>
                {editing ? t("common.save") : t("findings.create")}
              </Button>
            </>
          }
        >
          <form id="finding-form" className="form-stack" onSubmit={submit}>
            <TextField
              label={t("findings.titleField")}
              value={form.title}
              onChange={(event) => setForm({ ...form, title: event.target.value })}
              maxLength={200}
              data-autofocus
              required
            />
            <TextAreaField
              label={t("findings.description")}
              value={form.description}
              onChange={(event) => setForm({ ...form, description: event.target.value })}
              rows={6}
              maxLength={8000}
            />
            <div className="form-grid">
              <SelectField label={t("findings.severity")} value={form.severity} onChange={(event) => setForm({ ...form, severity: event.target.value as Severity })}>
                {severities.map((severity) => <option key={severity} value={severity}>{t(`severity.${severity}`)}</option>)}
              </SelectField>
              <SelectField label={t("findings.status")} value={form.status} onChange={(event) => setForm({ ...form, status: event.target.value as FindingStatus })}>
                {statuses.map((status) => <option key={status} value={status}>{t(`status.${status}`)}</option>)}
              </SelectField>
            </div>
            <TextAreaField
              label={t("findings.evidence")}
              value={form.evidence}
              onChange={(event) => setForm({ ...form, evidence: event.target.value })}
              rows={8}
              hint={t("findings.evidenceHint")}
              className="code-field"
            />
            {formError ? <ErrorBanner message={formError} /> : null}
          </form>
        </Drawer>
      ) : null}

      {deleting ? (
        <Modal
          title={t("findings.delete")}
          description={t("findings.deleteConfirm")}
          onClose={() => setDeleting(null)}
          footer={
            <>
              <Button variant="ghost" onClick={() => setDeleting(null)}>{t("common.cancel")}</Button>
              <Button variant="danger" onClick={() => void deleteFinding()}>{t("common.delete")}</Button>
            </>
          }
        >
          <p className="modal-copy">{deleting.title}</p>
        </Modal>
      ) : null}
    </div>
  );
}
