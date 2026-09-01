import { ExternalLink } from "lucide-react";
import {
  previewBrowseUrl,
  prodBrowseUrl,
  previewHostFromApi,
} from "@/lib/format";

export function commerceStorefrontUrl(
  projectId: string,
  versionId: string,
  prodUrl?: string | null,
  isProd?: boolean,
): string {
  if (isProd) {
    return prodBrowseUrl(projectId, prodUrl);
  }
  return previewBrowseUrl(projectId, versionId);
}

export function CommerceStorefrontEmbed({
  projectId,
  versionId,
  prodUrl,
  isProd = false,
  previewUrl,
}: {
  projectId: string;
  versionId: string;
  prodUrl?: string | null;
  isProd?: boolean;
  previewUrl?: string | null;
}) {
  const src = isProd
    ? prodBrowseUrl(projectId, prodUrl)
    : previewBrowseUrl(projectId, versionId, previewUrl);

  const hostLabel = (() => {
    try {
      if (prodUrl && isProd) return new URL(prodUrl).host;
      if (previewUrl) return new URL(previewUrl).host;
    } catch {
      /* fall through */
    }
    return previewHostFromApi(projectId, versionId, previewUrl);
  })();

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2 className="text-label-14 font-medium">Storefront</h2>
          <p className="mt-1 text-sm text-muted-foreground">
            Live Worker on gateway (Host{" "}
            <span className="font-mono text-xs">{hostLabel}</span>)
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
