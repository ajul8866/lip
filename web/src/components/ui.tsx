"use client";

import { useId } from "react";
import type {
  ButtonHTMLAttributes,
  InputHTMLAttributes,
  ReactNode,
  SelectHTMLAttributes,
  TextareaHTMLAttributes,
} from "react";
import type { ApprovalStatus, AssetKind, FindingStatus, Risk, Role, Severity, TaskStatus } from "@/lib/types";

export type IconName =
  | "activity"
  | "alert"
  | "arrowRight"
  | "assets"
  | "ban"
  | "check"
  | "copy"
  | "chevronDown"
  | "clock"
  | "inbox"
  | "dashboard"
  | "download"
  | "edit"
  | "external"
  | "file"
  | "filter"
  | "findings"
  | "folder"
  | "globe"
  | "key"
  | "lock"
  | "logout"
  | "menu"
  | "moon"
  | "plus"
  | "refresh"
  | "report"
  | "search"
  | "server"
  | "settings"
  | "shield"
  | "sun"
  | "tasks"
  | "trash"
  | "users"
  | "x"
  | "chat";

const paths: Record<IconName, ReactNode> = {
  activity: <path d="M3 12h4l2.5-6 5 12L17 12h4" />,
  alert: (
    <>
      <path d="M10.3 3.6 2.6 17a2 2 0 0 0 1.7 3h15.4a2 2 0 0 0 1.7-3L13.7 3.6a2 2 0 0 0-3.4 0Z" />
      <path d="M12 9v4" />
      <path d="M12 17h.01" />
    </>
  ),
  arrowRight: <path d="M5 12h14M13 6l6 6-6 6" />,
  assets: (
    <>
      <path d="M4 7h16" />
      <path d="M6 7v12a1 1 0 0 0 1 1h10a1 1 0 0 0 1-1V7" />
      <path d="M9 7V5a1 1 0 0 1 1-1h4a1 1 0 0 1 1 1v2" />
      <path d="M10 11v5M14 11v5" />
    </>
  ),
  ban: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="m5.7 5.7 12.6 12.6" />
    </>
  ),
  check: <path d="m4 12 5 5L20 6" />,
  chevronDown: <path d="m6 9 6 6 6-6" />,
  clock: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3 2" />
    </>
  ),
  copy: (
    <>
      <rect x="8" y="8" width="12" height="12" rx="2" />
      <path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2" />
    </>
  ),
  dashboard: (
    <>
      <rect x="3" y="3" width="7" height="9" rx="1.5" />
      <rect x="14" y="3" width="7" height="5" rx="1.5" />
      <rect x="14" y="12" width="7" height="9" rx="1.5" />
      <rect x="3" y="16" width="7" height="5" rx="1.5" />
    </>
  ),
  download: <path d="M12 3v12m0 0 4-4m-4 4-4-4M4 21h16" />,
  edit: (
    <>
      <path d="M12 20h9" />
      <path d="M16.5 3.5a2.1 2.1 0 0 1 3 3L7 19l-4 1 1-4Z" />
    </>
  ),
  inbox: (
    <>
      <path d="M4 4h16v12l-4 4H8l-4-4Z" />
      <path d="M4 12h5l1.5 2h3L15 12h5" />
    </>
  ),
  external: (
    <>
      <path d="M14 4h6v6" />
      <path d="M20 4 11 13" />
      <path d="M20 14v5a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1h5" />
    </>
  ),
  file: (
    <>
      <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8Z" />
      <path d="M14 2v6h6M8 13h8M8 17h5" />
    </>
  ),
  filter: <path d="M3 5h18l-7 8v6l-4 2v-8Z" />,
  findings: (
    <>
      <path d="m12 3 8 4v5c0 5-3.5 8-8 9-4.5-1-8-4-8-9V7Z" />
      <path d="m9 12 2 2 4-4" />
    </>
  ),
  folder: <path d="M3 6a2 2 0 0 1 2-2h4l2 3h8a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2Z" />,
  globe: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M3 12h18M12 3a15 15 0 0 1 0 18M12 3a15 15 0 0 0 0 18" />
    </>
  ),
  key: (
    <>
      <circle cx="8" cy="15" r="4" />
      <path d="m11 12 9-9M17 6l3 3M15 8l2 2" />
    </>
  ),
  lock: (
    <>
      <rect x="4" y="10" width="16" height="11" rx="2" />
      <path d="M8 10V7a4 4 0 0 1 8 0v3" />
    </>
  ),
  logout: (
    <>
      <path d="M10 17l5-5-5-5M15 12H3" />
      <path d="M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4" />
    </>
  ),
  menu: <path d="M4 6h16M4 12h16M4 18h16" />,
  moon: <path d="M21 12.8A8.5 8.5 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8Z" />,
  plus: <path d="M12 5v14M5 12h14" />,
  refresh: <path d="M20 11a8 8 0 1 0-2.3 6M20 4v7h-7" />,
  report: (
    <>
      <path d="M14 2H6a2 2 0 0 0-2 2v16a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2V8Z" />
      <path d="M14 2v6h6M8 13h8M8 17h5" />
    </>
  ),
  search: (
    <>
      <circle cx="11" cy="11" r="7" />
      <path d="m20 20-4-4" />
    </>
  ),
  server: (
    <>
      <rect x="3" y="4" width="18" height="6" rx="1.5" />
      <rect x="3" y="14" width="18" height="6" rx="1.5" />
      <path d="M7 7h.01M7 17h.01" />
    </>
  ),
  settings: (
    <>
      <circle cx="12" cy="12" r="3" />
      <path d="M19.4 15a1.7 1.7 0 0 0 .3 1.9l.1.1-2.8 2.8-.1-.1a1.7 1.7 0 0 0-1.9-.3 1.7 1.7 0 0 0-1 1.6v.2h-4v-.2a1.7 1.7 0 0 0-1-1.6 1.7 1.7 0 0 0-1.9.3l-.1.1L4.2 17l.1-.1a1.7 1.7 0 0 0 .3-1.9 1.7 1.7 0 0 0-1.6-1H3v-4h.2a1.7 1.7 0 0 0 1.6-1 1.7 1.7 0 0 0-.3-1.9L4.4 7 7.2 4.2l.1.1A1.7 1.7 0 0 0 9.2 5a1.7 1.7 0 0 0 1-1.6V3h4v.2a1.7 1.7 0 0 0 1 1.6 1.7 1.7 0 0 0 1.9-.3l.1-.1L20 7l-.1.1a1.7 1.7 0 0 0-.3 1.9 1.7 1.7 0 0 0 1.6 1h.2v4h-.2a1.7 1.7 0 0 0-1.6 1Z" />
    </>
  ),
  shield: (
    <>
      <path d="m12 3 8 4v5c0 5-3.5 8-8 9-4.5-1-8-4-8-9V7Z" />
      <path d="m9 12 2 2 4-4" />
    </>
  ),
  sun: (
    <>
      <circle cx="12" cy="12" r="4" />
      <path d="M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4" />
    </>
  ),
  tasks: (
    <>
      <rect x="4" y="3" width="16" height="18" rx="2" />
      <path d="m8 8 1.5 1.5L12 7M8 14l1.5 1.5L12 13M14 8h4M14 14h4" />
    </>
  ),
  trash: (
    <>
      <path d="M3 6h18M8 6V4h8v2M6 6l1 15h10l1-15M10 11v6M14 11v6" />
    </>
  ),
  users: (
    <>
      <circle cx="9" cy="8" r="4" />
      <path d="M2 21a7 7 0 0 1 14 0" />
      <path d="M17 4a4 4 0 0 1 0 8M22 21a6 6 0 0 0-4-5.6" />
    </>
  ),
  x: <path d="M6 6l12 12M18 6 6 18" />,
  chat: (
    <>
      <path d="M21 12a8 8 0 0 1-8 8H4l2.4-3.6A8 8 0 1 1 21 12Z" />
      <path d="M8 11h.01M12 11h.01M16 11h.01" />
    </>
  ),
};

