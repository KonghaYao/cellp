import { ExternalLink } from "lucide-react";
import { gatewayBase } from "@/lib/format";

export function commerceStorefrontUrl(
  projectId: string,
  versionId: string,
  _prodUrl?: string | null,
): string {
  return `${gatewayBase()}/${projectId}/${versionId}/`;
}

export function CommerceStorefrontEmbed({
  projectId,
  versionId,
  prodUrl,
}: {
  projectId: string;
  versionId: string;
  prodUrl?: string | null;
}) {
  const src = commerceStorefrontUrl(projectId, versionId, prodUrl);

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 className="text-label-14 font-medium">Storefront</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Live app UI on the gateway — D1, KV, Queue, Workflow, R2, Cron
          </p>
        </div>
        <a
          href={src}
          target="_blank"
          rel="noreferrer"
          className="inline-flex h-8 items-center gap-2 rounded-md border border-border bg-card px-3 text-sm font-medium transition-colors hover:bg-muted"
        >
          Open full screen
          <ExternalLink className="size-3.5 text-muted-foreground" />
        </a>
      </div>
      <div className="overflow-hidden rounded-md border border-border bg-card">
        <iframe
          title="Commerce storefront"
          src={src}
          className="h-[min(720px,70vh)] w-full border-0 bg-[#0f1117]"
        />
      </div>
    </div>
  );
}
