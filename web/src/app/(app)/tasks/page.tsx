"use client";

import { useMemo, useState } from "react";
import { api } from "@/lib/api";
import { apiErrorMessage } from "@/lib/errors";
import { useApi, useInterval } from "@/lib/hooks";
import type { Approval, Conversation, ListResponse, Task, TaskStatus, ToolDefinition } from "@/lib/types";
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
  Icon,
  JsonBlock,
  PageHeader,
  RiskBadge,
  SelectField,
  Skeleton,
  StatusBadge,
  TextAreaField,
  TextField,
} from "@/components/ui";

const statuses: TaskStatus[] = ["queued", "awaiting_approval", "running", "succeeded", "failed", "cancelled", "blocked"];

export default function TasksPage() {
  const { t, formatDate } = useI18n();
  const { user } = useAuth();
  const canWrite = user?.role !== "viewer" && !user?.mustChangePassword;
  const { selected, loading: projectLoading } = useProjects();
  const { push } = useToast();
  const [statusFilter, setStatusFilter] = useState("");
  const taskPath = selected ? `/projects/${selected.id}/tasks?limit=200${statusFilter ? `&status=${statusFilter}` : ""}` : null;
  const tasks = useApi<ListResponse<Task>>(taskPath);
  const approvals = useApi<ListResponse<Approval>>(selected ? `/projects/${selected.id}/approvals?status=pending&limit=100` : null);
  const tools = useApi<{ items: ToolDefinition[] }>("/meta/tools");
  const conversations = useApi<ListResponse<Conversation>>(selected ? `/projects/${selected.id}/conversations?limit=100` : null);
  const [creating, setCreating] = useState(false);
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [tool, setTool] = useState("inventory_summary");
  const [parameters, setParameters] = useState("{}");
  const [conversationId, setConversationId] = useState("");
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [viewing, setViewing] = useState<Task | null>(null);
  const [deciding, setDeciding] = useState<{ approval: Approval; decision: "approve" | "deny" } | null>(null);
  const [decisionReason, setDecisionReason] = useState("");
  const [cancelling, setCancelling] = useState<Task | null>(null);

  useInterval(() => {
    void tasks.reload();
    void approvals.reload();
  }, 5000);


  const taskById = useMemo(() => new Map((tasks.data?.items ?? []).map((task) => [task.id, task])), [tasks.data]);

  function selectTool(name: string) {
    setTool(name);
    if (name === "http_head") setParameters('{\n  "url": ""\n}');
    else setParameters("{}");
  }

  async function createTask(event: React.FormEvent) {
    event.preventDefault();
    if (!selected) return;
    let parsed: unknown;
    try {
      parsed = JSON.parse(parameters || "{}");
    } catch {
      setFormError(t("tasks.invalidJson"));
      return;
    }
    setSaving(true);
    setFormError(null);
    try {
      const response = await api<{ task: Task; approval?: Approval }>(`/projects/${selected.id}/tasks`, {
        method: "POST",
        body: {
          title: title.trim(),
          description: description.trim(),
          tool,
          parameters: parsed,
          conversationId: conversationId || undefined,
        },
      });
      await tasks.reload();
      await approvals.reload();
      setCreating(false);
      setTitle("");
      setDescription("");
      setConversationId("");
      selectTool("inventory_summary");
      push(t(response.approval ? "tasks.createdAwaiting" : "tasks.createdToast", { title: response.task.title }), "success");
    } catch (err) {
      setFormError(apiErrorMessage(t, err));
    } finally {
      setSaving(false);
    }
  }

  async function decide() {
    if (!deciding) return;
    if (deciding.decision === "deny" && decisionReason.trim().length < 3) {
      setFormError(t("tasks.denyReason"));
      return;
    }
    setSaving(true);
    setFormError(null);
    try {
      await api(`/approvals/${deciding.approval.id}/decide`, {
        method: "POST",
        body: { decision: deciding.decision, reason: decisionReason.trim() },
      });
      await Promise.all([tasks.reload(), approvals.reload()]);
      push(t(deciding.decision === "approve" ? "tasks.approved" : "tasks.denied"), "success");
      setDeciding(null);
      setDecisionReason("");
    } catch (err) {
      setFormError(apiErrorMessage(t, err));
    } finally {
      setSaving(false);
    }
  }

  async function cancelTask() {
    if (!cancelling) return;
    try {
      await api(`/tasks/${cancelling.id}/cancel`, { method: "POST" });
      await Promise.all([tasks.reload(), approvals.reload()]);
      push(t("common.success"), "success");
      setCancelling(null);
    } catch (err) {
      push(apiErrorMessage(t, err), "error");
    }
  }

  if (projectLoading) return <Skeleton lines={5} />;
  if (!selected) {
    return <Card><EmptyState icon="folder" title={t("projects.empty")} body={t("projects.emptyDesc")} /></Card>;
  }

  const pending = approvals.data?.items ?? [];

  return (
    <div className="page-stack page-enter">
      <PageHeader
        title={t("tasks.title")}
        subtitle={t("tasks.subtitle")}
        badge={<span className="badge badge-neutral">{selected.name}</span>}
        actions={canWrite ? <Button variant="primary" icon="plus" onClick={() => setCreating(true)}>{t("tasks.new")}</Button> : undefined}
      />

      {pending.length > 0 ? (
        <Card
          title={t("tasks.approvals")}
          subtitle={t("conversations.planHint")}
          className="approval-card"
          actions={<span className="count-pill">{pending.length}</span>}
          padded={false}
        >
          <div className="approval-list">
            {pending.map((approval) => {
              const task = taskById.get(approval.taskId);
              return (
                <div key={approval.id} className="approval-row">
                  <div className="approval-icon"><Icon name="shield" size={18} /></div>
                  <div className="approval-copy">
                    <strong>{task?.title ?? approval.taskId}</strong>
                    <span>
                      {t("tasks.approvalFor")} · {t("tasks.expires")}: {formatDate(approval.expiresAt)}
                    </span>
                  </div>
                  {canWrite ? (
                    <div className="approval-actions">
                      <Button size="sm" onClick={() => { setDeciding({ approval, decision: "deny" }); setDecisionReason(""); setFormError(null); }}>
                        {t("tasks.deny")}
                      </Button>
                      <Button size="sm" variant="primary" onClick={() => { setDeciding({ approval, decision: "approve" }); setDecisionReason(""); setFormError(null); }}>
                        {t("tasks.approve")}
                      </Button>
                    </div>
                  ) : null}
                </div>
              );
            })}
          </div>
        </Card>
      ) : null}

      <div className="filter-row">
        <div className="filter-label"><Icon name="filter" size={15} /> {t("tasks.filters")}</div>
        <SelectField label={t("tasks.status")} value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)}>
          <option value="">{t("tasks.allStatuses")}</option>
          {statuses.map((status) => <option key={status} value={status}>{t(`status.${status}`)}</option>)}
        </SelectField>
        <span className="filter-count">{t("common.total")}: {tasks.data?.total ?? 0}</span>
        <Button size="sm" variant="ghost" icon="refresh" onClick={() => { void tasks.reload(); void approvals.reload(); }}>
          {t("common.refresh")}
        </Button>
      </div>

      {tasks.error ? <ErrorBanner message={t(`errors.${tasks.error.code}`)} onRetry={tasks.reload} retryLabel={t("common.retry")} /> : null}

      <Card padded={false}>
        {tasks.loading ? <div className="card-padded"><Skeleton lines={6} /></div> : null}
        {!tasks.loading && tasks.data?.items.length === 0 ? (
          <EmptyState
            icon="tasks"
            title={t("tasks.empty")}
            body={t("tasks.emptyDesc")}
            action={canWrite ? <Button variant="primary" icon="plus" onClick={() => setCreating(true)}>{t("tasks.new")}</Button> : undefined}
          />
        ) : (
          !tasks.loading && (
            <div className="table-wrap">
              <table className="data-table">
                <thead>
                  <tr>
                    <th>{t("tasks.titleField")}</th>
                    <th>{t("tasks.tool")}</th>
                    <th>{t("tasks.risk")}</th>
                    <th>{t("tasks.status")}</th>
                    <th>{t("tasks.created")}</th>
                    <th aria-label={t("common.actions")} />
                  </tr>
                </thead>
                <tbody>
                  {(tasks.data?.items ?? []).map((task) => (
                    <tr key={task.id}>
                      <td>
                        <button type="button" className="table-title" onClick={() => setViewing(task)}>
                          {task.title}
                        </button>
                        {task.lastError ? <span className="table-sub error-text">{task.lastError.slice(0, 90)}</span> : null}
                      </td>
                      <td><code className="mono-chip">{task.tool}</code></td>
                      <td><RiskBadge risk={task.riskLevel} label={t(`risk.${task.riskLevel}`)} /></td>
                      <td><StatusBadge status={task.status} label={t(`status.${task.status}`)} /></td>
                      <td className="muted">{formatDate(task.createdAt)}</td>
                      <td className="row-actions">
                        <button type="button" className="icon-btn" aria-label={t("tasks.viewResult")} onClick={() => setViewing(task)}>
                          <Icon name="search" size={15} />
                        </button>
                        {canWrite && (task.status === "queued" || task.status === "awaiting_approval" || task.status === "blocked") ? (
                          <button type="button" className="icon-btn icon-btn-danger" aria-label={t("tasks.cancel")} onClick={() => setCancelling(task)}>
                            <Icon name="ban" size={15} />
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

      {creating ? (
        <Drawer
          title={t("tasks.newTitle")}
          description={t("tasks.subtitle")}
          size="lg"
          onClose={() => setCreating(false)}
          footer={
            <>
              <Button variant="ghost" onClick={() => setCreating(false)}>{t("common.cancel")}</Button>
              <Button variant="primary" type="submit" form="task-form" loading={saving} disabled={!title.trim()}>{t("tasks.create")}</Button>
            </>
          }
        >
          <form id="task-form" className="form-stack" onSubmit={createTask}>
            <TextField label={t("tasks.titleField")} value={title} onChange={(event) => setTitle(event.target.value)} maxLength={200} data-autofocus required />
            <TextAreaField label={t("tasks.description")} value={description} onChange={(event) => setDescription(event.target.value)} rows={3} maxLength={2000} />
            <SelectField label={t("tasks.tool")} value={tool} onChange={(event) => selectTool(event.target.value)}>
              {(tools.data?.items ?? []).map((definition) => (
                <option key={definition.name} value={definition.name}>
                  {t(`tools.${definition.name}`)} · {t(`risk.${definition.risk}`)}
                </option>
              ))}
            </SelectField>
            <SelectField label={t("tasks.conversationOptional")} value={conversationId} onChange={(event) => setConversationId(event.target.value)}>
              <option value="">{t("common.none")}</option>
              {(conversations.data?.items ?? []).map((conversation) => (
                <option key={conversation.id} value={conversation.id}>{conversation.title}</option>
              ))}
            </SelectField>
            <TextAreaField
              label={t("tasks.parameters")}
              value={parameters}
              onChange={(event) => setParameters(event.target.value)}
              rows={5}
              hint={t("tasks.parametersHint")}
              className="code-field"
            />
            <div className="banner banner-info">
              <Icon name="shield" size={18} />
              <span>{t("tasks.activeWarning")}</span>
            </div>
            {formError ? <ErrorBanner message={formError} /> : null}
          </form>
        </Drawer>
      ) : null}

      {viewing ? (
        <Drawer
          title={viewing.title}
          description={`${t("tasks.tool")}: ${viewing.tool} · ${t("tasks.attempts")}: ${viewing.attempts}`}
          size="lg"
          onClose={() => setViewing(null)}
        >
          <div className="detail-grid">
            <div><span className="detail-label">{t("tasks.status")}</span><StatusBadge status={viewing.status} label={t(`status.${viewing.status}`)} /></div>
            <div><span className="detail-label">{t("tasks.risk")}</span><RiskBadge risk={viewing.riskLevel} label={t(`risk.${viewing.riskLevel}`)} /></div>
            <div><span className="detail-label">{t("tasks.created")}</span><span>{formatDate(viewing.createdAt)}</span></div>
            <div><span className="detail-label">{t("tasks.approvedBy")}</span><span>{viewing.approvedBy ? viewing.approvedBy.slice(0, 8) : t("common.none")}</span></div>
          </div>
          {viewing.description ? <p className="detail-copy">{viewing.description}</p> : null}
          {viewing.lastError ? (
            <div className="banner banner-error"><Icon name="alert" size={18} /><span>{viewing.lastError}</span></div>
          ) : null}
          <div>
            <h3 className="section-label">{t("tasks.result")}</h3>
            <JsonBlock value={viewing.result} maxHeight={360} />
          </div>
          <div>
            <h3 className="section-label">{t("tasks.parameters")}</h3>
            <JsonBlock value={viewing.parameters} maxHeight={200} />
          </div>
        </Drawer>
      ) : null}

      {deciding ? (
        <Modal
          title={deciding.decision === "approve" ? t("tasks.approve") : t("tasks.deny")}
          description={deciding.decision === "approve" ? t("tasks.approveConfirm") : t("tasks.denyReason")}
          onClose={() => setDeciding(null)}
          footer={
            <>
              <Button variant="ghost" onClick={() => setDeciding(null)}>{t("common.cancel")}</Button>
              <Button variant={deciding.decision === "approve" ? "primary" : "danger"} onClick={() => void decide()} loading={saving}>
                {t(deciding.decision === "approve" ? "tasks.approve" : "tasks.deny")}
              </Button>
            </>
          }
        >
          <TextAreaField
            label={t("tasks.approvalReason")}
            value={decisionReason}
            onChange={(event) => setDecisionReason(event.target.value)}
            rows={3}
            maxLength={500}
            hint={deciding.decision === "deny" ? t("common.required") : t("common.optional")}
            data-autofocus
          />
          {deciding.decision === "approve" && user?.role === "analyst" ? (
            <p className="modal-copy">{t("tasks.selfApproval")}</p>
          ) : null}
          {formError ? <ErrorBanner message={formError} /> : null}
        </Modal>
      ) : null}

      {cancelling ? (
        <Modal
          title={t("tasks.cancel")}
          description={t("tasks.cancelConfirm")}
          onClose={() => setCancelling(null)}
          footer={
            <>
              <Button variant="ghost" onClick={() => setCancelling(null)}>{t("common.cancel")}</Button>
              <Button variant="danger" onClick={() => void cancelTask()}>{t("tasks.cancel")}</Button>
            </>
          }
        >
          <p className="modal-copy">{cancelling.title}</p>
        </Modal>
      ) : null}
    </div>
  );
}
