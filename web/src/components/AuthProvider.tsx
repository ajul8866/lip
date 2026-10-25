"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { ApiError, api } from "@/lib/api";
import type { MeResponse, User } from "@/lib/types";
import { useI18n } from "@/i18n";

type AuthStatus = "loading" | "authenticated" | "anonymous" | "error";

interface AuthContextValue {
  status: AuthStatus;
  user: User | null;
  sessionId: string | null;
  safeLocalOnly: boolean;
  safeModeSource: string;
  maxConcurrentTasks: number;
  autoRun: boolean;
  error: ApiError | null;
  refresh: () => Promise<void>;
  logout: () => Promise<void>;
  setUser: (user: User) => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const { setLocale } = useI18n();
  const [status, setStatus] = useState<AuthStatus>("loading");
  const [me, setMe] = useState<MeResponse | null>(null);
  const [error, setError] = useState<ApiError | null>(null);

  const refresh = useCallback(async () => {
    setStatus((current) => (current === "authenticated" ? current : "loading"));
    setError(null);
    try {
      const response = await api<MeResponse>("/auth/me");
      setMe(response);
      setStatus("authenticated");
      setLocale(response.user.locale, true);
    } catch (err) {
      if (err instanceof ApiError && err.status === 401) {
        setMe(null);
        setStatus("anonymous");
        return;
      }
      setError(err instanceof ApiError ? err : new ApiError("unknown", 0, String(err)));
      setStatus("error");
    }
  }, [setLocale]);

  useEffect(() => {
    void refresh();
  }, [refresh]);

  const logout = useCallback(async () => {
    try {
      await api("/auth/logout", { method: "POST" });
    } finally {
      setMe(null);
      setStatus("anonymous");
    }
  }, []);

  const setUser = useCallback((user: User) => {
    setMe((current) => (current ? { ...current, user } : current));
    setLocale(user.locale, true);
  }, [setLocale]);

  const value = useMemo<AuthContextValue>(
    () => ({
      status,
      user: me?.user ?? null,
      sessionId: me?.sessionId ?? null,
      safeLocalOnly: me?.safeLocalOnly ?? true,
      safeModeSource: me?.safeModeSource ?? "default",
      maxConcurrentTasks: me?.maxConcurrentTasks ?? 2,
      autoRun: me?.autoRun ?? true,
      error,
      refresh,
      logout,
      setUser,
    }),
    [status, me, error, refresh, logout, setUser],
  );

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const value = useContext(AuthContext);
  if (!value) throw new Error("useAuth must be used inside AuthProvider");
  return value;
}
