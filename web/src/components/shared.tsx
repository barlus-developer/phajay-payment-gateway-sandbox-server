import { useEffect, useId, useState, type ReactNode } from "react";
import QRCode from "qrcode";
import { Copy, LoaderCircle } from "lucide-react";
import { toast } from "sonner";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Field, FieldLabel, FieldDescription } from "@/components/ui/field";
import { Badge } from "@/components/ui/badge";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";

export function TextField({
  label,
  help,
  ...props
}: React.ComponentProps<typeof Input> & { label: string; help?: string }) {
  const id = useId();
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <Input id={id} {...props} />
      {help && <FieldDescription>{help}</FieldDescription>}
    </Field>
  );
}
export function Choice({
  label,
  value,
  options,
  onChange,
}: {
  label: string;
  value: string;
  options: { value: string; label: string }[];
  onChange: (value: string) => void;
}) {
  const id = useId();
  return (
    <Field>
      <FieldLabel id={id}>{label}</FieldLabel>
      <Select
        value={value}
        onValueChange={(v) => {
          if (v !== null) onChange(v);
        }}
      >
        <SelectTrigger aria-labelledby={id} className="w-full">
          <SelectValue>
            {options.find((o) => o.value === value)?.label || "Select"}
          </SelectValue>
        </SelectTrigger>
        <SelectContent>
          {options.map((o) => (
            <SelectItem key={o.value} value={o.value}>
              {o.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </Field>
  );
}
export function Status({ value }: { value: string }) {
  return (
    <Badge
      variant="outline"
      className={
        value === "PAYMENT_COMPLETED" || value === "DELIVERED"
          ? "border-emerald-200 bg-emerald-50 text-emerald-700"
          : value === "WAITING" || value === "PENDING"
            ? "border-amber-200 bg-amber-50 text-amber-700"
            : "bg-muted text-muted-foreground"
      }
    >
      {value.replaceAll("_", " ")}
    </Badge>
  );
}
export function Loading() {
  return (
    <div role="status" aria-label="Loading" className="grid gap-4">
      <Skeleton className="h-10 w-48" />
      <Skeleton className="h-52 w-full" />
    </div>
  );
}
export function Empty({
  title,
  children,
}: {
  title: string;
  children?: ReactNode;
}) {
  return (
    <div className="rounded-xl border border-dashed p-10 text-center">
      <h2 className="font-medium">{title}</h2>
      <div className="mt-2 text-sm text-muted-foreground">{children}</div>
    </div>
  );
}
export function ErrorState({
  error,
  retry,
}: {
  error: Error | null;
  retry?: () => void;
}) {
  return (
    <div
      role="alert"
      className="rounded-lg border border-destructive/20 bg-destructive/5 p-4 text-sm text-destructive"
    >
      {error?.message || "Something went wrong"}
      {retry && (
        <Button variant="outline" className="ml-3" onClick={retry}>
          Retry
        </Button>
      )}
    </div>
  );
}
export function Busy({
  children,
  pending,
  ...props
}: React.ComponentProps<typeof Button> & { pending: boolean }) {
  return (
    <Button {...props} disabled={pending || props.disabled}>
      {pending && <LoaderCircle className="animate-spin" />}
      {children}
    </Button>
  );
}
export function Title({
  title,
  description,
  action,
}: {
  title: string;
  description: string;
  action?: ReactNode;
}) {
  return (
    <div className="mb-7 flex flex-wrap items-start justify-between gap-4">
      <div>
        <h1 className="page-heading">{title}</h1>
        <p className="page-description">{description}</p>
      </div>
      {action}
    </div>
  );
}
export function Secret({ value, close }: { value: string; close: () => void }) {
  return (
    <Dialog
      open={Boolean(value)}
      onOpenChange={(open) => {
        if (!open) close();
      }}
    >
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Save your sandbox secret</DialogTitle>
          <DialogDescription>
            This secret is displayed once. Store it securely before closing.
          </DialogDescription>
        </DialogHeader>
        <Input
          aria-label="New secret"
          readOnly
          value={value}
          className="font-mono text-xs"
        />
        <DialogFooter>
          <Button
            variant="outline"
            onClick={() => {
              navigator.clipboard
                .writeText(value)
                .then(() => toast.success("Copied"))
                .catch(() =>
                  toast.error("Select and copy the secret manually"),
                );
            }}
          >
            <Copy />
            Copy secret
          </Button>
          <Button onClick={close}>Saved securely</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
export function Confirm({
  title,
  description,
  action,
  children,
}: {
  title: string;
  description: string;
  action: () => Promise<unknown>;
  children: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const [pending, setPending] = useState(false);
  return (
    <>
      <Button variant="outline" size="sm" onClick={() => setOpen(true)}>
        {children}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{title}</DialogTitle>
            <DialogDescription>{description}</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              Keep
            </Button>
            <Busy
              pending={pending}
              variant="destructive"
              onClick={async () => {
                setPending(true);
                try {
                  await action();
                  setOpen(false);
                } catch (e) {
                  toast.error((e as Error).message);
                } finally {
                  setPending(false);
                }
              }}
            >
              Confirm
            </Busy>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
export function QR({ value }: { value: string }) {
  const [image, setImage] = useState("");
  const [error, setError] = useState("");
  useEffect(() => {
    let active = true;
    QRCode.toDataURL(value, {
      width: 320,
      margin: 2,
      errorCorrectionLevel: "M",
    })
      .then((v) => {
        if (active) setImage(v);
      })
      .catch(() => {
        if (active) setError("QR could not be generated");
      });
    return () => {
      active = false;
    };
  }, [value]);
  if (error) return <p role="alert">{error}</p>;
  return image ? (
    <img
      src={image}
      alt="Sandbox payment QR code"
      width={320}
      height={320}
      className="mx-auto w-full max-w-80 rounded-lg"
    />
  ) : (
    <Skeleton className="mx-auto size-64" />
  );
}
export function Pagination({
  offset,
  total,
  limit = 25,
  setOffset,
}: {
  offset: number;
  total: number;
  limit?: number;
  setOffset: (value: number) => void;
}) {
  return (
    <nav
      aria-label="Pagination"
      className="mt-4 flex items-center justify-between gap-3 text-sm text-muted-foreground"
    >
      <span>
        {total === 0
          ? "0 results"
          : `${offset + 1}–${Math.min(total, offset + limit)} of ${total}`}
      </span>
      <div className="flex gap-2">
        <Button
          variant="outline"
          size="sm"
          disabled={offset === 0}
          onClick={() => setOffset(Math.max(0, offset - limit))}
        >
          Previous
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={offset + limit >= total}
          onClick={() => setOffset(offset + limit)}
        >
          Next
        </Button>
      </div>
    </nav>
  );
}
