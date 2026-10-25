export type Role = "admin" | "analyst" | "viewer";
export type Locale = "en" | "ko";

export interface User {
  id: string;
  email: string;
  displayName: string;
  role: Role;
  locale: Locale;
  mustChangePassword: boolean;
  disabledAt?: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface Project {
  id: string;
  name: string;
  description: string;
  createdBy?: string;
  archivedAt?: string | null;
  createdAt: string;
  updatedAt: string;
}

export interface Conversation {
  id: string;
  projectId: string;
  title: string;
  createdBy?: string;
  createdAt: string;
  updatedAt: string;
}

export interface Message {
  id: string;
  conversationId: string;
  role: "system" | "user" | "assistant" | "tool";
  content: string;
  metadata: Record<string, unknown>;
  createdBy?: string;
  createdAt: string;
}

export type TaskStatus =
  | "queued"
  | "awaiting_approval"
  | "running"
  | "succeeded"
  | "failed"
  | "cancelled"
  | "blocked";
export type Risk = "passive" | "active" | "destructive";

export interface Task {
  id: string;
  projectId: string;
  conversationId?: string | null;
  title: string;
  description: string;
  tool: string;
  parameters: Record<string, unknown>;
  riskLevel: Risk;
  status: TaskStatus;
  attempts: number;
  lastError: string;
  result: Record<string, unknown>;
  createdBy?: string;
  approvedBy?: string;
  startedAt?: string | null;
  finishedAt?: string | null;
  createdAt: string;
  updatedAt: string;
}

export type Severity = "info" | "low" | "medium" | "high" | "critical";
export type FindingStatus = "open" | "triaged" | "accepted" | "fixed" | "false_positive";

export interface Finding {
  id: string;
  projectId: string;
  taskId?: string | null;
  assetId?: string | null;
  title: string;
  description: string;
  severity: Severity;
  status: FindingStatus;
  source: "manual" | "llm" | "import";
  evidence: Record<string, unknown>;
  createdBy?: string;
  createdAt: string;
  updatedAt: string;
}

export type AssetKind = "host" | "domain" | "ip" | "cidr" | "url" | "repo" | "other";

export interface Asset {
  id: string;
  projectId: string;
  kind: AssetKind;
  value: string;
  source: string;
  attributes: Record<string, unknown>;
  firstSeen: string;
  lastSeen: string;
  createdBy?: string;
}

export type ApprovalStatus = "pending" | "approved" | "denied" | "expired";

export interface Approval {
  id: string;
  projectId: string;
  taskId: string;
  action: string;
  reason: string;
  status: ApprovalStatus;
  requestedBy?: string;
  decidedBy?: string;
  expiresAt: string;
  decidedAt?: string | null;
  createdAt: string;
}

export interface AuditEntry {
  id: number;
  actorId?: string;
  actorEmail: string;
  action: string;
  targetType: string;
  targetId: string;
  result: "success" | "failure" | "denied";
  ip: string;
  metadata: Record<string, unknown>;
  createdAt: string;
}

export interface LLMProvider {
  id: string;
  name: string;
  kind: "ollama" | "openai";
  baseUrl: string;
  model: string;
  apiKeyEnv: string;
  enabled: boolean;
  isDefault: boolean;
  config: Record<string, unknown>;
  createdAt: string;
  updatedAt: string;
}

export interface Setting {
  key: string;
  value: unknown;
  updatedBy?: string;
  updatedAt: string;
}

export interface ToolDefinition {
  name: string;
  description: string;
  risk: Risk;
  requiresApproval: boolean;
  parameterHint: string;
}

export interface Dashboard {
  projects: number;
  tasks: number;
  queuedTasks: number;
  runningTasks: number;
  awaitingTasks: number;
  failedTasks: number;
  findings: number;
  openFindings: number;
  criticalOpen: number;
  highOpen: number;
  assets: number;
  recentFindings: Finding[];
  recentTasks: Task[];
}

export interface ListResponse<T> {
  items: T[];
  total: number;
  limit: number;
  offset: number;
}

export interface MeResponse {
  user: User;
  sessionId: string;
  safeLocalOnly: boolean;
  safeModeSource: string;
  maxConcurrentTasks: number;
  autoRun: boolean;
}

export interface SystemStatus {
  version: string;
  appName: string;
  startedAt: string;
  setupRequired: boolean;
  defaultLocale: Locale;
  supportedLocales: Locale[];
  safeLocalOnly: boolean;
  safeModeSource: string;
  egressRuleCount: number;
  maxConcurrentTasks: number;
  autoRun: boolean;
}

export interface SetupStatus {
  setupRequired: boolean;
  appName: string;
  safeLocalOnly: boolean;
}

export interface ApiErrorEnvelope {
  error: {
    code: string;
    message: string;
    details?: unknown;
  };
}
