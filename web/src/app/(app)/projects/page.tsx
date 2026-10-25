"use client";

import Link from "next/link";
import { useState } from "react";
import { api } from "@/lib/api";
import { apiErrorMessage } from "@/lib/errors";
import type { Project } from "@/lib/types";
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
  PageHeader,
  Skeleton,
  TextAreaField,
  TextField,
} from "@/components/ui";

export default function ProjectsPage() {
  const { t, formatDate } = useI18n();
  const { user } = useAuth();
  const { projects, selectedId, select, loading, error, reload } = useProjects();
  const { push } = useToast();
  const [creating, setCreating] = useState(false);
  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const [deleting, setDeleting] = useState<Project | null>(null);

  const canWrite = (user?.role === "admin" || user?.role === "analyst") && !user.mustChangePassword;
  const isAdmin = user?.role === "admin" && !user.mustChangePassword;

  async function createProject(event: React.FormEvent) {
    event.preventDefault();
    setSaving(true);
    setFormError(null);
    try {
      const response = await api<{ project: Project }>("/projects", {
        method: "POST",
        body: { name: name.trim(), description: description.trim() },
      });
      await reload();
      select(response.project.id);
      push(t("common.success"), "success");
      setCreating(false);
      setName("");
      setDescription("");
    } catch (err) {
      setFormError(apiErrorMessage(t, err));
    } finally {
      setSaving(false);
    }
  }

  async function deleteProject() {
    if (!deleting) return;
    try {
      await api(`/projects/${deleting.id}`, { method: "DELETE" });
      await reload();
      push(t("common.success"), "success");
      setDeleting(null);
    } catch (err) {
      push(apiErrorMessage(t, err), "error");
    }
  }

  return (
    <div className="page-stack page-enter">
      <PageHeader
        title={t("projects.title")}
        subtitle={t("projects.subtitle")}
        actions={
          canWrite ? (
            <Button variant="primary" icon="plus" onClick={() => setCreating(true)}>
              {t("projects.new")}
            </Button>
          ) : undefined
        }
      />

      {error ? <ErrorBanner message={t(`errors.${error.code}`)} onRetry={() => void reload()} retryLabel={t("common.retry")} /> : null}
      {loading ? <Skeleton lines={5} /> : null}

      {!loading && projects.length === 0 ? (
        <Card>
          <EmptyState
            icon="folder"
            title={t("projects.empty")}
            body={t("projects.emptyDesc")}
            action={canWrite ? <Button variant="primary" icon="plus" onClick={() => setCreating(true)}>{t("projects.new")}</Button> : undefined}
          />
        </Card>
      ) : null}

      <div className="project-grid">
        {projects.map((project) => (
          <article key={project.id} className={`project-card ${project.id === selectedId ? "is-selected" : ""}`}>
            <div className="project-card-head">
              <span className="project-icon"><Icon name="folder" size={20} /></span>
              {project.id === selectedId ? <span className="badge badge-accent">{t("projects.active")}</span> : null}
            </div>
            <h2>{project.name}</h2>
            <p>{project.description || t("common.none")}</p>
            <span className="project-date">{t("common.created")} · {formatDate(project.createdAt)}</span>
            <div className="project-actions">
              <Button size="sm" onClick={() => select(project.id)} disabled={project.id === selectedId}>
                {t("projects.select")}
              </Button>
              <Link href="/assets" className="btn btn-ghost btn-sm" onClick={() => select(project.id)}>
                <Icon name="assets" size={15} /> {t("projects.manageAssets")}
              </Link>
              {isAdmin ? (
                <button type="button" className="icon-btn icon-btn-danger" aria-label={t("projects.delete")} onClick={() => setDeleting(project)}>
                  <Icon name="trash" size={16} />
                </button>
              ) : null}
            </div>
          </article>
        ))}
      </div>

      {creating ? (
        <Drawer
          title={t("projects.newTitle")}
          onClose={() => setCreating(false)}
          footer={
            <>
              <Button variant="ghost" onClick={() => setCreating(false)}>{t("common.cancel")}</Button>
              <Button variant="primary" type="submit" form="project-form" loading={saving} disabled={!name.trim()}>{t("common.create")}</Button>
            </>
          }
        >
          <form id="project-form" onSubmit={createProject} className="form-stack">
            <TextField label={t("projects.name")} value={name} onChange={(event) => setName(event.target.value)} maxLength={120} data-autofocus required />
            <TextAreaField label={t("projects.description")} value={description} onChange={(event) => setDescription(event.target.value)} rows={4} maxLength={4000} />
            {formError ? <ErrorBanner message={formError} /> : null}
          </form>
        </Drawer>
      ) : null}

      {deleting ? (
        <Modal
          title={t("projects.delete")}
          description={t("projects.deleteConfirm")}
          onClose={() => setDeleting(null)}
          footer={
            <>
              <Button variant="ghost" onClick={() => setDeleting(null)}>{t("common.cancel")}</Button>
              <Button variant="danger" onClick={() => void deleteProject()}>{t("common.delete")}</Button>
            </>
          }
        >
          <p className="modal-copy">{deleting.name}</p>
        </Modal>
      ) : null}
    </div>
  );
}
