"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { api } from "@/lib/api";
import { apiErrorMessage } from "@/lib/errors";
import { useApi, useInterval } from "@/lib/hooks";
import type { Conversation, ListResponse, Message, Task } from "@/lib/types";
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
  TextField,
} from "@/components/ui";

export default function ConversationsPage() {
  const { t, formatDate } = useI18n();
  const { selected, loading: projectLoading } = useProjects();
  const { user } = useAuth();
  const canWrite = user?.role !== "viewer" && !user?.mustChangePassword;
  const { push } = useToast();
  const conversations = useApi<ListResponse<Conversation>>(
    selected ? `/projects/${selected.id}/conversations?limit=100` : null,
  );
  const [activeId, setActiveId] = useState<string | null>(null);
  const active = useMemo(
    () => conversations.data?.items.find((item) => item.id === activeId) ?? null,
    [conversations.data, activeId],
  );
  const messages = useApi<ListResponse<Message>>(active ? `/conversations/${active.id}/messages?limit=200` : null);
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const [planning, setPlanning] = useState(false);
  const [creating, setCreating] = useState(false);
  const [title, setTitle] = useState("");
  const [deleting, setDeleting] = useState<Conversation | null>(null);
  const [formError, setFormError] = useState<string | null>(null);
  const endRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!activeId || !conversations.data?.items.some((item) => item.id === activeId)) {
      setActiveId(conversations.data?.items[0]?.id ?? null);
    }
  }, [conversations.data, activeId]);

  useEffect(() => {
    endRef.current?.scrollIntoView({ block: "end" });
  }, [messages.data?.items.length, activeId]);

  useInterval(() => {
    if (active) void messages.reload();
  }, 6000);

  async function createConversation(event: React.FormEvent) {
    event.preventDefault();
    if (!selected) return;
    setSending(true);
    setFormError(null);
    try {
      const response = await api<{ conversation: Conversation }>(`/projects/${selected.id}/conversations`, {
        method: "POST",
        body: { title: title.trim() },
      });
      await conversations.reload();
      setActiveId(response.conversation.id);
      setCreating(false);
      setTitle("");
      push(t("common.success"), "success");
    } catch (err) {
      setFormError(apiErrorMessage(t, err));
    } finally {
      setSending(false);
    }
  }

  async function sendMessage() {
    if (!active || !draft.trim()) return;
    setSending(true);
    try {
      await api(`/conversations/${active.id}/messages`, { method: "POST", body: { content: draft.trim() } });
      setDraft("");
      await messages.reload();
      await conversations.reload();
    } catch (err) {
      push(apiErrorMessage(t, err), "error");
    } finally {
      setSending(false);
    }
  }

  async function generatePlan() {
    if (!active) return;
    setPlanning(true);
    try {
      const response = await api<{ tasks: Task[] }>(`/conversations/${active.id}/plan`, { method: "POST", body: {} });
      await messages.reload();
      await conversations.reload();
      push(t("conversations.planCreated", { count: response.tasks.length }), "success");
    } catch (err) {
      push(apiErrorMessage(t, err), "error");
    } finally {
      setPlanning(false);
    }
  }

  async function deleteConversation() {
    if (!deleting) return;
    try {
      await api(`/conversations/${deleting.id}`, { method: "DELETE" });
      await conversations.reload();
      setDeleting(null);
      push(t("common.success"), "success");
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
        title={t("conversations.title")}
        subtitle={t("conversations.subtitle")}
        badge={<span className="badge badge-neutral">{selected.name}</span>}
        actions={canWrite ? <Button variant="primary" icon="plus" onClick={() => setCreating(true)}>{t("conversations.new")}</Button> : undefined}
      />

      {conversations.error ? (
        <ErrorBanner message={t(`errors.${conversations.error.code}`)} onRetry={conversations.reload} retryLabel={t("common.retry")} />
      ) : null}

      <div className="conversation-layout">
        <aside className="conversation-list">
          <header className="panel-heading">
            <span>{t("conversations.messages")}</span>
            <span className="count-pill">{conversations.data?.total ?? 0}</span>
          </header>
          {conversations.loading ? <Skeleton lines={5} /> : null}
          {!conversations.loading && conversations.data?.items.length === 0 ? (
            <EmptyState
              icon="chat"
              title={t("conversations.empty")}
              body={t("conversations.emptyDesc")}
              action={<Button variant="primary" size="sm" icon="plus" onClick={() => setCreating(true)}>{t("conversations.new")}</Button>}
            />
          ) : null}
          <div className="conversation-items">
            {(conversations.data?.items ?? []).map((conversation) => (
              <button
                key={conversation.id}
                type="button"
                className={`conversation-item ${conversation.id === activeId ? "is-active" : ""}`}
                onClick={() => setActiveId(conversation.id)}
              >
                <span className="conversation-item-title">{conversation.title}</span>
                <span className="conversation-item-meta">{formatDate(conversation.updatedAt)}</span>
              </button>
            ))}
          </div>
        </aside>

        <section className="conversation-thread card">
          {active ? (
            <>
              <header className="thread-head">
                <div>
                  <h2>{active.title}</h2>
                  <span className="thread-meta">{t("conversations.planHint")}</span>
                </div>
                {canWrite ? (
                  <div className="thread-actions">
                    <Button variant="ghost" size="sm" icon="trash" onClick={() => setDeleting(active)} aria-label={t("common.delete")} />
                    <Button size="sm" variant="primary" icon="activity" onClick={() => void generatePlan()} loading={planning}>
                      {t("conversations.generatePlan")}
                    </Button>
                  </div>
                ) : null}
              </header>

              <div className="thread-messages" aria-live="polite">
                {messages.loading ? <Skeleton lines={6} /> : null}
                {messages.error ? <ErrorBanner message={t(`errors.${messages.error.code}`)} onRetry={messages.reload} retryLabel={t("common.retry")} /> : null}
                {!messages.loading && messages.data?.items.length === 0 ? (
                  <EmptyState icon="chat" title={t("conversations.emptyMessages")} body={t("conversations.placeholder")} />
                ) : null}
                {(messages.data?.items ?? []).map((message) => (
                  <article key={message.id} className={`message message-${message.role}`}>
                    <header>
                      <span className="message-role">
                        {t(`conversations.${message.role === "assistant" ? "assistant" : message.role === "tool" ? "tool" : message.role === "system" ? "system" : "user"}`)}
                      </span>
                      <time>{formatDate(message.createdAt)}</time>
                    </header>
                    <div className="message-body">{message.content}</div>
                  </article>
                ))}
                <div ref={endRef} />
              </div>

              {canWrite ? (
                <div className="composer">
                  <textarea
                    className="input textarea composer-input"
                    value={draft}
                    placeholder={t("conversations.placeholder")}
                    onChange={(event) => setDraft(event.target.value)}
                    rows={3}
                    aria-label={t("conversations.placeholder")}
                    onKeyDown={(event) => {
                      if (event.key === "Enter" && (event.metaKey || event.ctrlKey)) {
                        event.preventDefault();
                        void sendMessage();
                      }
                    }}
                  />
                  <div className="composer-foot">
                    <span>{t("conversations.planHint")}</span>
                    <Button variant="primary" icon="arrowRight" onClick={() => void sendMessage()} loading={sending} disabled={!draft.trim()}>
                      {t("conversations.send")}
                    </Button>
                  </div>
                </div>
              ) : (
                <div className="composer">
                  <div className="banner banner-info"><Icon name="lock" size={18} /><span>{t("auth.viewerReadOnly")}</span></div>
                </div>
              )}
            </>
          ) : (
            <EmptyState icon="chat" title={t("conversations.selectConversation")} body={t("conversations.emptyDesc")} />
          )}
        </section>
      </div>

      {creating ? (
        <Drawer
          title={t("conversations.newTitle")}
          onClose={() => setCreating(false)}
          footer={
            <>
              <Button variant="ghost" onClick={() => setCreating(false)}>{t("common.cancel")}</Button>
              <Button variant="primary" type="submit" form="conversation-form" loading={sending} disabled={!title.trim()}>{t("common.create")}</Button>
            </>
          }
        >
          <form id="conversation-form" className="form-stack" onSubmit={createConversation}>
            <TextField label={t("conversations.titleField")} value={title} onChange={(event) => setTitle(event.target.value)} maxLength={200} data-autofocus required />
            {formError ? <ErrorBanner message={formError} /> : null}
          </form>
        </Drawer>
      ) : null}

      {deleting ? (
        <Modal
          title={t("common.delete")}
          description={t("conversations.deleteConfirm")}
          onClose={() => setDeleting(null)}
          footer={
            <>
              <Button variant="ghost" onClick={() => setDeleting(null)}>{t("common.cancel")}</Button>
              <Button variant="danger" onClick={() => void deleteConversation()}>{t("common.delete")}</Button>
            </>
          }
        >
          <p className="modal-copy">{deleting.title}</p>
        </Modal>
      ) : null}
    </div>
  );
}
