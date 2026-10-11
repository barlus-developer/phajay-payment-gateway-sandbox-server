import { describe, expect, test, afterEach } from "bun:test";
import {
  api,
  APIError,
  money,
  paymentToken,
  remaining,
  setCSRF,
  validateImage,
} from "./api";
const original = globalThis.fetch;
afterEach(() => {
  globalThis.fetch = original;
  setCSRF("");
});
describe("portal requests", () => {
  test("uses session cookies and CSRF on mutations", async () => {
    let captured: RequestInit | undefined;
    globalThis.fetch = (async (_input: unknown, init?: RequestInit) => {
      captured = init;
      return Response.json({ data: { secret: "shown-once" } });
    }) as unknown as typeof fetch;
    setCSRF("csrf-test");
    expect(
      await api<{ secret: string }>("/projects/p/api-keys", "POST", {
        name: "SDK",
      }),
    ).toEqual({
      secret: "shown-once",
    });
    expect(captured?.credentials).toBe("same-origin");
    expect((captured?.headers as Record<string, string>)["X-CSRF-Token"]).toBe(
      "csrf-test",
    );
  });
  test("surfaces authentication errors", async () => {
    globalThis.fetch = (async () =>
      Response.json(
        { error: { message: "Login required" } },
        { status: 401 },
      )) as unknown as typeof fetch;
    await expect(api("/auth/me")).rejects.toBeInstanceOf(APIError);
  });
});
describe("scanner and payments", () => {
  test("uses exact stored minor amounts", () => {
    expect(money(1234)).toBe("12.34 LAK");
    expect(remaining(100, 101)).toBe(0);
  });
  test("extracts only sandbox pay tokens", () => {
    expect(paymentToken("https://sandbox.test/sandbox/pay/pay_secret")).toBe(
      "pay_secret",
    );
    expect(() => paymentToken("https://sandbox.test/production")).toThrow();
  });
  test("rejects unsupported images and oversized uploads", () => {
    expect(() => validateImage({ size: 10, type: "image/svg+xml" })).toThrow();
    expect(() =>
      validateImage({ size: 9 * 1024 * 1024, type: "image/png" }),
    ).toThrow();
    expect(() => validateImage({ size: 100, type: "image/png" })).not.toThrow();
  });
});
