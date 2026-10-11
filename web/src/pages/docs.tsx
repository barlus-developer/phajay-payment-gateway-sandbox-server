import { usePortal } from "@/hooks/portal";
import { Title } from "@/components/shared";

const setup = `go get github.com/barlus-developer/phajay-payment-gateway

// Until released upstream, use the managed SDK fork:
// replace github.com/barlus-developer/phajay-payment-gateway => /path/to/platform/sdk-fork

client := phajay.New(phajay.Config{
    APIKey: os.Getenv("PHAJAY_SANDBOX_API_KEY"),
    BaseURL: "http://localhost:8080",
    WebhookSignatureKey: os.Getenv("PHAJAY_SANDBOX_SIGNING_KEY"),
    BypassSignatureVerification: false,
}, phajay.WithSandbox())`;
const payment = `link, err := client.CreatePaymentLink(ctx, phajay.PaymentLinkRequest{
    OrderNo: "order-001", Amount: 12500.50, Description: "Test order",
})
// Open link.RedirectURL for the live sandbox checkout.

qr, err := client.GenerateQR(ctx, phajay.BankJDB, phajay.PaymentQRRequest{
    Amount: 12500.50, Description: "Test QR payment",
})
// Encode qr.QRCode as a QR image; qr.Link opens the simulator.
// Also supported: BankLDB, BankIB, BankBCEL, BankSTB.`;
const callback = `func webhook(w http.ResponseWriter, r *http.Request) {
    raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
    if err != nil { http.Error(w, "body too large", 413); return }
    var callback phajay.PaymentQRWebhookCallback
    if err := client.DecodeWebhook(raw, r.Header, &callback); err != nil {
        http.Error(w, "invalid signature", 401); return
    }
    eventID := r.Header.Get("X-Phajay-Sandbox-Event-ID")
    // Atomically record eventID AND apply the business operation.
    // Deduplicate by transactionId as well; acknowledge duplicates.
    _ = eventID
    w.WriteHeader(http.StatusNoContent)
}
// Link payments use phajay.SandboxPaymentLinkWebhookCallback.
// Preserve exact raw bytes before JSON parsing.
// During rotation, try the previous client only until validUntil.`;
const development = `# Backend (load .env into your environment first)
go run ./cmd/server

# Frontend (web/)
bun install
bun run dev
# Backend PUBLIC_URL must match the Vite origin: http://localhost:5173

# Build static assets, then serve everything from Go:
bun run build
# Set PUBLIC_URL back to http://localhost:8080

# Docker after .env setup:
docker compose up --build -d`;
export function Docs() {
  const { project } = usePortal();
  return (
    <>
      <Title
        title="Integration guide"
        description="SDK contracts, signed callbacks, and local sandbox workflows."
      />
      <div className="grid gap-6">
        <section className="panel">
          <h2 className="font-semibold">1. Configure the Go SDK</h2>
          <p className="mt-2 text-sm text-muted-foreground">
            Create a project and generate an API key. Keep secrets outside
            source control. This workspace uses an explicit managed SDK fork for
            the custom sandbox verifier. Upstream verification still fails
            closed until that patch is released.
          </p>
          <pre className="code mt-4">{setup}</pre>
          {project && (
            <p className="mt-3 text-xs text-muted-foreground">
              Selected project: {project.name} · {project.id}
            </p>
          )}
        </section>
        <section className="panel">
          <h2 className="font-semibold">2. Create a payment</h2>
          <p className="mt-2 text-sm text-muted-foreground">
            Only six documented test routes are exposed. Link authentication
            uses Basic plus base64 of the API key alone; QR authentication uses
            secretKey. Positive LAK decimals support two fractional digits and
            are stored as integer minor units.
          </p>
          <pre className="code mt-4">{payment}</pre>
        </section>
        <section className="panel">
          <h2 className="font-semibold">3. Scan and confirm</h2>
          <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
            Open /sandbox/scan on a mobile device. HTTPS enables camera access;
            image upload and manual entry are also available. Review the
            merchant and amount, then explicitly confirm payment. Checkout uses
            SSE and reloads the current state after reconnecting. Failure and
            cancellation are simulator actions.
          </p>
        </section>
        <section className="panel">
          <h2 className="font-semibold">4. Receive verified webhooks</h2>
          <p className="mt-2 text-sm leading-relaxed text-muted-foreground">
            Generate a signing key, register an enabled callback URL and
            subscriptions, then send a test. HMAC-SHA256 signs timestamp +
            period + exact raw body. Required headers:
            X-Phajay-Sandbox-Timestamp, X-Phajay-Sandbox-Signature,
            X-Phajay-Sandbox-Event-ID. Signatures are lowercase hexadecimal with
            v1=. Timestamp tolerance and key rotation overlap are five minutes.
          </p>
          <pre className="code mt-4">{callback}</pre>
          <p className="mt-3 text-sm text-muted-foreground">
            Delivery is at least once. Preserve event IDs and process duplicate
            payments idempotently. This custom sandbox protocol does not
            reproduce Phajay's private production signing algorithm.
          </p>
        </section>
        <section className="panel">
          <h2 className="font-semibold">5. Development and hosting</h2>
          <pre className="code mt-4">{development}</pre>
          <p className="mt-3 text-sm text-muted-foreground">
            Generate MASTER_KEY with openssl rand -base64 32 and keep it stable
            across upgrades. Local callbacks require APP_ENV=development and
            ALLOW_LOCAL_WEBHOOKS=true. Use one Go server and a persistent SQLite
            volume. Repository deployment documentation covers HTTPS, safe WAL
            backups, and restoration.
          </p>
          <a
            className="mt-4 inline-block text-sm text-primary"
            href="/api/internal/v1/openapi"
            target="_blank"
            rel="noreferrer"
          >
            OpenAPI specification ↗
          </a>
        </section>
      </div>
    </>
  );
}
