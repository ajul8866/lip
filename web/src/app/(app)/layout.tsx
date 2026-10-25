import { AppShell } from "@/components/AppShell";
import { ProjectProvider } from "@/components/ProjectProvider";

export default function AppLayout({ children }: { children: React.ReactNode }) {
  return (
    <ProjectProvider>
      <AppShell>{children}</AppShell>
    </ProjectProvider>
  );
}
