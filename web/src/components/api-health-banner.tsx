import { useEffect, useState } from "react";
import { healthCheck } from "@/lib/cellp-api";

function adminTokenConfigured(): boolean {
  const token = import.meta.env.VITE_CELLP_ADMIN_TOKEN;
  return typeof token === "string" && token.trim().length > 0;
}

export function ApiHealthBanner() {
  const [apiMessage, setApiMessage] = useState<string | null>(null);
  const tokenMissing = !adminTokenConfigured();

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        await healthCheck();
        if (!cancelled) setApiMessage(null);
      } catch {
        if (!cancelled) {
          setApiMessage(
            "Cannot reach cellpd API. Run cellp dev or ./dev/scripts/up.sh (API :8790).",
          );
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  if (!apiMessage && !tokenMissing) return null;

  return (
    <div className="space-y-0 border-b border-border">
      {tokenMissing ? (
        <div
          className="border-b border-amber-500/30 bg-amber-500/10 px-4 py-2 text-sm text-amber-950 dark:text-amber-100 md:px-8"
          data-testid="admin-token-hint"
        >
          Set <code className="text-xs">VITE_CELLP_ADMIN_TOKEN</code> in{" "}
          <code className="text-xs">web/.env</code> (local default:{" "}
          <code className="text-xs">dev-local-token</code>, same as cellp dev). The
          Dashboard has no login UI — it sends this Bearer token on every API call.
        </div>
      ) : null}
      {apiMessage ? (
        <div className="border-destructive/40 bg-destructive/10 px-4 py-2 text-sm text-destructive md:px-8">
          {apiMessage}
        </div>
      ) : null}
    </div>
  );
}
