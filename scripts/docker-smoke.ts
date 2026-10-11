// Run with Bun on a host with Docker access and a validated web/bun.lock.
// Owns an isolated Compose project and its disposable test volume.
import { randomBytes } from "node:crypto";

const project = `phajay-test-${Date.now()}`;
const port = "18080";
const origin = `http://127.0.0.1:${port}`;
const env = {
  ...process.env,
  MASTER_KEY: randomBytes(32).toString("base64"),
  PORT: port,
  PUBLIC_URL: origin,
  APP_ENV: "development",
  ALLOW_LOCAL_WEBHOOKS: "true",
};
let cookies = "",
  csrf = "";
async function compose(...args: string[]) {
  const child = Bun.spawn(["docker", "compose", "-p", project, ...args], {
    env,
    stdout: "inherit",
    stderr: "inherit",
  });
  if ((await child.exited) !== 0)
    throw new Error(`Compose operation failed: ${args[0]}`);
}
async function call<T>(
  path: string,
  method = "GET",
  body?: unknown,
  sdkHeader?: Record<string, string>,
): Promise<T> {
  const response = await fetch(origin + path, {
    method,
    headers: {
      "Content-Type": "application/json",
      Origin: origin,
      "X-Phajay-Portal": "1",
      ...(cookies ? { Cookie: cookies } : {}),
      ...(csrf ? { "X-CSRF-Token": csrf } : {}),
      ...sdkHeader,
    },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
  });
  const value = await response.json();
  if (!response.ok) throw new Error(`HTTP ${response.status} on ${path}`);
  if (path.endsWith("/auth/login")) {
    cookies = response.headers
      .getSetCookie()
      .map((c) => c.split(";")[0])
      .join("; ");
    csrf = value.data.csrfToken;
  }
  return path.startsWith("/v1/") ? value : value.data;
}
async function waitFor<T>(
  action: () => Promise<T>,
  accept: (value: T) => boolean,
) {
  for (let i = 0; i < 40; i++) {
    try {
      const value = await action();
      if (accept(value)) return value;
    } catch {}
    await Bun.sleep(500);
  }
  throw new Error("Timed out waiting for smoke-test state");
}
function assert(value: unknown, message: string): asserts value {
  if (!value) throw new Error(message);
}
try {
  await compose("up", "--build", "-d");
  await waitFor(
    async () => fetch(origin + "/readyz").then((r) => r.status),
    (s) => s === 200,
  );
  const html = await fetch(origin + "/register").then((r) => r.text());
  const asset = html.match(/src="(\/assets\/[^\"]+\.js)"/)?.[1];
  assert(asset, "Compiled SPA asset missing");
  assert((await fetch(origin + asset)).ok, "Frontend asset unavailable");
  const email = `smoke-${Date.now()}@example.test`,
    password = randomBytes(24).toString("hex");
  await call("/api/internal/v1/auth/register", "POST", {
    email,
    name: "Smoke developer",
    password,
  });
  await call("/api/internal/v1/auth/login", "POST", { email, password });
  const merchant = await call<{ id: string }>(
    "/api/internal/v1/projects",
    "POST",
    { name: "Smoke project", merchantName: "Simulated merchant" },
  );
  const base = `/api/internal/v1/projects/${merchant.id}`;
  const key = await call<{ secret: string }>(base + "/api-keys", "POST", {
    name: "Smoke SDK",
  });
  await call(base + "/signing-keys", "POST", {});
  await call(base + "/webhooks", "POST", {
    url: "http://127.0.0.1:8080/healthz",
    enabled: true,
    events: ["payment.completed"],
  }); // POST returns 404, intentionally testing durable retry.
  const qr = await call<{
    transactionId: string;
    qrCode: string;
    link: string;
  }>(
    "/v1/api/test/payment/generate-jdb-qr",
    "POST",
    { amount: 12.34, description: "Docker smoke" },
    { secretKey: key.secret },
  );
  const token = new URL(qr.link).pathname.split("/").at(-1)!;
  const waiting = await call<{ status: string }>(
    "/api/internal/v1/simulator/resolve",
    "POST",
    { reference: qr.qrCode },
  );
  assert(waiting.status === "WAITING", "Scanning completed a payment");
  const paid = await call<{ status: string }>(
    `/api/internal/v1/simulator/payments/${token}/confirm`,
    "POST",
    { status: "PAYMENT_COMPLETED" },
  );
  assert(paid.status === "PAYMENT_COMPLETED", "Confirmation failed");
  const job = await waitFor(
    () =>
      call<{ id: string; eventId: string; attempts: number; status: string }[]>(
        base + "/webhook-jobs",
      ),
    (jobs) => jobs.length === 1 && jobs[0].attempts >= 1,
  );
  await compose("down");
  await compose("up", "-d"); // Recreates container, preserves this test's volume.
  await waitFor(
    async () => fetch(origin + "/readyz").then((r) => r.status),
    (s) => s === 200,
  );
  const projects = await call<{ id: string }[]>("/api/internal/v1/projects");
  assert(
    projects.some((p) => p.id === merchant.id),
    "Project lost after recreation",
  );
  const recovered = await waitFor(
    () =>
      call<{ id: string; eventId: string; attempts: number }[]>(
        base + "/webhook-jobs",
      ),
    (jobs) => jobs.length === 1 && jobs[0].attempts > job[0].attempts,
  );
  assert(
    recovered[0].eventId === job[0].eventId,
    "Restart changed webhook event ID",
  );
  const tx = await call<{ transaction: { status: string } }>(
    base + `/transactions/${qr.transactionId}`,
  );
  assert(
    tx.transaction.status === "PAYMENT_COMPLETED",
    "Transaction lost after restart",
  );
  console.log(
    "Docker smoke checks passed: assets, migrations, auth, payment, persistence, webhook recovery.",
  );
} finally {
  await compose("down", "--volumes");
}