export function Icon({ name, size = 18, className }: { name: IconName; size?: number; className?: string }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.8"
      strokeLinecap="round"
      strokeLinejoin="round"
      className={className}
      aria-hidden="true"
      focusable="false"
    >
      {paths[name]}
    </svg>
  );
}

type ButtonVariant = "primary" | "secondary" | "ghost" | "danger";
type ButtonSize = "sm" | "md";

interface ButtonProps extends ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  loading?: boolean;
  icon?: IconName;
}

export function Button({
  variant = "secondary",
  size = "md",
  loading = false,
  icon,
  children,
  className = "",
  disabled,
  ...props
}: ButtonProps) {
  return (
    <button
      type={props.type ?? "button"}
      className={`btn btn-${variant} btn-${size} ${className}`}
      disabled={disabled || loading}
      {...props}
    >
      {loading ? <span className="spinner" aria-hidden="true" /> : icon ? <Icon name={icon} size={16} /> : null}
      {children ? <span>{children}</span> : null}
    </button>
  );
}

export function Card({
  title,
  subtitle,
  actions,
  children,
  className = "",
  padded = true,
}: {
  title?: ReactNode;
  subtitle?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
  padded?: boolean;
}) {
  return (
    <section className={`card ${padded ? "card-padded" : ""} ${className}`}>
      {(title || actions) && (
        <header className="card-header">
          <div>
            {title ? <h2 className="card-title">{title}</h2> : null}
            {subtitle ? <p className="card-subtitle">{subtitle}</p> : null}
          </div>
          {actions ? <div className="card-actions">{actions}</div> : null}
        </header>
      )}
      {children}
    </section>
  );
}

