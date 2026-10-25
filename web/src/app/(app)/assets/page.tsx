"use client";

import { useMemo, useState } from "react";
import { api } from "@/lib/api";
import { apiErrorMessage } from "@/lib/errors";
import { useApi } from "@/lib/hooks";
import type { Asset, AssetKind, ListResponse } from "@/lib/types";
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
  KindBadge,
  PageHeader,
  SelectField,
  Skeleton,
  TextAreaField,
  TextField,
} from "@/components/ui";

const kinds: AssetKind[] = ["domain", "host", "ip", "cidr", "url", "repo", "other"];

export default function AssetsPage() {
  const { t, formatDate } = useI18n();
  const { selected, loading: projectLoading } = useProjects();
  const { safeLocalOnly, user } = useAuth();
  const canWrite = user?.role !== "viewer" && !user?.mustChangePassword;
  const { push } = useToast();
  const [kindFilter, setKindFilter] = useState("");
  const path = selected ? `/projects/${selected.id}/assets?limit=200${kindFilter ? `&kind=${kindFilter}` : ""}` : null;
  const { data, error, loading, reload } = useApi<ListResponse<Asset>>(path);
  const [importOpen, setImportOpen] = useState(false);
  const [addOpen, setAddOpen] = useState(false);
  const [deleting, setDeleting] = useState<Asset | null>(null);
  const [source, setSource] = useState<"inline" | "file" | "url">("inline");
  const [values, setValues] = useState("");
  const [filePath, setFilePath] = useState("");
  const [remoteUrl, setRemoteUrl] = useState("");
  const [kind, setKind] = useState<AssetKind>("domain");
  const [value, setValue] = useState("");
  const [saving, setSaving] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);

  const assetCount = data?.items.length ?? 0;
  const sourceLabel = useMemo(
    () => ({ inline: t("assets.inline"), file: t("assets.file"), url: t("assets.url") }),
    [t],
  );

  async function importAssets(event: React.FormEvent) {
    event.preventDefault();
    if (!selected) return;
    setSaving(true);
    setFormError(null);
    try {
      const body =
        source === "inline"
          ? { source, values: values.split("\n").map((line) => line.trim()).filter(Boolean) }
          : source === "file"
            ? { source, path: filePath.trim() }
            : { source, url: remoteUrl.trim() };
      const result = await api<{ inserted: number; updated: number; skipped: number; total: number }>(
        `/projects/${selected.id}/assets/import`,
        { method: "POST", body },
      );
      await reload();
      push(t("assets.imported", { inserted: result.inserted, updated: result.updated, skipped: result.skipped }), "success");
      setImportOpen(false);
      setValues("");
      setFilePath("");
      setRemoteUrl("");
    } catch (err) {
      setFormError(apiErrorMessage(t, err));
    } finally {
      setSaving(false);
    }
  }

  async function addAsset(event: React.FormEvent) {
    event.preventDefault();
    if (!selected) return;
    setSaving(true);
    setFormError(null);
    try {
      await api(`/projects/${selected.id}/assets`, { method: "POST", body: { kind, value: value.trim() } });
      await reload();
      push(t("assets.added"), "success");
      setAddOpen(false);
      setValue("");
    } catch (err) {
      setFormError(apiErrorMessage(t, err));
    } finally {
      setSaving(false);
    }
  }

  async function deleteAsset() {
    if (!deleting) return;
    try {
      await api(`/assets/${deleting.id}`, { method: "DELETE" });
      await reload();
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
        title={t("assets.title")}
        subtitle={t("assets.subtitle")}
        badge={<span className="badge badge-neutral">{selected.name}</span>}
        actions={canWrite ? (
          <>
            <Button icon="plus" onClick={() => setAddOpen(true)}>{t("assets.add")}</Button>
            <Button variant="primary" icon="download" onClick={() => setImportOpen(true)}>{t("assets.import")}</Button>
          </>
        ) : undefined}
      />

      <div className="filter-row">
        <div className="filter-label"><Icon name="filter" size={15} /> {t("tasks.filters")}</div>
        <SelectField label={t("assets.kind")} value={kindFilter} onChange={(event) => setKindFilter(event.target.value)}>
          <option value="">{t("common.all")}</option>
          {kinds.map((item) => <option key={item} value={item}>{item}</option>)}
        </SelectField>
        <span className="filter-count">{t("common.total")}: {data?.total ?? assetCount}</span>
      </div>

      {error ? <ErrorBanner message={t(`errors.${error.code}`)} onRetry={reload} retryLabel={t("common.retry")} /> : null}
      {loading ? <Skeleton lines={6} /> : null}

      <Card padded={false}>
        {!loading && data?.items.length === 0 ? (
          <EmptyState
            icon="assets"
            title={t("assets.empty")}
            body={t("assets.hint")}
            action={canWrite ? <Button variant="primary" icon="plus" onClick={() => setImportOpen(true)}>{t("assets.import")}</Button> : undefined}
          />
        ) : (
          <div className="table-wrap">
            <table className="data-table">
              <thead>
                <tr>
                  <th>{t("assets.kind")}</th>
                  <th>{t("assets.value")}</th>
                  <th>{t("assets.sourceLabel")}</th>
                  <th>{t("assets.lastSeen")}</th>
                  <th aria-label={t("common.actions")} />
                </tr>
              </thead>
              <tbody>
                {(data?.items ?? []).map((asset) => (
                  <tr key={asset.id}>
                    <td><KindBadge kind={asset.kind} /></td>
                    <td><code className="value-code">{asset.value}</code></td>
                    <td><span className="muted">{asset.source}</span></td>
                    <td><span className="muted">{formatDate(asset.lastSeen)}</span></td>
                    <td className="row-actions">
                      {canWrite ? (
                        <button type="button" className="icon-btn icon-btn-danger" aria-label={t("common.delete")} onClick={() => setDeleting(asset)}>
                          <Icon name="trash" size={15} />
                        </button>
                      ) : null}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {importOpen ? (
        <Drawer
          title={t("assets.importTitle")}
          description={t("assets.hint")}
          size="lg"
          onClose={() => setImportOpen(false)}
          footer={
            <>
              <Button variant="ghost" onClick={() => setImportOpen(false)}>{t("common.cancel")}</Button>
              <Button variant="primary" type="submit" form="import-form" loading={saving}>{t("assets.importSubmit")}</Button>
            </>
          }
        >
          <form id="import-form" className="form-stack" onSubmit={importAssets}>
            <div className="source-tabs" role="tablist">
              {(["inline", "file", "url"] as const).map((item) => (
                <button
                  key={item}
                  type="button"
                  role="tab"
                  aria-selected={source === item}
                  className={`source-tab ${source === item ? "is-active" : ""}`}
                  onClick={() => { setSource(item); setFormError(null); }}
                  disabled={item === "url" && safeLocalOnly}
                >
                  <Icon name={item === "inline" ? "file" : item === "file" ? "server" : "globe"} size={16} />
                  {sourceLabel[item]}
                </button>
              ))}
            </div>
            {source === "inline" ? (
              <TextAreaField
                label={t("assets.values")}
                value={values}
                onChange={(event) => setValues(event.target.value)}
                rows={10}
                placeholder={"domain example.com\nurl https://example.com\nip 192.0.2.10\ncidr 192.0.2.0/24"}
                data-autofocus
              />
            ) : null}
            {source === "file" ? (
              <TextField
                label={t("assets.path")}
                value={filePath}
                onChange={(event) => setFilePath(event.target.value)}
                hint={t("assets.fileHint")}
                placeholder="seed/assets.txt"
                data-autofocus
              />
            ) : null}
            {source === "url" ? (
              safeLocalOnly ? (
                <div className="banner banner-warning"><Icon name="lock" size={18} /> {t("assets.safeModeBlocked")}</div>
              ) : (
                <TextField
                  label={t("assets.urlField")}
                  value={remoteUrl}
                  onChange={(event) => setRemoteUrl(event.target.value)}
                  placeholder="https://example.com/assets.txt"
                  data-autofocus
                />
              )
            ) : null}
            {formError ? <ErrorBanner message={formError} /> : null}
          </form>
        </Drawer>
      ) : null}

      {addOpen ? (
        <Drawer
          title={t("assets.addTitle")}
          onClose={() => setAddOpen(false)}
          footer={
            <>
              <Button variant="ghost" onClick={() => setAddOpen(false)}>{t("common.cancel")}</Button>
              <Button variant="primary" type="submit" form="asset-form" loading={saving} disabled={!value.trim()}>{t("common.add")}</Button>
            </>
          }
        >
          <form id="asset-form" className="form-stack" onSubmit={addAsset}>
            <SelectField label={t("assets.kind")} value={kind} onChange={(event) => setKind(event.target.value as AssetKind)}>
              {kinds.map((item) => <option key={item} value={item}>{item}</option>)}
            </SelectField>
            <TextField label={t("assets.value")} value={value} onChange={(event) => setValue(event.target.value)} data-autofocus required />
            {formError ? <ErrorBanner message={formError} /> : null}
          </form>
        </Drawer>
      ) : null}

      {deleting ? (
        <Modal
          title={t("common.delete")}
          description={t("assets.deleteConfirm")}
          onClose={() => setDeleting(null)}
          footer={
            <>
              <Button variant="ghost" onClick={() => setDeleting(null)}>{t("common.cancel")}</Button>
              <Button variant="danger" onClick={() => void deleteAsset()}>{t("common.delete")}</Button>
            </>
          }
        >
          <code className="value-code">{deleting.value}</code>
        </Modal>
      ) : null}
    </div>
  );
}
