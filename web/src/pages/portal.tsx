import { useEffect, useState, type ReactNode } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  ArrowUpRight,
  CheckCheck,
  CircleDollarSign,
  Clock,
  FolderPlus,
  KeyRound,
  Plus,
  ShieldCheck,
  Webhook,
} from "lucide-react";
import { toast } from "sonner";
import { api, date, money } from "@/lib/api";
import type {
  APILog,
  DashboardData,
  Delivery,
  Endpoint,
  History,
  Job,
  Key,
  Page,
  Project,
  Session,
  Transaction,
} from "@/lib/types";
import { usePortal, useResource, useTask } from "@/hooks/portal";
import { Button } from "@/components/ui/button";
import { FieldGroup, Field, FieldLabel } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Busy,
  Choice,
  Confirm,
  Empty,
  ErrorState,
  Loading,
  Pagination,
  QR,
  Secret,
  Status,
  TextField,
  Title,
} from "@/components/shared";

function Rows<T extends { id: string }>({
  rows,
  columns,
}: {
  rows: T[];
  columns: { title: string; render: (row: T) => ReactNode }[];
}) {
  return (
    <div className="overflow-hidden rounded-xl border bg-card">
      <Table>
        <TableHeader>
          <TableRow>
            {columns.map((c) => (
              <TableHead key={c.title}>{c.title}</TableHead>
            ))}
          </TableRow>
        </TableHeader>
        <TableBody>
          {rows.map((row) => (
            <TableRow key={row.id}>
              {columns.map((c) => (
                <TableCell key={c.title}>{c.render(row)}</TableCell>
              ))}
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  );
}
function NeedProject() {
  return (
    <Empty title="Create your first sandbox project">
      <Link to="/portal/projects" className="mt-3 inline-block text-primary">
        Create a project →
      </Link>
    </Empty>
  );
}
function QueryState({
  query,
}: {
  query: { isPending: boolean; error: Error | null; refetch: () => unknown };
}) {
  if (query.error)
    return <ErrorState error={query.error} retry={() => query.refetch()} />;
  return <Loading />;
}
function TransactionRows({ rows }: { rows: Transaction[] }) {
  return (
    <Rows
      rows={rows}
      columns={[
        {
          title: "Transaction",
          render: (t) => (
            <Link
              className="font-mono text-xs text-primary"
              to={`/portal/transactions/${t.id}`}
            >
              {t.id.slice(0, 18)}…
            </Link>
          ),
        },
        {
          title: "Amount",
          render: (t) => (
            <span className="font-medium tabular-nums">
              {money(t.amountMinor)}
            </span>
          ),
        },
        { title: "Bank", render: (t) => t.bank },
        { title: "Status", render: (t) => <Status value={t.status} /> },
        {
          title: "Created",
          render: (t) => (
            <span className="text-xs text-muted-foreground">
              {date(t.createdAt)}
            </span>
          ),
        },
      ]}
    />
  );
}

export function Dashboard() {
  const { project } = usePortal(),
    q = useResource<DashboardData>("dashboard"),
    tx = useResource<Page<Transaction>>("transactions", "?limit=5");
  if (!project) return <NeedProject />;
  if (!q.data) return <QueryState query={q} />;
  const data = q.data,
    max = Math.max(1, ...data.activity.map((a) => a.total));
  return (
    <>
      <Title
        title="Sandbox overview"
        description={`Payment activity for ${project.merchantName}. All amounts are simulated.`}
        action={
          <Link
            to="/portal/docs"
            className="flex items-center gap-2 text-sm text-primary"
          >
            Integration guide <ArrowUpRight className="size-4" />
          </Link>
        }
      />
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {[
          {
            label: "Total transactions",
            value: data.total,
            icon: CircleDollarSign,
          },
          {
            label: "Completed payments",
            value: data.completed,
            icon: CheckCheck,
          },
          { label: "Pending payments", value: data.pending, icon: Clock },
          { label: "Unsuccessful payments", value: data.failed, icon: Webhook },
        ].map(({ label, value, icon: Icon }) => (
          <div className="panel" key={label}>
            <div className="flex items-center justify-between text-sm text-muted-foreground">
              {label}
              <Icon className="size-4" />
            </div>
            <p className="mt-4 text-3xl font-semibold tabular-nums">{value}</p>
          </div>
        ))}
      </div>
      <div className="mt-5 grid gap-5 lg:grid-cols-3">
        <section className="panel lg:col-span-2">
          <h2 className="font-semibold">Payment activity</h2>
          <p className="mt-1 text-xs text-muted-foreground">
            Last 7 days · transactions created in UTC
          </p>
          {data.activity.length ? (
            <div
              className="mt-6 flex h-44 items-end justify-around gap-3"
              role="img"
              aria-label="Daily transaction activity"
            >
              {data.activity.map((a) => (
                <div
                  key={a.day}
                  className="flex h-full flex-1 flex-col items-center justify-end gap-2"
                >
                  <span className="text-xs tabular-nums">{a.total}</span>
                  <div
                    style={{ height: `${Math.max(5, (a.total / max) * 100)}%` }}
                    className="w-full max-w-12 rounded-t bg-primary"
                    title={`${a.day}: ${a.total} transactions, ${a.completed} completed`}
                  />
                  <span className="text-[10px] text-muted-foreground">
                    {a.day.slice(5)}
                  </span>
                </div>
              ))}
            </div>
          ) : (
            <Empty title="No payment activity yet">
              Generate a payment using the SDK to get started.
            </Empty>
          )}
        </section>
        <section className="panel">
          <h2 className="font-semibold">Simulated volume</h2>
          <p className="mt-4 text-2xl font-semibold tabular-nums">
            {money(data.volumeMinor)}
          </p>
          <p className="mt-1 text-xs text-muted-foreground">
            Completed payments only
          </p>
          <div className="mt-7 flex flex-col gap-3 border-t pt-5 text-sm">
            <h3 className="font-medium">Webhook delivery</h3>
            <p className="flex justify-between">
              Delivered <span>{data.webhookDelivered}</span>
            </p>
            <p className="flex justify-between">
              Failed <span>{data.webhookFailed}</span>
            </p>
            <Link to="/portal/webhooks" className="text-primary">
              Inspect deliveries →
            </Link>
          </div>
        </section>
      </div>
      <section className="mt-7">
        <div className="mb-4 flex justify-between">
          <h2 className="font-semibold">Recent transactions</h2>
          <Link to="/portal/transactions" className="text-sm text-primary">
            View all
          </Link>
        </div>
        {tx.error ? (
          <ErrorState error={tx.error} />
        ) : tx.data?.items.length ? (
          <TransactionRows rows={tx.data.items} />
        ) : (
          <Empty title="No transactions yet">
            Follow the SDK guide to create your first sandbox payment.
          </Empty>
        )}
      </section>
    </>
  );
}

function ProjectForm({ value, close }: { value?: Project; close: () => void }) {
  const { select } = usePortal(),
    client = useQueryClient();
  const [name, setName] = useState(value?.name || ""),
    [merchant, setMerchant] = useState(value?.merchantName || ""),
    [description, setDescription] = useState(value?.description || "");
  const save = useMutation({
    mutationFn: () =>
      api<Project>(
        value ? `/projects/${value.id}` : "/projects",
        value ? "PATCH" : "POST",
        {
          name,
          merchantName: merchant,
          description,
          ...(value
            ? {
                paymentTTL: value.paymentTTL,
                webhookMaxAttempts: value.webhookMaxAttempts,
              }
            : {}),
        },
      ),
    onSuccess: (p: Project) => {
      client.setQueryData<Project[]>(
        ["projects"],
        (current: Project[] | undefined) => {
          const existing = current || [];
          return existing.some((item) => item.id === p.id)
            ? existing.map((item) => (item.id === p.id ? p : item))
            : [...existing, p];
        },
      );
      client.invalidateQueries({ queryKey: ["projects"] });
      select(p.id);
      toast.success("Project saved");
      close();
    },
  });
  return (
    <form
      className="flex flex-col gap-5"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <FieldGroup>
        <TextField
          label="Project name"
          required
          maxLength={100}
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <TextField
          label="Merchant display name"
          required
          maxLength={150}
          value={merchant}
          onChange={(e) => setMerchant(e.target.value)}
        />
        <TextField
          label="Description"
          maxLength={1000}
          value={description}
          onChange={(e) => setDescription(e.target.value)}
        />
      </FieldGroup>
      {save.error && <ErrorState error={save.error} />}
      <DialogFooter>
        <Button type="button" variant="outline" onClick={close}>
          Cancel
        </Button>
        <Busy pending={save.isPending} type="submit">
          Save project
        </Busy>
      </DialogFooter>
    </form>
  );
}
export function Projects() {
  const { projects, project, select } = usePortal();
  const [editor, setEditor] = useState<Project | "new" | null>(null);
  return (
    <>
      <Title
        title="Projects"
        description="Independent credentials, transactions, and webhooks for each integration."
        action={
          <Button onClick={() => setEditor("new")}>
            <FolderPlus />
            New project
          </Button>
        }
      />
      {projects.length ? (
        <div className="grid gap-5 md:grid-cols-2 xl:grid-cols-3">
          {projects.map((p) => (
            <section className="panel" key={p.id}>
              <div className="flex justify-between">
                <h2 className="font-semibold">{p.name}</h2>
                {p.id === project?.id && <Status value="ACTIVE" />}
              </div>
              <p className="mt-1 text-sm text-muted-foreground">
                {p.merchantName}
              </p>
              <p className="mt-5 min-h-10 text-sm">
                {p.description || "No description"}
              </p>
              <p className="mt-3 break-all font-mono text-xs text-muted-foreground">
                {p.id}
              </p>
              <div className="mt-5 flex gap-2">
                <Button
                  size="sm"
                  variant={p.id === project?.id ? "secondary" : "default"}
                  onClick={() => select(p.id)}
                >
                  {p.id === project?.id ? "Selected" : "Switch project"}
                </Button>
                <Button
                  size="sm"
                  variant="outline"
                  onClick={() => setEditor(p)}
                >
                  Edit
                </Button>
              </div>
            </section>
          ))}
        </div>
      ) : (
        <Empty title="Your workspace starts with a project">
          Create a project to generate sandbox credentials.
        </Empty>
      )}
      <Dialog
        open={editor !== null}
        onOpenChange={(open) => {
          if (!open) setEditor(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {editor === "new" ? "Create project" : "Edit project"}
            </DialogTitle>
            <DialogDescription>
              Merchant information appears on simulated checkout pages.
            </DialogDescription>
          </DialogHeader>
          {editor && (
            <ProjectForm
              key={typeof editor === "string" ? "new" : editor.id}
              value={typeof editor === "string" ? undefined : editor}
              close={() => setEditor(null)}
            />
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}

export function APIKeys() {
  const { project } = usePortal(),
    q = useResource<Key[]>("api-keys"),
    task = useTask(),
    client = useQueryClient();
  const [name, setName] = useState(""),
    [secret, setSecret] = useState("");
  const generate = useMutation({
    mutationFn: (rotate?: string) =>
      api<{ secret: string }>(
        `/projects/${project!.id}/api-keys${rotate ? `/${rotate}/rotate` : ""}`,
        "POST",
        { name: name || "Rotated SDK key" },
      ),
    onSuccess: (data: { secret: string }) => {
      setSecret(data.secret);
      setName("");
      client.invalidateQueries();
    },
  });
  if (!project) return <NeedProject />;
  return (
    <>
      <Title
        title="API keys"
        description="Sandbox credentials for SDK requests. New secrets are shown once."
      />
      <form
        className="panel mb-6 flex flex-wrap items-end gap-4"
        onSubmit={(e) => {
          e.preventDefault();
          generate.mutate(undefined);
        }}
      >
        <div className="min-w-60 flex-1">
          <TextField
            label="Key name"
            value={name}
            onChange={(e) => setName(e.target.value)}
            maxLength={100}
            required
            placeholder="Local integration"
          />
        </div>
        <Busy type="submit" pending={generate.isPending}>
          <KeyRound />
          Generate key
        </Busy>
      </form>
      {generate.error && <ErrorState error={generate.error} />}{" "}
      {!q.data ? (
        <QueryState query={q} />
      ) : !q.data.length ? (
        <Empty title="No API keys yet">
          Generate a key for your sandbox SDK client.
        </Empty>
      ) : (
        <Rows<Key>
          rows={q.data}
          columns={[
            { title: "Name", render: (k) => k.name },
            {
              title: "Key prefix",
              render: (k) => <code className="text-xs">{k.prefix}…</code>,
            },
            { title: "Created", render: (k) => date(k.createdAt) },
            { title: "Last used", render: (k) => date(k.lastUsedAt) },
            {
              title: "Status",
              render: (k) => (
                <Status value={k.revokedAt ? "REVOKED" : "ACTIVE"} />
              ),
            },
            {
              title: "Actions",
              render: (k) =>
                !k.revokedAt && (
                  <div className="flex gap-2">
                    <Confirm
                      title="Rotate API key?"
                      description="The current key will stop working immediately. Save the new secret after rotation."
                      action={() => generate.mutateAsync(k.id)}
                    >
                      Rotate
                    </Confirm>
                    <Confirm
                      title="Revoke API key?"
                      description="Requests using this key will be rejected."
                      action={() =>
                        task.mutateAsync({
                          path: `/projects/${project.id}/api-keys/${k.id}`,
                          method: "DELETE",
                        })
                      }
                    >
                      Revoke
                    </Confirm>
                  </div>
                ),
            },
          ]}
        />
      )}
      <Secret value={secret} close={() => setSecret("")} />
    </>
  );
}

export function Transactions() {
  const { project } = usePortal();
  const [search, setSearch] = useState(""),
    [status, setStatus] = useState(""),
    [bank, setBank] = useState(""),
    [offset, setOffset] = useState(0),
    [from, setFrom] = useState(""),
    [to, setTo] = useState("");
  const params = new URLSearchParams({
    search,
    status,
    bank,
    offset: String(offset),
    limit: "25",
  });
  if (from)
    params.set(
      "from",
      String(Math.floor(new Date(from + "T00:00:00").getTime() / 1000)),
    );
  if (to)
    params.set(
      "to",
      String(Math.floor(new Date(to + "T23:59:59").getTime() / 1000)),
    );
  const q = useResource<Page<Transaction>>("transactions", `?${params}`);
  if (!project) return <NeedProject />;
  const reset = (f: (v: string) => void) => (v: string) => {
    f(v);
    setOffset(0);
  };
  return (
    <>
      <Title
        title="Transactions"
        description="Track every simulated payment and inspect its history."
      />
      <div className="mb-5 grid gap-3 md:grid-cols-3 xl:grid-cols-5">
        <TextField
          label="Search reference"
          value={search}
          onChange={(e) => reset(setSearch)(e.target.value)}
          placeholder="Transaction or order number"
        />
        <Choice
          label="Status"
          value={status}
          onChange={reset(setStatus)}
          options={[
            { value: "", label: "All statuses" },
            ...[
              "WAITING",
              "PAYMENT_COMPLETED",
              "FAILED",
              "EXPIRED",
              "CANCELED",
            ].map((value) => ({ value, label: value.replaceAll("_", " ") })),
          ]}
        />
        <Choice
          label="Bank"
          value={bank}
          onChange={reset(setBank)}
          options={[
            { value: "", label: "All banks" },
            ...["JDB", "LDB", "IB", "BCEL", "STB"].map((value) => ({
              value,
              label: value,
            })),
          ]}
        />
        <TextField
          label="From date"
          type="date"
          value={from}
          onChange={(e) => reset(setFrom)(e.target.value)}
        />
        <TextField
          label="To date"
          type="date"
          value={to}
          onChange={(e) => reset(setTo)(e.target.value)}
        />
      </div>
      {!q.data ? (
        <QueryState query={q} />
      ) : !q.data.items.length ? (
        <Empty title="No matching transactions">
          Try another filter or create a payment using the SDK.
        </Empty>
      ) : (
        <TransactionRows rows={q.data.items} />
      )}{" "}
      {q.data && (
        <Pagination
          offset={offset}
          total={q.data.total}
          setOffset={setOffset}
        />
      )}
    </>
  );
}
export function TransactionDetail() {
  const { id } = useParams();
  const { project } = usePortal();
  const q = useResource<{ transaction: Transaction; history: History[] }>(
    `transactions/${id}`,
  );
  if (!project) return <NeedProject />;
  if (!q.data) return <QueryState query={q} />;
  const t = q.data.transaction;
  return (
    <>
      <Title
        title="Transaction details"
        description={t.id}
        action={
          <Link to="/portal/transactions" className="text-sm text-primary">
            ← Transactions
          </Link>
        }
      />
      <div className="grid gap-5 lg:grid-cols-3">
        <section className="panel lg:col-span-2">
          <div className="flex items-center justify-between">
            <h2 className="text-3xl font-semibold">
              {money(t.amountMinor, t.currency)}
            </h2>
            <Status value={t.status} />
          </div>
          <dl className="mt-7 grid gap-5 sm:grid-cols-2">
            {[
              ["Merchant", t.merchantName],
              ["Bank", t.bank],
              ["Order", t.orderNo],
              ["Description", t.description],
              ["Created", date(t.createdAt)],
              ["Expires", date(t.expiresAt)],
              ["Completed", date(t.completedAt)],
              ["Receipt", t.completedAt ? t.receiptRef : "—"],
              ["Tag 1", t.tag1 || "—"],
              ["Tag 2", t.tag2 || "—"],
              ["Tag 3", t.tag3 || "—"],
            ].map(([label, value]) => (
              <div key={label}>
                <dt className="text-xs text-muted-foreground">{label}</dt>
                <dd className="mt-1 break-all text-sm">{value}</dd>
              </div>
            ))}
          </dl>
          <h3 className="mt-8 border-t pt-5 font-semibold">
            Transaction history
          </h3>
          <ol className="mt-4 flex flex-col gap-4">
            {q.data.history.map((e) => (
              <li
                className="flex flex-wrap items-center justify-between gap-2"
                key={e.id}
              >
                <Status value={e.status} />
                <span className="text-xs text-muted-foreground">
                  {date(e.createdAt)}
                </span>
              </li>
            ))}
          </ol>
        </section>
        <section className="panel text-center">
          <h2 className="font-semibold">Sandbox payment QR</h2>
          <QR value={t.paymentURL} />
          <p className="mb-5 text-xs text-muted-foreground">
            Scan with the sandbox scanner. Scanning does not confirm payment.
          </p>
          <a
            href={t.checkoutURL}
            target="_blank"
            rel="noreferrer"
            className="text-sm text-primary"
          >
            Open live checkout ↗
          </a>
          <p className="mt-4 text-xs font-medium text-muted-foreground">
            SANDBOX PAYMENT — NOT A REAL TRANSACTION
          </p>
        </section>
      </div>
    </>
  );
}

const eventTypes = [
  "payment.completed",
  "payment.failed",
  "payment.canceled",
  "payment.expired",
];
export function Webhooks() {
  const { project } = usePortal(),
    q = useResource<Endpoint[]>("webhooks"),
    [jobOffset, setJobOffset] = useState(0),
    jobs = useResource<Job[]>("webhook-jobs", `?offset=${jobOffset}`),
    task = useTask(),
    client = useQueryClient();
  const [editor, setEditor] = useState<Endpoint | "new" | null>(null),
    [url, setURL] = useState(""),
    [enabled, setEnabled] = useState(true),
    [events, setEvents] = useState<string[]>(["payment.completed"]),
    [inspect, setInspect] = useState("");
  const attempts = useQuery<Delivery[], Error>({
    queryKey: ["project", project?.id, "deliveries", inspect],
    queryFn: () =>
      api(`/projects/${project!.id}/webhook-jobs/${inspect}/deliveries`),
    enabled: Boolean(inspect && project),
  });
  const save = useMutation({
    mutationFn: () =>
      api(
        `/projects/${project!.id}/webhooks${typeof editor === "object" && editor ? `/${editor.id}` : ""}`,
        typeof editor === "object" ? "PATCH" : "POST",
        { url, enabled, events },
      ),
    onSuccess: () => {
      client.invalidateQueries();
      setEditor(null);
      toast.success("Webhook saved");
    },
  });
  const edit = (value: Endpoint | "new") => {
    setURL(value === "new" ? "" : value.url);
    setEnabled(value === "new" ? true : value.enabled);
    setEvents(value === "new" ? ["payment.completed"] : value.events);
    setEditor(value);
  };
  if (!project) return <NeedProject />;
  return (
    <>
      <Title
        title="Webhooks"
        description="Signed callbacks with durable delivery, automatic retries, and attempt history."
        action={
          <Button onClick={() => edit("new")}>
            <Plus />
            Add endpoint
          </Button>
        }
      />
      {!q.data ? (
        <QueryState query={q} />
      ) : !q.data.length ? (
        <Empty title="No webhook endpoints">
          Add a callback URL and generate a signing key.
        </Empty>
      ) : (
        <Rows<Endpoint>
          rows={q.data}
          columns={[
            {
              title: "Destination",
              render: (e) => (
                <div>
                  <p className="max-w-lg truncate font-mono text-xs">{e.url}</p>
                  <p className="mt-1 text-xs text-muted-foreground">
                    {e.events.join(", ")}
                  </p>
                </div>
              ),
            },
            {
              title: "Status",
              render: (e) => (
                <Status value={e.enabled ? "ENABLED" : "DISABLED"} />
              ),
            },
            {
              title: "Actions",
              render: (e) => (
                <div className="flex gap-2">
                  <Button size="sm" variant="outline" onClick={() => edit(e)}>
                    Edit
                  </Button>
                  <Button
                    size="sm"
                    variant="outline"
                    disabled={!e.enabled || task.isPending}
                    onClick={() =>
                      task.mutate({
                        path: `/projects/${project.id}/webhooks/${e.id}/test`,
                      })
                    }
                  >
                    Send test
                  </Button>
                  {e.enabled && (
                    <Confirm
                      title="Disable endpoint?"
                      description="Pending deliveries to this endpoint will stop."
                      action={() =>
                        task.mutateAsync({
                          path: `/projects/${project.id}/webhooks/${e.id}`,
                          method: "DELETE",
                        })
                      }
                    >
                      Disable
                    </Confirm>
                  )}
                </div>
              ),
            },
          ]}
        />
      )}
      <section className="mt-8">
        <h2 className="mb-4 font-semibold">Delivery events</h2>
        {jobs.error ? (
          <ErrorState error={jobs.error} />
        ) : jobs.data?.length ? (
          <Rows<Job>
            rows={jobs.data}
            columns={[
              {
                title: "Event",
                render: (j) => (
                  <code className="text-xs">{j.eventId.slice(0, 18)}…</code>
                ),
              },
              {
                title: "Destination",
                render: (j) => (
                  <span className="inline-block max-w-64 truncate text-xs">
                    {j.url}
                  </span>
                ),
              },
              { title: "Status", render: (j) => <Status value={j.status} /> },
              { title: "Attempts", render: (j) => j.attempts },
              {
                title: "Actions",
                render: (j) => (
                  <div className="flex gap-2">
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => setInspect(j.id)}
                    >
                      Inspect
                    </Button>
                    {["FAILED", "DELIVERED", "DISABLED"].includes(j.status) && (
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={task.isPending}
                        onClick={() =>
                          task.mutate({
                            path: `/projects/${project.id}/webhook-jobs/${j.id}/retry`,
                          })
                        }
                      >
                        Retry
                      </Button>
                    )}
                  </div>
                ),
              },
            ]}
          />
        ) : (
          <Empty title="No deliveries yet">
            Send a test event or complete a sandbox payment.
          </Empty>
        )}
        <div className="mt-4 flex justify-end gap-2">
          <Button
            variant="outline"
            disabled={!jobOffset}
            onClick={() => setJobOffset(Math.max(0, jobOffset - 25))}
          >
            Previous
          </Button>
          <Button
            variant="outline"
            disabled={(jobs.data?.length || 0) < 25}
            onClick={() => setJobOffset(jobOffset + 25)}
          >
            Next
          </Button>
        </div>
      </section>
      <Dialog
        open={editor !== null}
        onOpenChange={(v) => {
          if (!v) setEditor(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {editor === "new"
                ? "Add webhook endpoint"
                : "Edit webhook endpoint"}
            </DialogTitle>
            <DialogDescription>
              Use an HTTPS destination. Local callbacks require explicit server
              development configuration.
            </DialogDescription>
          </DialogHeader>
          <form
            className="flex flex-col gap-5"
            onSubmit={(e) => {
              e.preventDefault();
              save.mutate();
            }}
          >
            <FieldGroup>
              <TextField
                label="Callback URL"
                type="url"
                required
                value={url}
                onChange={(e) => setURL(e.target.value)}
                maxLength={2048}
              />
              <Field>
                <FieldLabel>Event subscriptions</FieldLabel>
                {eventTypes.map((event) => (
                  <label
                    className="flex items-center gap-2 text-sm"
                    key={event}
                  >
                    <input
                      type="checkbox"
                      checked={events.includes(event)}
                      onChange={(e) =>
                        setEvents((current) =>
                          e.target.checked
                            ? [...current, event]
                            : current.filter((v) => v !== event),
                        )
                      }
                    />
                    {event}
                  </label>
                ))}
              </Field>
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={enabled}
                  onChange={(e) => setEnabled(e.target.checked)}
                />
                Enable delivery
              </label>
            </FieldGroup>
            {save.error && <ErrorState error={save.error} />}
            <DialogFooter>
              <Busy
                pending={save.isPending}
                type="submit"
                disabled={!events.length}
              >
                Save endpoint
              </Busy>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
      <Dialog
        open={Boolean(inspect)}
        onOpenChange={(v) => {
          if (!v) setInspect("");
        }}
      >
        <DialogContent className="max-h-[85vh] overflow-auto">
          <DialogHeader>
            <DialogTitle>Delivery attempts</DialogTitle>
            <DialogDescription>
              Sanitized responses and delivery errors. Each retry retains the
              same event ID.
            </DialogDescription>
          </DialogHeader>
          {attempts.error ? (
            <ErrorState error={attempts.error} />
          ) : attempts.data?.length ? (
            attempts.data.map((a: Delivery) => (
              <section className="rounded-lg border p-4" key={a.id}>
                <div className="flex justify-between text-sm">
                  <strong>
                    Attempt {a.attempt} · HTTP {a.httpStatus || "—"}
                  </strong>
                  <span>{a.durationMS} ms</span>
                </div>
                <p className="mt-1 text-xs text-muted-foreground">
                  {date(a.createdAt)}
                </p>
                {a.error && (
                  <p className="mt-3 text-sm text-destructive">{a.error}</p>
                )}
                {a.response && (
                  <pre className="code mt-3 whitespace-pre-wrap break-all">
                    {a.response}
                  </pre>
                )}
              </section>
            ))
          ) : (
            <Empty title="No attempts recorded yet" />
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}
export function WebhookSecurity() {
  const { project } = usePortal(),
    q = useResource<Key[]>("signing-keys"),
    task = useTask(),
    client = useQueryClient();
  const [secret, setSecret] = useState("");
  const rotate = useMutation({
    mutationFn: () =>
      api<{ secret: string }>(
        `/projects/${project!.id}/signing-keys`,
        "POST",
        {},
      ),
    onSuccess: (data: { secret: string }) => {
      setSecret(data.secret);
      client.invalidateQueries();
    },
  });
  if (!project) return <NeedProject />;
  return (
    <>
      <Title
        title="Webhook security"
        description="Project-specific HMAC signing keys, encrypted at rest. Verification is enabled by default."
        action={
          <Busy pending={rotate.isPending} onClick={() => rotate.mutate()}>
            <ShieldCheck />
            Generate or rotate key
          </Busy>
        }
      />
      <div className="panel mb-6 text-sm leading-relaxed">
        <h2 className="font-semibold">Sandbox signature protocol</h2>
        <p className="mt-2 text-muted-foreground">
          HMAC-SHA256 signs <code>timestamp.rawRequestBody</code>. Headers carry
          the Unix timestamp, lowercase <code>v1=</code> signature, and unique
          event ID. Verification allows a five-minute timestamp tolerance.
        </p>
        <p className="mt-3 text-muted-foreground">
          Rotated keys have a five-minute overlap. Keep the previous verifier
          only until its listed valid-until time. Revocation ends that overlap
          immediately. This is a custom sandbox protocol.
        </p>
        <Link to="/portal/docs" className="mt-4 inline-block text-primary">
          SDK fork and verification example →
        </Link>
      </div>
      {rotate.error && <ErrorState error={rotate.error} />}{" "}
      {!q.data ? (
        <QueryState query={q} />
      ) : q.data.length ? (
        <Rows<Key>
          rows={q.data}
          columns={[
            { title: "Key", render: (k) => <code>{k.prefix}…</code> },
            { title: "Created", render: (k) => date(k.createdAt) },
            {
              title: "Status",
              render: (k) => (
                <Status
                  value={
                    k.revokedAt
                      ? "REVOKED"
                      : k.active
                        ? "ACTIVE"
                        : (k.validUntil || 0) > Date.now() / 1000
                          ? "ROTATION OVERLAP"
                          : "RETIRED"
                  }
                />
              ),
            },
            {
              title: "Valid until",
              render: (k) =>
                k.active ? "Until rotated or revoked" : date(k.validUntil),
            },
            {
              title: "Actions",
              render: (k) =>
                !k.revokedAt && (
                  <Confirm
                    title="Revoke signing key?"
                    description="Remove this key from your callback verifier. Revoking the active key prevents delivery until a replacement is generated."
                    action={() =>
                      task.mutateAsync({
                        path: `/projects/${project.id}/signing-keys/${k.id}`,
                        method: "DELETE",
                      })
                    }
                  >
                    Revoke
                  </Confirm>
                ),
            },
          ]}
        />
      ) : (
        <Empty title="No signing key configured">
          Generate a signing key before webhook delivery.
        </Empty>
      )}
      <Secret value={secret} close={() => setSecret("")} />
    </>
  );
}
export function APILogs() {
  const { project } = usePortal(),
    [offset, setOffset] = useState(0),
    q = useResource<APILog[]>("api-logs", `?offset=${offset}`);
  if (!project) return <NeedProject />;
  return (
    <>
      <Title
        title="API request history"
        description="SDK requests associated with this project. Credentials and request bodies are never recorded."
      />
      {!q.data ? (
        <QueryState query={q} />
      ) : q.data.length ? (
        <Rows<APILog>
          rows={q.data}
          columns={[
            {
              title: "Request",
              render: (l) => (
                <code className="text-xs">
                  {l.method} {l.path}
                </code>
              ),
            },
            {
              title: "Response",
              render: (l) => <Status value={String(l.status)} />,
            },
            { title: "Duration", render: (l) => `${l.durationMS} ms` },
            { title: "Timestamp", render: (l) => date(l.createdAt) },
            { title: "Error", render: (l) => l.error || "—" },
          ]}
        />
      ) : (
        <Empty title="No requests recorded">
          Create a payment through your project's API key.
        </Empty>
      )}
      <div className="mt-4 flex justify-end gap-2">
        <Button
          variant="outline"
          disabled={!offset}
          onClick={() => setOffset(Math.max(0, offset - 25))}
        >
          Previous
        </Button>
        <Button
          variant="outline"
          disabled={(q.data?.length || 0) < 25}
          onClick={() => setOffset(offset + 25)}
        >
          Next
        </Button>
      </div>
    </>
  );
}
export function SettingsPage() {
  const { account, project } = usePortal(),
    task = useTask();
  const [name, setName] = useState(account.developer.name),
    [current, setCurrent] = useState(""),
    [next, setNext] = useState(""),
    [ttl, setTTL] = useState(project?.paymentTTL || 900),
    [attempts, setAttempts] = useState(project?.webhookMaxAttempts || 8);
  const sessions = useQuery<Session[], Error>({
    queryKey: ["sessions"],
    queryFn: () => api("/sessions"),
  });
  useEffect(() => {
    setTTL(project?.paymentTTL || 900);
    setAttempts(project?.webhookMaxAttempts || 8);
  }, [project]);
  return (
    <>
      <Title
        title="Settings"
        description="Account security, active sessions, and sandbox project behavior."
      />
      <div className="grid gap-6 lg:grid-cols-2">
        <form
          className="panel flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            task.mutate({ path: "/account", method: "PATCH", body: { name } });
          }}
        >
          <h2 className="font-semibold">Developer account</h2>
          <TextField
            label="Display name"
            required
            value={name}
            onChange={(e) => setName(e.target.value)}
            maxLength={100}
          />
          <TextField label="Email" readOnly value={account.developer.email} />
          <Busy pending={task.isPending} type="submit">
            Save account
          </Busy>
        </form>
        <form
          className="panel flex flex-col gap-4"
          onSubmit={async (e) => {
            e.preventDefault();
            try {
              await task.mutateAsync({
                path: "/account/password",
                body: { currentPassword: current, newPassword: next },
              });
              setCurrent("");
              setNext("");
            } catch {
              /* mutation shows error */
            }
          }}
        >
          <h2 className="font-semibold">Change password</h2>
          <TextField
            label="Current password"
            type="password"
            autoComplete="current-password"
            required
            value={current}
            onChange={(e) => setCurrent(e.target.value)}
            maxLength={128}
          />
          <TextField
            label="New password"
            type="password"
            autoComplete="new-password"
            required
            minLength={12}
            maxLength={128}
            value={next}
            onChange={(e) => setNext(e.target.value)}
          />
          <p className="text-xs text-muted-foreground">
            Other sessions are revoked after a password change.
          </p>
          <Busy pending={task.isPending} type="submit">
            Update password
          </Busy>
        </form>
        {project && (
          <form
            className="panel flex flex-col gap-4"
            onSubmit={(e) => {
              e.preventDefault();
              task.mutate({
                path: `/projects/${project.id}`,
                method: "PATCH",
                body: {
                  ...project,
                  paymentTTL: ttl,
                  webhookMaxAttempts: attempts,
                },
              });
            }}
          >
            <h2 className="font-semibold">Project configuration</h2>
            <TextField
              label="Default expiration (seconds)"
              type="number"
              min={60}
              max={86400}
              required
              value={ttl}
              onChange={(e) => setTTL(Number(e.target.value))}
            />
            <TextField
              label="Webhook maximum attempts"
              type="number"
              min={1}
              max={12}
              required
              value={attempts}
              onChange={(e) => setAttempts(Number(e.target.value))}
            />
            <p className="text-xs text-muted-foreground">
              Changes apply to new payments and outgoing events.
            </p>
            <Busy pending={task.isPending} type="submit">
              Save configuration
            </Busy>
          </form>
        )}
      </div>
      <section className="mt-7">
        <h2 className="mb-4 font-semibold">Active sessions</h2>
        {sessions.error ? (
          <ErrorState error={sessions.error} />
        ) : (
          sessions.data && (
            <Rows<Session>
              rows={sessions.data}
              columns={[
                {
                  title: "Session",
                  render: (s) =>
                    s.current ? "Current session" : s.id.slice(0, 18) + "…",
                },
                { title: "Created", render: (s) => date(s.createdAt) },
                { title: "Expires", render: (s) => date(s.expiresAt) },
                {
                  title: "Actions",
                  render: (s) =>
                    !s.current && (
                      <Confirm
                        title="Revoke session?"
                        description="This device will need to log in again."
                        action={() =>
                          task.mutateAsync({
                            path: `/sessions/${s.id}`,
                            method: "DELETE",
                          })
                        }
                      >
                        Revoke
                      </Confirm>
                    ),
                },
              ]}
            />
          )
        )}
      </section>
    </>
  );
}
