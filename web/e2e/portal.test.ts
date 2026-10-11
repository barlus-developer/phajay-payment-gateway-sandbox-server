// Browser flows run in Bun against the real Go server and a disposable SQLite DB.
// Install the pinned Playwright browser with: bun --bun playwright install chromium
import {
  afterAll,
  afterEach,
  beforeAll,
  beforeEach,
  expect,
  test,
} from "bun:test";
import {
  chromium,
  type Browser,
  type BrowserContext,
  type Page,
} from "playwright";
import QRCode from "qrcode";
import { mkdtemp, rm } from "node:fs/promises";
import { randomBytes } from "node:crypto";
import { tmpdir } from "node:os";
import { join } from "node:path";

const root = new URL("../../", import.meta.url).pathname;
const origin = "http://127.0.0.1:18081";
const email = `browser-${randomBytes(8).toString("hex")}@example.test`;
let registered = false;
let directory = "",
  browser: Browser,
  context: BrowserContext,
  page: Page,
  server: ReturnType<typeof Bun.spawn>;
async function json(path: string, method = "GET", body?: unknown) {
  return page.evaluate(
    async ({ path, method, body }) => {
      const csrf =
        document.cookie
          .split("; ")
          .find((c) => c.startsWith("phajay_csrf="))
          ?.split("=")
          .slice(1)
          .join("=") || "";
      const response = await fetch("/api/internal/v1" + path, {
        method,
        headers: {
          "Content-Type": "application/json",
          "X-Phajay-Portal": "1",
          "X-CSRF-Token": csrf,
        },
        ...(body === undefined ? {} : { body: JSON.stringify(body) }),
      });
      if (!response.ok)
        throw new Error(`Request failed with ${response.status}`);
      return (await response.json()).data;
    },
    { path, method, body },
  );
}
async function createProject(name: string) {
  await page.goto(origin + "/portal/projects");
  await page.getByRole("button", { name: "New project" }).click();
  await page.getByLabel("Project name", { exact: true }).fill(name);
  await page
    .getByLabel("Merchant display name", { exact: true })
    .fill(name + " Merchant");
  await page.getByRole("button", { name: "Save project" }).click();
  await page.getByRole("dialog").waitFor({ state: "hidden" });
  return (await json("/projects")).find(
    (p: { name: string }) => p.name === name,
  ) as { id: string };
}
async function keyFor(project: string) {
  const value = await json(`/projects/${project}/api-keys`, "POST", {
    name: "Browser SDK",
  });
  return value.secret as string;
}
async function createQR(key: string, amount = 12.34) {
  const response = await context.request.post(
    origin + "/v1/api/test/payment/generate-jdb-qr",
    {
      headers: { secretKey: key },
      data: { amount, description: "Browser integration" },
    },
  );
  expect(response.status()).toBe(200);
  return (await response.json()) as {
    transactionId: string;
    qrCode: string;
    link: string;
  };
}
beforeAll(async () => {
  directory = await mkdtemp(join(tmpdir(), "phajay-browser-"));
  const binary = join(directory, "server");
  const build = Bun.spawn(["go", "build", "-o", binary, "./cmd/server"], {
    cwd: root,
    stdout: "inherit",
    stderr: "inherit",
  });
  if ((await build.exited) !== 0)
    throw new Error("Go browser-test build failed");
  server = Bun.spawn([binary], {
    cwd: root,
    env: {
      ...process.env,
      HTTP_ADDR: "127.0.0.1:18081",
      PUBLIC_URL: origin,
      MASTER_KEY: randomBytes(32).toString("base64"),
      DATABASE_PATH: join(directory, "app.db"),
      APP_ENV: "development",
      ALLOW_LOCAL_WEBHOOKS: "true",
      WEB_DIST_DIR: join(root, "web/dist"),
      MIGRATIONS_DIR: join(root, "migrations"),
    },
    stdout: "ignore",
    stderr: "inherit",
  });
  let ready = false;
  for (let i = 0; i < 80; i++) {
    try {
      if ((await fetch(origin + "/readyz")).ok) {
        ready = true;
        break;
      }
    } catch {}
    await Bun.sleep(250);
  }
  if (!ready) throw new Error("Browser-test server did not start");
  browser = await chromium.launch({ headless: true });
}, 60000);
beforeEach(async () => {
  context = await browser.newContext({
    viewport: { width: 1280, height: 900 },
  });
  page = await context.newPage();
  await page.goto(origin + (registered ? "/login" : "/register"));
  if (!registered) await page.getByLabel("Your name").fill("Browser Developer");
  await page.getByLabel("Email address").fill(email);
  await page
    .getByLabel("Password", { exact: true })
    .fill("browser-long-password");
  await page
    .getByRole("button", {
      name: registered ? "Sign in" : "Create account",
      exact: true,
    })
    .click();
  await page.waitForURL(origin + "/portal");
  registered = true;
  await page.getByRole("button", { name: "Log out" }).waitFor();
});
afterEach(async () => {
  await context?.close();
});
afterAll(async () => {
  await browser?.close();
  if (server) {
    server.kill("SIGTERM");
    await server.exited;
  }
  if (directory) await rm(directory, { recursive: true, force: true });
});

