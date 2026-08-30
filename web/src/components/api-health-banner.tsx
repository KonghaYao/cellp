import { useEffect, useState } from "react";
import { healthCheck } from "@/lib/cellp-api";

export function ApiHealthBanner() {
  const [message, setMessage] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      try {
        await healthCheck();
        if (!cancelled) setMessage(null);
      } catch {
        if (!cancelled) {
          setMessage(
            "Cannot reach cellpd API. Run ./dev/scripts/up.sh and keep cellpd on :8790.",
          );
        }
      }
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  if (!message) return null;

  return (
    <div className="border-b border-destructive/40 bg-destructive/10 px-4 py-2 text-sm text-destructive md:px-8">
      {message}
    </div>
  );
}
