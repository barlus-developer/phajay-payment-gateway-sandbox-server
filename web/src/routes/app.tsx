import { useEffect, useState } from "react";
import {
  Link,
  NavLink,
  Navigate,
  Outlet,
  Route,
  Routes,
  useNavigate,
} from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity,
  BookOpen,
  FolderKanban,
  KeyRound,
  LayoutDashboard,
  LogOut,
  Menu,
  QrCode,
  Settings,
  ShieldCheck,
  Webhook,
  X,
} from "lucide-react";
import { api, APIError, setCSRF } from "@/lib/api";
import type { Account, Project } from "@/lib/types";
import { PortalContext } from "@/hooks/portal";
import { Button } from "@/components/ui/button";
import { Choice, ErrorState, Loading } from "@/components/shared";
import { AuthPage } from "@/pages/auth";
import {
  Dashboard,
  Projects,
  APIKeys,
  Transactions,
  TransactionDetail,
  Webhooks,
  WebhookSecurity,
  APILogs,
  SettingsPage,
} from "@/pages/portal";
import { Docs } from "@/pages/docs";
import { Scanner, Pay, Checkout } from "@/pages/simulator";

const links = [
  { path: "", name: "Overview", icon: LayoutDashboard },
  { path: "/projects", name: "Projects", icon: FolderKanban },
  { path: "/api-keys", name: "API keys", icon: KeyRound },
  { path: "/transactions", name: "Transactions", icon: Activity },
  { path: "/webhooks", name: "Webhooks", icon: Webhook },
  { path: "/webhook-security", name: "Webhook security", icon: ShieldCheck },
  { path: "/api-logs", name: "API logs", icon: Activity },
  { path: "/docs", name: "Documentation", icon: BookOpen },
  { path: "/settings", name: "Settings", icon: Settings },
];
function Portal() {
  const navigate = useNavigate(),
    client = useQueryClient();
  const account = useQuery<Account, Error>({
    queryKey: ["account"],
    queryFn: () => api("/auth/me"),
    retry: false,
    refetchInterval: 60000,
  });
  const projects = useQuery<Project[], Error>({
    queryKey: ["projects"],
    queryFn: () => api("/projects"),
    enabled: Boolean(account.data),
  });
  const [selected, setSelected] = useState(
    () => localStorage.getItem("phajay-project") || "",
  );
  const [mobile, setMobile] = useState(false);
  useEffect(() => {
    setCSRF(account.data?.csrfToken || "");
  }, [account.data]);
  useEffect(() => {
    if (projects.data && !projects.data.some((p: Project) => p.id === selected))
      setSelected(projects.data[0]?.id || "");
  }, [projects.data, selected]);
  useEffect(() => {
    if (selected) localStorage.setItem("phajay-project", selected);
  }, [selected]);
  if (account.error instanceof APIError && account.error.status === 401)
    return <Navigate to="/login" replace />;
  if (account.error || projects.error)
    return (
      <div className="p-8">
        <ErrorState
          error={account.error || projects.error}
          retry={() => {
            account.refetch();
            projects.refetch();
          }}
        />
      </div>
    );
  if (!account.data || !projects.data)
    return (
      <div className="p-8">
        <Loading />
      </div>
    );
  const project = projects.data.find((p: Project) => p.id === selected);
  return (
    <PortalContext
      value={{
        account: account.data,
        projects: projects.data,
        project,
        select: setSelected,
      }}
    >
      <div className="min-h-screen bg-muted/50">
        <header className="sticky top-0 z-30 flex h-16 items-center justify-between border-b bg-background px-5 lg:hidden">
          <Link to="/portal" className="flex items-center gap-2 font-semibold">
            <QrCode className="text-primary" />
            Phajay{" "}
            <span className="text-xs text-muted-foreground">SANDBOX</span>
          </Link>
          <Button
            variant="ghost"
            size="icon"
            aria-label="Toggle navigation"
            onClick={() => setMobile(!mobile)}
          >
            {mobile ? <X /> : <Menu />}
          </Button>
        </header>
        <aside
          className={`${mobile ? "flex" : "hidden"} fixed inset-y-16 left-0 z-40 w-64 flex-col border-r bg-background p-5 shadow-lg lg:inset-y-0 lg:flex lg:shadow-none`}
        >
          <Link
            to="/portal"
            className="mb-8 flex items-center gap-3 text-lg font-semibold"
          >
            <div className="rounded-lg bg-primary p-2 text-primary-foreground">
              <QrCode className="size-5" />
            </div>
            Phajay
          </Link>
          <div className="mb-6">
            <Choice
              label="Sandbox project"
              value={selected}
              onChange={setSelected}
              options={projects.data.map((p: Project) => ({
                value: p.id,
                label: p.name,
              }))}
            />
          </div>
          <nav className="flex flex-col gap-1">
            {links.map(({ path, name, icon: Icon }) => (
              <NavLink
                key={path}
                to={`/portal${path}`}
                end={path === ""}
                onClick={() => setMobile(false)}
                className={({ isActive }) =>
                  `flex items-center gap-3 rounded-lg px-3 py-2.5 text-sm ${isActive ? "bg-accent font-medium text-accent-foreground" : "text-muted-foreground hover:bg-muted"}`
                }
              >
                <Icon className="size-4" />
                {name}
              </NavLink>
            ))}
          </nav>
          <div className="mt-auto pt-8">
            <Link
              to="/sandbox/scan"
              className="flex items-center gap-2 text-sm text-primary"
            >
              <QrCode className="size-4" />
              Open payment scanner
            </Link>
            <div className="mt-5 flex items-center justify-between border-t pt-5">
              <div className="min-w-0">
                <p className="truncate text-sm font-medium">
                  {account.data.developer.name}
                </p>
                <p className="truncate text-xs text-muted-foreground">
                  Sandbox developer
                </p>
              </div>
              <Button
                variant="ghost"
                size="icon"
                aria-label="Log out"
                onClick={async () => {
                  try {
                    await api("/auth/logout", "POST", {});
                    setCSRF("");
                    client.clear();
                    navigate("/login");
                  } catch {
                    account.refetch();
                  }
                }}
              >
                <LogOut />
              </Button>
            </div>
          </div>
        </aside>
        <div className="lg:ml-64">
          <div className="hidden h-16 items-center justify-between border-b bg-background px-8 lg:flex">
            <p className="text-sm text-muted-foreground">
              Developer workspace <span className="mx-2">/</span>
              <span className="font-medium text-foreground">
                {project?.name || "Create a project"}
              </span>
            </p>
            <span className="rounded-full border border-primary/20 bg-accent px-3 py-1 text-xs font-medium text-primary">
              Sandbox environment
            </span>
          </div>
          <main className="mx-auto max-w-7xl p-5 md:p-8">
            <Outlet />
          </main>
          <footer className="border-t px-8 py-4 text-xs text-muted-foreground">
            All payments are simulations. No banking connections or real money
            transfers.
          </footer>
        </div>
      </div>
    </PortalContext>
  );
}
export function App() {
  return (
    <Routes>
      <Route path="/" element={<Navigate to="/portal" replace />} />
      <Route path="/login" element={<AuthPage />} />
      <Route path="/register" element={<AuthPage register />} />
      <Route path="/portal" element={<Portal />}>
        <Route index element={<Dashboard />} />
        <Route path="projects" element={<Projects />} />
        <Route path="api-keys" element={<APIKeys />} />
        <Route path="transactions" element={<Transactions />} />
        <Route path="transactions/:id" element={<TransactionDetail />} />
        <Route path="webhooks" element={<Webhooks />} />
        <Route path="webhook-security" element={<WebhookSecurity />} />
        <Route path="api-logs" element={<APILogs />} />
        <Route path="docs" element={<Docs />} />
        <Route path="settings" element={<SettingsPage />} />
      </Route>
      <Route path="/sandbox/scan" element={<Scanner />} />
      <Route path="/sandbox/pay/:token" element={<Pay />} />
      <Route path="/sandbox/checkout/:transactionId" element={<Checkout />} />
      <Route
        path="*"
        element={
          <div className="p-10">
            <h1 className="page-heading">Page not found</h1>
            <Link to="/portal" className="text-primary">
              Return to portal
            </Link>
          </div>
        }
      />
    </Routes>
  );
}