test("registration, session settings and logout require authentication", async () => {
  await page.goto(origin + "/portal/settings");
  await page.getByLabel("Display name").fill("Updated Developer");
  await page.getByRole("button", { name: "Save account" }).click();
  await page.getByText("Updated Developer", { exact: true }).first().waitFor();
  expect((await json("/auth/me")).developer.name).toBe("Updated Developer");
  await page.getByRole("button", { name: "Log out" }).click();
  await page.waitForURL(origin + "/login");
  await page.goto(origin + "/portal/settings");
  await page.waitForURL(origin + "/login");
});
test("new project selection and project switching use real isolated data", async () => {
  await createProject("Alpha");
  const beta = await createProject("Beta");
  expect(
    await page.getByRole("combobox", { name: "Sandbox project" }).innerText(),
  ).toContain("Beta");
  await page.getByRole("combobox", { name: "Sandbox project" }).click();
  await page.getByRole("option", { name: "Alpha", exact: true }).click();
  await page.getByRole("link", { name: "Overview", exact: true }).click();
  await page
    .getByText("Payment activity for Alpha Merchant.", { exact: false })
    .waitFor();
  await json(`/projects/${beta.id}/api-keys`, "POST", { name: "Beta only" });
  await page.goto(origin + "/portal/api-keys");
  expect(await page.getByText("Beta only", { exact: true }).count()).toBe(0);
});
test("API key reveal, rotation and revocation work without persistent secret reveal", async () => {
  const project = await createProject("Keys");
  await page.goto(origin + "/portal/api-keys");
  await page.getByLabel("Key name").fill("Browser key");
  await page.getByRole("button", { name: "Generate key" }).click();
  await page.getByRole("dialog").waitFor();
  const secret = await page.getByLabel("New secret").inputValue();
  expect(secret.startsWith("sk_sandbox_")).toBe(true);
  await page.getByRole("button", { name: "Saved securely" }).click();
  expect((await page.locator("body").innerText()).includes(secret)).toBe(false);
  await createQR(secret);
  await page
    .getByRole("button", { name: "Rotate", exact: true })
    .first()
    .click();
  await page.getByRole("button", { name: "Confirm", exact: true }).click();
  await page.getByLabel("New secret").waitFor();
  const next = await page.getByLabel("New secret").inputValue();
  await page.getByRole("button", { name: "Saved securely" }).click();
  expect(
    (
      await context.request.post(
        origin + "/v1/api/test/payment/generate-jdb-qr",
        {
          headers: { secretKey: secret },
          data: { amount: 1, description: "Revoked" },
        },
      )
    ).status(),
  ).toBe(401);
  await createQR(next);
  const key = (await json(`/projects/${project.id}/api-keys`)).find(
    (k: { revokedAt: number | null }) => !k.revokedAt,
  );
  await page
    .getByRole("button", { name: "Revoke", exact: true })
    .last()
    .click();
  await page.getByRole("button", { name: "Confirm", exact: true }).click();
  expect(
    (await json(`/projects/${project.id}/api-keys`)).find(
      (k: { id: string }) => k.id === key.id,
    ).revokedAt,
  ).toBeTruthy();
});
test("webhook signing configuration, test delivery and history are functional", async () => {
  const project = await createProject("Hooks");
  await page.goto(origin + "/portal/webhook-security");
  await page.getByRole("button", { name: "Generate or rotate key" }).click();
  await page.getByRole("button", { name: "Saved securely" }).click();
  await page.goto(origin + "/portal/webhooks");
  await page.getByRole("button", { name: "Add endpoint" }).click();
  await page.getByLabel("Callback URL").fill(origin + "/healthz");
  await page.getByRole("button", { name: "Save endpoint" }).click();
  await page.getByRole("dialog").waitFor({ state: "hidden" });
  await page.getByRole("button", { name: "Send test" }).click();
  let jobs: { id: string; attempts: number }[] = [];
  for (let i = 0; i < 20; i++) {
    jobs = await json(`/projects/${project.id}/webhook-jobs`);
    if (jobs[0]?.attempts) break;
    await Bun.sleep(500);
  }
  expect(jobs[0]?.attempts).toBeGreaterThan(0);
  await page.reload();
  await page
    .getByRole("button", { name: "Inspect", exact: true })
    .first()
    .click();
  await page.getByText("Attempt 1 · HTTP 404", { exact: true }).waitFor();
});
test("manual QR scan requires explicit confirmation and checkout recovers completion", async () => {
  const project = await createProject("Payment");
  const qr = await createQR(await keyFor(project.id));
  const token = new URL(qr.link).pathname.split("/").at(-1)!;
  const transaction = await json(`/simulator/payments/${token}`);
  const checkout = await context.newPage();
  await checkout.goto(transaction.checkoutURL);
  await checkout
    .getByRole("img", { name: "Sandbox payment QR code" })
    .waitFor();
  await page.goto(origin + "/sandbox/scan");
  await page.getByLabel("Payment URL, token, or reference").fill(qr.qrCode);
  await page.getByRole("button", { name: "Find sandbox payment" }).click();
  await page.waitForURL(qr.link);
  expect((await json(`/simulator/payments/${token}`)).status).toBe("WAITING");
  await page.getByRole("button", { name: "Confirm simulated payment" }).click();
  await page
    .getByRole("heading", { name: "Simulated payment received" })
    .waitFor();
  await checkout
    .getByRole("heading", { name: "Simulated payment received" })
    .waitFor();
  await checkout.reload();
  await checkout
    .getByRole("heading", { name: "Simulated payment received" })
    .waitFor();
  await page.goto(origin + `/portal/transactions/${qr.transactionId}`);
  await page.getByRole("heading", { name: "Transaction details" }).waitFor();
  expect(
    await page.getByText("PAYMENT COMPLETED", { exact: true }).count(),
  ).toBeGreaterThan(0);
  await checkout.close();
});
test("uploaded QR image resolves and camera denial has a graceful fallback", async () => {
  const project = await createProject("Scanner");
  const qr = await createQR(await keyFor(project.id));
  const image = await QRCode.toBuffer(qr.qrCode, { width: 512, margin: 4 });
  await page.goto(origin + "/sandbox/scan");
  await page.getByRole("button", { name: "Start camera" }).click();
  await page.getByText("Camera could not start.", { exact: false }).waitFor();
  await page.locator("input[type=file]").setInputFiles({
    name: "sandbox-qr.png",
    mimeType: "image/png",
    buffer: image,
  });
  await page.waitForURL(qr.link);
  expect(
    (
      await json(
        `/simulator/payments/${new URL(qr.link).pathname.split("/").at(-1)}`,
      )
    ).status,
  ).toBe("WAITING");
});
