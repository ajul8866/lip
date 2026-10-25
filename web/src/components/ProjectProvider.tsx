"use client";

import { createContext, useCallback, useContext, useEffect, useMemo, useState } from "react";
import { ApiError, api } from "@/lib/api";
import type { ListResponse, Project } from "@/lib/types";
import { useAuth } from "./AuthProvider";

interface ProjectContextValue {
  projects: Project[];
  selected: Project | null;
  selectedId: string | null;
  loading: boolean;
  error: ApiError | null;
  select: (id: string) => void;
  reload: () => Promise<void>;
}

const ProjectContext = createContext<ProjectContextValue | null>(null);

const STORAGE_KEY = "lip_project";

export function ProjectProvider({ children }: { children: React.ReactNode }) {
  const { status } = useAuth();
  const [projects, setProjects] = useState<Project[]>([]);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<ApiError | null>(null);
  const [tick, setTick] = useState(0);

  const reload = useCallback(async () => {
    setTick((value) => value + 1);
  }, []);

  useEffect(() => {
    if (status !== "authenticated") {
      setLoading(status === "loading");
      return;
    }
    let cancelled = false;
    setLoading(true);
    api<ListResponse<Project>>("/projects?limit=200")
      .then((response) => {
        if (cancelled) return;
        setProjects(response.items);
        setSelectedId((current) => {
          const stored = typeof window !== "undefined" ? window.localStorage.getItem(STORAGE_KEY) : null;
          const candidate = current ?? stored;
          const found = response.items.find((project) => project.id === candidate);
          return found?.id ?? response.items[0]?.id ?? null;
        });
        setError(null);
      })
      .catch((err: unknown) => {
        if (cancelled) return;
        setError(err instanceof ApiError ? err : new ApiError("unknown", 0, String(err)));
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [status, tick]);

  const select = useCallback((id: string) => {
    setSelectedId(id);
    try {
      window.localStorage.setItem(STORAGE_KEY, id);
    } catch {
      // localStorage may be disabled.
    }
  }, []);

  const selected = projects.find((project) => project.id === selectedId) ?? null;
  const value = useMemo(
    () => ({ projects, selected, selectedId, loading, error, select, reload }),
    [projects, selected, selectedId, loading, error, select, reload],
  );

  return <ProjectContext.Provider value={value}>{children}</ProjectContext.Provider>;
}

export function useProjects(): ProjectContextValue {
  const value = useContext(ProjectContext);
  if (!value) throw new Error("useProjects must be used inside ProjectProvider");
  return value;
}
