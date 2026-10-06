import { useEffect, useState } from "react";
import { healthCheck } from "@/lib/cellp-api";

export function ApiHealthBanner() {
  const [apiMessage, setApiMessage] = useState<string | null>(null);

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

  if (!apiMessage) return null;

  return (
    <div className="border-b border-border">
      <div className="border-destructive/40 bg-destructive/10 px-4 py-2 text-sm text-destructive md:px-8">
        {apiMessage}
      </div>
    </div>
  );
}