export function PageHeader({
  title,
  subtitle,
  actions,
  badge,
}: {
  title: ReactNode;
  subtitle?: ReactNode;
  actions?: ReactNode;
  badge?: ReactNode;
}) {
  return (
    <header className="page-header page-enter">
      <div className="page-header-copy">
        <div className="page-title-row">
          <h1>{title}</h1>
          {badge}
        </div>
        {subtitle ? <p className="page-subtitle">{subtitle}</p> : null}
      </div>
      {actions ? <div className="page-actions">{actions}</div> : null}
    </header>
  );
}

export function Stat({
  label,
  value,
  hint,
  icon,
  tone = "neutral",
}: {
  label: ReactNode;
  value: ReactNode;
  hint?: ReactNode;
  icon?: IconName;
  tone?: "neutral" | "accent" | "warning" | "danger" | "success";
}) {
  return (
    <div className={`stat stat-${tone}`}>
      <div className="stat-top">
        <span className="stat-label">{label}</span>
        {icon ? (
          <span className="stat-icon">
            <Icon name={icon} size={18} />
          </span>
        ) : null}
      </div>
      <div className="stat-value">{value}</div>
      {hint ? <div className="stat-hint">{hint}</div> : null}
    </div>
  );
}

const toneByStatus: Record<string, string> = {
  queued: "neutral",
  running: "info",
  awaiting_approval: "warning",
  pending: "warning",
  succeeded: "success",
  approved: "success",
  open: "info",
  triaged: "accent",
  accepted: "accent",
  fixed: "success",
  failed: "danger",
  denied: "danger",
  cancelled: "neutral",
  blocked: "danger",
  expired: "neutral",
  false_positive: "neutral",
};

export function StatusBadge({ status, label }: { status: string; label: string }) {
  return <span className={`badge badge-${toneByStatus[status] ?? "neutral"}`}>{label}</span>;
}

export function SeverityBadge({ severity, label }: { severity: Severity; label: string }) {
  const tone =
    severity === "critical" ? "critical" : severity === "high" ? "danger" : severity === "medium" ? "warning" : severity === "low" ? "info" : "neutral";
  return <span className={`badge badge-${tone} badge-dot`}>{label}</span>;
}

export function RiskBadge({ risk, label }: { risk: Risk; label: string }) {
  const tone = risk === "destructive" ? "critical" : risk === "active" ? "warning" : "neutral";
  return <span className={`badge badge-${tone}`}>{label}</span>;
}

export function RoleBadge({ role, label }: { role: Role; label: string }) {
  const tone = role === "admin" ? "accent" : role === "analyst" ? "info" : "neutral";
  return <span className={`badge badge-${tone}`}>{label}</span>;
}

export function Field({
  label,
  hint,
  error,
  htmlFor,
  children,
  className = "",
}: {
  label: ReactNode;
  hint?: ReactNode;
  error?: ReactNode;
  htmlFor?: string;
  children: ReactNode;
  className?: string;
}) {
  return (
    <div className={`field ${className}`}>
      <label className="field-label" htmlFor={htmlFor}>
        {label}
      </label>
      {children}
      {hint && !error ? <p className="field-hint">{hint}</p> : null}
      {error ? <p className="field-error">{error}</p> : null}
    </div>
  );
}

interface TextFieldProps extends InputHTMLAttributes<HTMLInputElement> {
  label: ReactNode;
  hint?: ReactNode;
  error?: ReactNode;
}

export function TextField({ label, hint, error, id, ...props }: TextFieldProps) {
  const generated = useId();
  const inputId = id ?? generated;
  return (
    <Field label={label} hint={hint} error={error} htmlFor={inputId}>
      <input id={inputId} className={`input ${error ? "input-invalid" : ""}`} {...props} />
    </Field>
  );
}

interface TextAreaFieldProps extends TextareaHTMLAttributes<HTMLTextAreaElement> {
  label: ReactNode;
  hint?: ReactNode;
  error?: ReactNode;
}

export function TextAreaField({ label, hint, error, id, ...props }: TextAreaFieldProps) {
  const generated = useId();
  const inputId = id ?? generated;
  return (
    <Field label={label} hint={hint} error={error} htmlFor={inputId}>
      <textarea id={inputId} className={`input textarea ${error ? "input-invalid" : ""}`} {...props} />
    </Field>
  );
}

