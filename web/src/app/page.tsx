"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { useAuth } from "@/components/AuthProvider";
import { useApi } from "@/lib/hooks";
import type { SystemStatus } from "@/lib/types";
import { useI18n } from "@/i18n";
import { Icon, Skeleton } from "@/components/ui";

export default function RootPage() {
  const { status } = useAuth();
  const system = useApi<SystemStatus>("/system/status");
  const router = useRouter();
  const { t } = useI18n();

  useEffect(() => {
    if (status === "loading" || system.loading) return;
    if (system.data?.setupRequired) {
      router.replace("/setup");
      return;
    }
    if (status === "authenticated") {
      router.replace("/dashboard");
    } else {
      router.replace("/login");
    }
  }, [status, system.loading, system.data?.setupRequired, router]);

  return (
    <div className="boot-screen">
      <span className="brand-mark brand-mark-lg">
        <Icon name="shield" size={30} />
      </span>
      <h1>{system.data?.appName ?? t("app.name")}</h1>
      <p>{t("boot.loading")}</p>
      <Skeleton lines={2} className="boot-skeleton" />
    </div>
  );
}
