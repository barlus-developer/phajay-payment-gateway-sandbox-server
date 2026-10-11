import { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { QrCode, ShieldCheck } from "lucide-react";
import { api, setCSRF } from "@/lib/api";
import type { Account } from "@/lib/types";
import { Busy, ErrorState, TextField } from "@/components/shared";
import { FieldGroup } from "@/components/ui/field";

export function AuthPage({ register = false }: { register?: boolean }) {
  const [email, setEmail] = useState(""),
    [name, setName] = useState(""),
    [password, setPassword] = useState("");
  const client = useQueryClient(),
    navigate = useNavigate();
  const auth = useMutation({
    mutationFn: async () => {
      if (register)
        await api("/auth/register", "POST", { email, name, password });
      return api<Account>("/auth/login", "POST", { email, password });
    },
    onSuccess: (data: Account) => {
      setCSRF(data.csrfToken);
      client.setQueryData(["account"], data);
      navigate("/portal");
    },
  });
  return (
    <main className="grid min-h-screen bg-muted/40 lg:grid-cols-2">
      <section className="hidden flex-col justify-between bg-primary p-14 text-primary-foreground lg:flex">
        <div className="flex items-center gap-3 text-xl font-semibold">
          <QrCode />
          Phajay Sandbox
        </div>
        <div>
          <p className="mb-6 text-sm uppercase tracking-[.18em] opacity-70">
            Build with confidence
          </p>
          <h1 className="max-w-lg text-5xl font-semibold leading-tight tracking-tight">
            Your payment integration.
            <br />A safe place to test.
          </h1>
          <p className="mt-6 max-w-md text-lg leading-relaxed opacity-80">
            Generate sandbox QR payments, simulate checkout, and verify signed
            webhooks in one developer workspace.
          </p>
        </div>
        <p className="flex items-center gap-2 text-sm opacity-80">
          <ShieldCheck className="size-4" />
          Simulated payments · No real money
        </p>
      </section>
      <section className="flex items-center justify-center p-6">
        <div className="w-full max-w-sm">
          <Link
            to="/portal"
            className="mb-10 flex items-center gap-2 text-lg font-semibold lg:hidden"
          >
            <QrCode className="text-primary" />
            Phajay Sandbox
          </Link>
          <h1 className="page-heading">
            {register ? "Create your account" : "Welcome back"}
          </h1>
          <p className="page-description">
            {register
              ? "Start building in your own sandbox workspace."
              : "Sign in to your developer workspace."}
          </p>
          <form
            className="mt-8 flex flex-col gap-5"
            onSubmit={(e) => {
              e.preventDefault();
              auth.mutate();
            }}
          >
            <FieldGroup>
              {register && (
                <TextField
                  label="Your name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                  autoComplete="name"
                  maxLength={100}
                  required
                />
              )}
              <TextField
                label="Email address"
                type="email"
                autoComplete="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
                required
                maxLength={254}
              />
              <TextField
                label="Password"
                type="password"
                autoComplete={register ? "new-password" : "current-password"}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                required
                minLength={register ? 12 : undefined}
                maxLength={128}
                help={register ? "Use at least 12 characters." : undefined}
              />
            </FieldGroup>
            {auth.error && <ErrorState error={auth.error} />}
            <Busy type="submit" pending={auth.isPending}>
              {register ? "Create account" : "Sign in"}
            </Busy>
          </form>
          <p className="mt-6 text-center text-sm text-muted-foreground">
            {register ? "Already have an account?" : "New to the sandbox?"}{" "}
            <Link
              className="font-medium text-primary"
              to={register ? "/login" : "/register"}
            >
              {register ? "Sign in" : "Create an account"}
            </Link>
          </p>
          <p className="mt-10 text-center text-xs text-muted-foreground">
            SANDBOX — No real transactions
          </p>
        </div>
      </section>
    </main>
  );
}
