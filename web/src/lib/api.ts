import type { ApiErrorEnvelope } from "./types";

const API_BASE = "/api/v1";

export class ApiError extends Error {
  code: string;
  status: number;
  details?: unknown;

  constructor(code: string, status: number, message: string, details?: unknown) {
    super(message);
    this.name = "ApiError";
    this.code = code;
    this.status = status;
    this.details = details;
  }
}

interface RequestOptions {
  method?: "GET" | "POST" | "PATCH" | "DELETE";
  body?: unknown;
  signal?: AbortSignal;
  headers?: Record<string, string>;
}

export async function api<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = { Accept: "application/json", ...(options.headers ?? {}) };
  let body: string | undefined;
  if (options.body !== undefined) {
    headers["Content-Type"] = "application/json";
    body = JSON.stringify(options.body);
  }
  let response: Response;
  try {
    response = await fetch(`${API_BASE}${path}`, {
      method: options.method ?? "GET",
      headers,
      credentials: "include",
      body,
      signal: options.signal,
      cache: "no-store",
    });
  } catch (error) {
    if ((error as Error).name === "AbortError") throw error;
    throw new ApiError("network", 0, "network request failed");
  }
  const text = await response.text();
  let payload: unknown = null;
  if (text) {
    try {
      payload = JSON.parse(text);
    } catch {
      if (!response.ok) {
        throw new ApiError("internal", response.status, "invalid server response");
      }
    }
  }
  if (!response.ok) {
    const envelope = payload as ApiErrorEnvelope | null;
    throw new ApiError(
      envelope?.error?.code ?? "internal",
      response.status,
      envelope?.error?.message ?? `HTTP ${response.status}`,
      envelope?.error?.details,
    );
  }
  return payload as T;
}

export function reportUrl(projectId: string): string {
  return `${API_BASE}/projects/${encodeURIComponent(projectId)}/report`;
}

export function prettyJSON(value: unknown): string {
  try {
    return JSON.stringify(value, null, 2);
  } catch {
    return String(value);
  }
}
