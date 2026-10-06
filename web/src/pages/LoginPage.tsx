import { FormEvent, useState } from "react";
import { Boxes } from "lucide-react";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { setStoredAdminToken } from "@/lib/auth";
import { CellpApiError, validateAdminToken } from "@/lib/cellp-api";

interface LoginPageProps {
  onAuthenticated: () => void;
}

export function LoginPage({ onAuthenticated }: LoginPageProps) {
  const [token, setToken] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  async function handleSubmit(event: FormEvent) {
    event.preventDefault();
    const trimmed = token.trim();
    if (!trimmed) {
      setError("Admin token is required.");
      return;
    }

    setSubmitting(true);
    setError(null);
    try {
      await validateAdminToken(trimmed);
      setStoredAdminToken(trimmed);
      onAuthenticated();
    } catch (err) {
      if (err instanceof CellpApiError && err.status === 401) {
        setError("Invalid admin token.");
      } else if (err instanceof CellpApiError) {
        setError(err.message || "Could not validate token.");
      } else {
        setError(
          "Cannot reach cellpd API. Check that the platform is running.",
        );
      }
    } finally {
      setSubmitting(false);
    }
  }

  return (
    <div className="flex min-h-screen items-center justify-center bg-background px-4">
      <Card className="w-full max-w-md">
        <CardHeader>
          <div className="mb-2 flex items-center gap-2">
            <div className="flex size-8 items-center justify-center rounded-md border border-border bg-card">
              <Boxes className="size-4" />
            </div>
            <span className="text-sm font-semibold tracking-tight">cellp</span>
          </div>
          <CardTitle>Sign in</CardTitle>
          <CardDescription>
            Enter the platform admin token. It is stored in this browser only
            (localStorage) and sent as a Bearer token on API requests.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form className="space-y-4" onSubmit={handleSubmit}>
            <div className="space-y-2">
              <label htmlFor="admin-token" className="text-sm font-medium">
                Admin token
              </label>
              <input
                id="admin-token"
                type="password"
                autoComplete="off"
                data-testid="admin-token-input"
                className="flex h-10 w-full rounded-md border border-border bg-background px-3 py-2 text-sm ring-offset-background placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                placeholder="Bearer token value"
                value={token}
                onChange={(event) => setToken(event.target.value)}
                disabled={submitting}
              />
            </div>
            {error ? (
              <p
                className="text-sm text-destructive"
                data-testid="login-error"
                role="alert"
              >
                {error}
              </p>
            ) : null}
            <Button
              type="submit"
              className="w-full"
              disabled={submitting}
              data-testid="login-submit"
            >
              {submitting ? "Signing in…" : "Sign in"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  );
}
