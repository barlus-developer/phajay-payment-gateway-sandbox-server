let csrfToken = "";
export const setCSRF = (value: string) => {
  csrfToken = value;
};
export class APIError extends Error {
  constructor(
    message: string,
    public status: number,
  ) {
    super(message);
  }
}
export async function api<T>(
  path: string,
  method = "GET",
  body?: unknown,
): Promise<T> {
  const response = await fetch(`/api/internal/v1${path}`, {
    method,
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      "X-Phajay-Portal": "1",
      ...(csrfToken ? { "X-CSRF-Token": csrfToken } : {}),
    },
    ...(body !== undefined ? { body: JSON.stringify(body) } : {}),
  });
  const result = (await response.json()) as {
    data?: T;
    error?: { message: string };
  };
  if (!response.ok)
    throw new APIError(
      result.error?.message || "Request failed",
      response.status,
    );
  return result.data as T;
}
export function money(minor: number, currency = "LAK") {
  return `${new Intl.NumberFormat("en", { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(minor / 100)} ${currency}`;
}
export function date(value?: number | null) {
  return value ? new Date(value * 1000).toLocaleString() : "—";
}
export function paymentToken(url: string) {
  const parsed = new URL(url);
  if (!parsed.pathname.startsWith("/sandbox/pay/"))
    throw new Error("Invalid sandbox payment URL");
  return parsed.pathname.slice("/sandbox/pay/".length);
}
export function validateImage(file: Pick<File, "size" | "type">) {
  if (file.size > 8 * 1024 * 1024)
    throw new Error("Image must be smaller than 8 MB");
  if (!["image/png", "image/jpeg", "image/webp"].includes(file.type))
    throw new Error("Upload a PNG, JPEG or WebP image");
}
export function remaining(expiresAt: number, now: number) {
  return Math.max(0, expiresAt - now);
}