interface SelectFieldProps extends SelectHTMLAttributes<HTMLSelectElement> {
  label: ReactNode;
  hint?: ReactNode;
  error?: ReactNode;
  children: ReactNode;
}

export function SelectField({ label, hint, error, id, children, ...props }: SelectFieldProps) {
  const generated = useId();
  const inputId = id ?? generated;
  return (
    <Field label={label} hint={hint} error={error} htmlFor={inputId}>
      <div className="select-wrap">
        <select id={inputId} className={`input select ${error ? "input-invalid" : ""}`} {...props}>
          {children}
        </select>
        <Icon name="chevronDown" size={16} className="select-chevron" />
      </div>
    </Field>
  );
}

export function CheckboxField({
  label,
  hint,
  checked,
  onChange,
  disabled,
}: {
  label: ReactNode;
  hint?: ReactNode;
  checked: boolean;
  onChange: (checked: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <label className={`checkbox-field ${disabled ? "is-disabled" : ""}`}>
      <input type="checkbox" checked={checked} onChange={(event) => onChange(event.target.checked)} disabled={disabled} />
      <span className="checkbox-box" aria-hidden="true">
        <Icon name="check" size={12} />
      </span>
      <span>
        <span className="checkbox-label">{label}</span>
        {hint ? <span className="checkbox-hint">{hint}</span> : null}
      </span>
    </label>
  );
}

export function EmptyState({
  icon = "inbox" as IconName,
  title,
  body,
  action,
}: {
  icon?: IconName;
  title: ReactNode;
  body?: ReactNode;
  action?: ReactNode;
}) {
  return (
    <div className="empty">
      <div className="empty-icon">
        <Icon name={icon} size={22} />
      </div>
      <h3>{title}</h3>
      {body ? <p>{body}</p> : null}
      {action ? <div className="empty-action">{action}</div> : null}
    </div>
  );
}

export function ErrorBanner({ message, onRetry, retryLabel }: { message: ReactNode; onRetry?: () => void; retryLabel?: ReactNode }) {
  return (
    <div className="banner banner-error" role="alert">
      <Icon name="alert" size={18} />
      <span>{message}</span>
      {onRetry ? (
        <button type="button" className="banner-action" onClick={onRetry}>
          {retryLabel}
        </button>
      ) : null}
    </div>
  );
}

export function InfoBanner({
  icon = "shield",
  children,
  tone = "info",
}: {
  icon?: IconName;
  children: ReactNode;
  tone?: "info" | "warning" | "success";
}) {
  return (
    <div className={`banner banner-${tone}`}>
      <Icon name={icon} size={18} />
      <span>{children}</span>
    </div>
  );
}

export function Skeleton({ lines = 3, className = "" }: { lines?: number; className?: string }) {
  return (
    <div className={`skeleton-stack ${className}`} aria-hidden="true">
      {Array.from({ length: lines }).map((_, index) => (
        <span key={index} className="skeleton-line" style={{ width: `${70 + ((index * 13) % 30)}%` }} />
      ))}
    </div>
  );
}

export function Spinner({ label }: { label?: string }) {
  return (
    <span className="loading-inline" role="status">
      <span className="spinner" aria-hidden="true" />
      {label ? <span>{label}</span> : null}
    </span>
  );
}

export function JsonBlock({ value, maxHeight }: { value: unknown; maxHeight?: number }) {
  return (
    <pre className="json-block" style={maxHeight ? { maxHeight } : undefined}>
      {JSON.stringify(value, null, 2)}
    </pre>
  );
}

export function Tabs({
  items,
  value,
  onChange,
}: {
  items: { id: string; label: string; count?: number }[];
  value: string;
  onChange: (id: string) => void;
}) {
  return (
    <div className="tabs" role="tablist">
      {items.map((item) => (
        <button
          key={item.id}
          type="button"
          role="tab"
          aria-selected={value === item.id}
          className={`tab ${value === item.id ? "is-active" : ""}`}
          onClick={() => onChange(item.id)}
        >
          <span>{item.label}</span>
          {typeof item.count === "number" ? <span className="tab-count">{item.count}</span> : null}
        </button>
      ))}
    </div>
  );
}

export function KindBadge({ kind }: { kind: AssetKind }) {
  return <span className="badge badge-neutral badge-mono">{kind}</span>;
}

export function ApprovalStatusBadge({ status, label }: { status: ApprovalStatus; label: string }) {
  return <StatusBadge status={status} label={label} />;
}

export function TaskStatusBadge({ status, label }: { status: TaskStatus; label: string }) {
  return <StatusBadge status={status} label={label} />;
}

export function FindingStatusBadge({ status, label }: { status: FindingStatus; label: string }) {
  return <StatusBadge status={status} label={label} />;
}
