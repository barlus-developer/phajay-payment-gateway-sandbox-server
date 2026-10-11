import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type ReactNode,
} from "react";
import {
  Link,
  useNavigate,
  useParams,
  useSearchParams,
} from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import QrScanner from "qr-scanner";
import {
  Camera,
  CheckCircle2,
  QrCode,
  RotateCw,
  ShieldCheck,
  Upload,
} from "lucide-react";
import {
  api,
  date,
  money,
  paymentToken,
  remaining,
  validateImage,
} from "@/lib/api";
import type { PaymentState, Transaction } from "@/lib/types";
import { Button } from "@/components/ui/button";
import {
  Busy,
  ErrorState,
  Loading,
  QR,
  Status,
  TextField,
} from "@/components/shared";

function Frame({ children }: { children: ReactNode }) {
  return (
    <main className="min-h-screen bg-muted px-4 py-6">
      <div className="mx-auto max-w-md">
        <header className="mb-6 flex items-center justify-between">
          <Link
            to="/sandbox/scan"
            className="flex items-center gap-2 font-semibold"
          >
            <QrCode className="size-5 text-primary" />
            Phajay Sandbox
          </Link>
          <span className="rounded-full bg-accent px-3 py-1 text-xs font-medium text-primary">
            SIMULATOR
          </span>
        </header>
        {children}
        <footer className="mt-7 text-center text-xs leading-relaxed text-muted-foreground">
          SANDBOX PAYMENT — NOT A REAL TRANSACTION
          <br />
          No banking connections. No real money transfers.
          <div className="mt-3">
            <Link to="/portal" className="text-primary">
              Developer portal
            </Link>
          </div>
        </footer>
      </div>
    </main>
  );
}
function useCountdown(expiresAt?: number) {
  const [now, setNow] = useState(Math.floor(Date.now() / 1000));
  useEffect(() => {
    const tick = setInterval(() => setNow(Math.floor(Date.now() / 1000)), 1000);
    return () => clearInterval(tick);
  }, []);
  const seconds = remaining(expiresAt || 0, now);
  return {
    seconds,
    label: `${Math.floor(seconds / 60)
      .toString()
      .padStart(2, "0")}:${(seconds % 60).toString().padStart(2, "0")}`,
  };
}
export function Scanner() {
  const navigate = useNavigate(),
    video = useRef<HTMLVideoElement>(null),
    scanner = useRef<QrScanner | null>(null),
    processing = useRef(false);
  const [camera, setCamera] = useState(false),
    [rear, setRear] = useState(true),
    [cameraError, setCameraError] = useState(""),
    [reference, setReference] = useState(""),
    [project, setProject] = useState(
      () => localStorage.getItem("phajay-project") || "",
    ),
    [imageError, setImageError] = useState("");
  const resolve = useMutation({
    mutationFn: (value: string) =>
      api<Transaction>("/simulator/resolve", "POST", {
        reference: value,
        projectId: project,
      }),
    onSuccess: (t: Transaction) => {
      scanner.current?.stop();
      navigate(`/sandbox/pay/${paymentToken(t.paymentURL)}`);
    },
    onSettled: () => {
      processing.current = false;
    },
  });
  const resolveRef = useRef(resolve.mutate);
  resolveRef.current = resolve.mutate;
  const scanned = useCallback((value: string) => {
    if (processing.current) return;
    processing.current = true;
    resolveRef.current(value);
  }, []);
  useEffect(() => {
    if (!camera || !video.current) return;
    if (!window.isSecureContext) {
      setCameraError(
        "Camera access requires HTTPS on mobile, or localhost on this device.",
      );
      setCamera(false);
      return;
    }
    let active = true;
    const reader = new QrScanner(
      video.current,
      (result: { data: string }) => scanned(result.data),
      {
        preferredCamera: rear ? "environment" : "user",
        returnDetailedScanResult: true,
        highlightScanRegion: true,
        highlightCodeOutline: true,
        maxScansPerSecond: 5,
      },
    );
    scanner.current = reader;
    reader.start().catch(() => {
      if (active) {
        setCameraError(
          "Camera could not start. Allow camera permission or upload an image.",
        );
        setCamera(false);
      }
    });
    return () => {
      active = false;
      reader.destroy();
      scanner.current = null;
    };
  }, [camera, rear, scanned]);
  return (
    <Frame>
      <section className="panel">
        <h1 className="text-xl font-semibold">Scan a sandbox QR</h1>
        <p className="mt-2 text-sm text-muted-foreground">
          Identify the payment, then review and confirm it on the next screen.
        </p>
        <div className="relative mt-5 aspect-square overflow-hidden rounded-xl bg-slate-950">
          <video
            ref={video}
            muted
            playsInline
            className={`size-full object-cover ${camera ? "" : "hidden"}`}
          />
          {!camera && (
            <div className="absolute inset-0 flex flex-col items-center justify-center gap-4 text-white">
              <QrCode className="size-20 opacity-70" />
              <p className="text-sm text-white/60">Sandbox QR scanner</p>
              <Button
                onClick={() => {
                  setCameraError("");
                  setCamera(true);
                }}
              >
                <Camera />
                Start camera
              </Button>
            </div>
          )}
        </div>
        {camera && (
          <div className="mt-3 flex justify-between gap-3">
            <Button variant="outline" onClick={() => setRear(!rear)}>
              <RotateCw />
              Switch camera
            </Button>
            <Button variant="outline" onClick={() => setCamera(false)}>
              Stop camera
            </Button>
          </div>
        )}
        {cameraError && (
          <p role="alert" className="mt-3 text-sm text-destructive">
            {cameraError}
          </p>
        )}
        <label className="mt-5 flex cursor-pointer items-center justify-center gap-2 rounded-lg border p-3 text-sm font-medium">
          <Upload className="size-4" />
          Upload QR image
          <input
            className="sr-only"
            type="file"
            accept="image/png,image/jpeg,image/webp"
            onChange={async (e) => {
              const file = e.target.files?.[0];
              if (!file) return;
              setImageError("");
              try {
                validateImage(file);
                const result = await QrScanner.scanImage(file, {
                  returnDetailedScanResult: true,
                });
                scanned(result.data);
              } catch (error) {
                setImageError(
                  error instanceof Error
                    ? error.message
                    : "No readable QR code in this image",
                );
              } finally {
                e.target.value = "";
              }
            }}
          />
        </label>
        {imageError && (
          <p role="alert" className="mt-2 text-sm text-destructive">
            {imageError}
          </p>
        )}
        <form
          className="mt-6 flex flex-col gap-4 border-t pt-5"
          onSubmit={(e) => {
            e.preventDefault();
            scanned(reference);
          }}
        >
          <TextField
            label="Payment URL, token, or reference"
            required
            maxLength={2048}
            value={reference}
            onChange={(e) => setReference(e.target.value)}
            placeholder="Paste a sandbox payment URL"
            help="Transaction IDs require login and ownership of the selected project."
          />
          <TextField
            label="Project ID (for transaction references)"
            value={project}
            onChange={(e) => setProject(e.target.value)}
            maxLength={100}
          />
          <Busy type="submit" pending={resolve.isPending}>
            Find sandbox payment
          </Busy>
          {resolve.error && <ErrorState error={resolve.error} />}
        </form>
      </section>
    </Frame>
  );
}
function Details({ t }: { t: Transaction }) {
  return (
    <dl className="mt-6 flex flex-col gap-4 text-sm">
      {[
        ["Merchant", t.merchantName],
        ["Description", t.description],
        ["Transaction", t.id],
        ["Currency", t.currency],
      ].map(([label, value]) => (
        <div className="flex justify-between gap-4" key={label}>
          <dt className="shrink-0 text-muted-foreground">{label}</dt>
          <dd className="max-w-[70%] break-all text-right">{value}</dd>
        </div>
      ))}
    </dl>
  );
}
function Receipt({ t }: { t: Transaction }) {
  return (
    <section className="panel text-center">
      <div className="mx-auto mb-4 flex size-14 items-center justify-center rounded-full bg-emerald-50 text-emerald-600">
        <CheckCircle2 className="size-8" />
      </div>
      <h1 className="text-xl font-semibold">Simulated payment received</h1>
      <p className="mt-3 text-3xl font-semibold tabular-nums">
        {money(t.amountMinor, t.currency)}
      </p>
      <Details t={t} />
      <dl className="mt-4 flex flex-col gap-4 border-t pt-4 text-sm">
        {[
          ["Simulated bank", t.bank],
          ["Completed", date(t.completedAt)],
          ["Receipt reference", t.receiptRef],
        ].map(([label, value]) => (
          <div className="flex justify-between gap-4" key={label}>
            <dt className="text-muted-foreground">{label}</dt>
            <dd className="break-all text-right">{value}</dd>
          </div>
        ))}
      </dl>
      <p className="mt-6 rounded-lg bg-accent p-3 text-xs font-semibold text-primary">
        SANDBOX PAYMENT — NOT A REAL TRANSACTION
      </p>
      <Link
        to="/sandbox/scan"
        className="mt-5 inline-block text-sm text-primary"
      >
        Scan another sandbox payment →
      </Link>
    </section>
  );
}
export function Pay() {
  const { token } = useParams(),
    client = useQueryClient();
  const q = useQuery<Transaction, Error>({
    queryKey: ["payment", token],
    queryFn: () => api(`/simulator/payments/${token}`),
    enabled: Boolean(token),
    refetchInterval: 5000,
  });
  const countdown = useCountdown(q.data?.expiresAt);
  const confirm = useMutation({
    mutationFn: (status: PaymentState) =>
      api<Transaction>(`/simulator/payments/${token}/confirm`, "POST", {
        status,
      }),
    onSuccess: (t: Transaction) => {
      client.setQueryData(["payment", token], t);
      client.invalidateQueries({ queryKey: ["checkout"] });
    },
    onError: () => q.refetch(),
  });
  useEffect(() => {
    if (q.data?.status === "WAITING" && countdown.seconds === 0) q.refetch();
  }, [q.data?.status, countdown.seconds, q.refetch]);
  if (q.error)
    return (
      <Frame>
        <ErrorState error={q.error} retry={() => q.refetch()} />
      </Frame>
    );
  if (!q.data)
    return (
      <Frame>
        <Loading />
      </Frame>
    );
  const t = q.data;
  if (t.status === "PAYMENT_COMPLETED")
    return (
      <Frame>
        <Receipt t={t} />
      </Frame>
    );
  const waiting = t.status === "WAITING" && countdown.seconds > 0;
  return (
    <Frame>
      <section className="panel">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-3">
            <div className="flex size-11 items-center justify-center rounded-lg bg-primary text-sm font-semibold text-primary-foreground">
              {t.bank}
            </div>
            <div>
              <h1 className="font-semibold">{t.bank} simulator</h1>
              <p className="text-xs text-muted-foreground">
                Simulated mobile banking
              </p>
            </div>
          </div>
          <ShieldCheck className="size-5 text-primary" />
        </div>
        <div className="mt-7 text-center">
          <p className="text-sm text-muted-foreground">Payment amount</p>
          <p className="mt-2 text-3xl font-semibold tabular-nums">
            {money(t.amountMinor, t.currency)}
          </p>
          <div className="mt-4">
            <Status value={t.status} />
          </div>
        </div>
        <Details t={t} />
        <p className="mt-5 text-center text-sm text-muted-foreground">
          {waiting
            ? `Expires in ${countdown.label}`
            : countdown.seconds === 0 && t.status === "WAITING"
              ? "Payment is expiring…"
              : "This payment is closed."}
        </p>
        {waiting && (
          <div className="mt-6 flex flex-col gap-3">
            <Busy
              pending={confirm.isPending}
              onClick={() => confirm.mutate("PAYMENT_COMPLETED")}
            >
              Confirm simulated payment
            </Busy>
            <div className="grid grid-cols-2 gap-3">
              <Button
                variant="outline"
                disabled={confirm.isPending}
                onClick={() => confirm.mutate("FAILED")}
              >
                Simulate failure
              </Button>
              <Button
                variant="outline"
                disabled={confirm.isPending}
                onClick={() => confirm.mutate("CANCELED")}
              >
                Cancel payment
              </Button>
            </div>
          </div>
        )}
        {confirm.error && (
          <div className="mt-4">
            <ErrorState error={confirm.error} />
          </div>
        )}
        <p className="mt-6 rounded-lg bg-muted p-3 text-center text-xs text-muted-foreground">
          Confirming changes only this sandbox transaction.
        </p>
      </section>
    </Frame>
  );
}
export function Checkout() {
  const { transactionId } = useParams(),
    [params] = useSearchParams(),
    token = params.get("token") || "",
    [connected, setConnected] = useState(false);
  const q = useQuery<Transaction, Error>({
    queryKey: ["checkout", transactionId, token],
    queryFn: () =>
      api(
        `/simulator/checkout/${transactionId}?token=${encodeURIComponent(token)}`,
      ),
    enabled: Boolean(transactionId && token),
    refetchInterval: 5000,
  });
  const countdown = useCountdown(q.data?.expiresAt);
  useEffect(() => {
    if (!transactionId || !token) return;
    const events = new EventSource(
      `/api/internal/v1/simulator/checkout/${transactionId}/events?token=${encodeURIComponent(token)}`,
    );
    events.onopen = () => {
      setConnected(true);
      q.refetch();
    };
    events.addEventListener("payment", () => q.refetch());
    events.onerror = () => setConnected(false);
    return () => events.close();
  }, [transactionId, token, q.refetch]);
  if (!token)
    return (
      <Frame>
        <ErrorState
          error={
            new Error(
              "This checkout requires the complete authorized payment link.",
            )
          }
        />
      </Frame>
    );
  if (q.error)
    return (
      <Frame>
        <ErrorState error={q.error} retry={() => q.refetch()} />
      </Frame>
    );
  if (!q.data)
    return (
      <Frame>
        <Loading />
      </Frame>
    );
  const t = q.data;
  if (t.status === "PAYMENT_COMPLETED")
    return (
      <Frame>
        <Receipt t={t} />
      </Frame>
    );
  return (
    <Frame>
      <section className="panel text-center">
        <p className="text-sm text-muted-foreground">Pay {t.merchantName}</p>
        <h1 className="mt-2 text-3xl font-semibold tabular-nums">
          {money(t.amountMinor, t.currency)}
        </h1>
        <p className="mt-2 text-sm text-muted-foreground">{t.description}</p>
        <div className="mt-5">
          <Status value={t.status} />
        </div>
        {t.status === "WAITING" && countdown.seconds > 0 && (
          <>
            <QR value={t.paymentURL} />
            <p className="text-sm text-muted-foreground">
              Scan this QR in the sandbox scanner.
            </p>
            <p className="mt-3 text-sm tabular-nums">
              Expires in {countdown.label}
            </p>
            <Link
              to={`/sandbox/pay/${paymentToken(t.paymentURL)}`}
              className="mt-5 inline-block rounded-lg bg-primary px-5 py-3 text-sm font-medium text-primary-foreground"
            >
              Open payment simulator
            </Link>
          </>
        )}
        <p className="mt-5 text-xs text-muted-foreground">
          {connected
            ? "Live checkout connected"
            : "Reconnecting · latest status checked automatically"}
        </p>
        <p className="mt-4 break-all font-mono text-[10px] text-muted-foreground">
          {t.id}
        </p>
      </section>
    </Frame>
  );
}
