import { ExternalLink } from "lucide-react";
import {
  gatewayWorkerUrl,
  previewBrowseUrl,
  prodBrowseUrl,
  previewHostFromApi,
  workerHasHtmlStorefront,
} from "@/lib/format";

export function commerceStorefrontUrl(
  projectId: string,
  versionId: string,
  prodUrl?: string | null,
  isProd?: boolean,
): string {
  const path = workerHasHtmlStorefront(projectId) ? "/" : "/count";
  if (isProd) {
    return prodBrowseUrl(projectId, prodUrl, path);
  }
  return previewBrowseUrl(projectId, versionId, undefined, path);
}

export function CommerceStorefrontEmbed({
  projectId,
  versionId,
  prodUrl,
  isProd,
}: {
  projectId: string;
  versionId: string;
  prodUrl?: string | null;
  isProd?: boolean;
}) {
  const src = workerHasHtmlStorefront(projectId)
    ? commerceStorefrontUrl(projectId, versionId, prodUrl, isProd)
    : gatewayWorkerUrl(projectId, isProd ? null : versionId, "/count", {
        prod: isProd,
        prodUrl: prodUrl ?? undefined,
      });
  const previewUrl = prodUrl ?? undefined;
  const hostLabel = (() => {
    try {
      if (prodUrl && isProd) return new URL(prodUrl).host;
    } catch {
      /* fall through */
    }
    return previewHostFromApi(projectId, versionId, previewUrl);
  })();

  const htmlStorefront = workerHasHtmlStorefront(projectId);

  return (
    <div className="rounded-lg border border-border bg-card">
      <div className="flex flex-wrap items-center justify-between gap-2 border-b border-border px-4 py-3">
        <div>
          <h2 className="text-sm font-medium text-foreground">Live on gateway</h2>
          <p className="text-xs text-muted-foreground">
            Host <code className="text-foreground">{hostLabel}</code>
            {!htmlStorefront && (
              <> · API Worker（根路径为 JSON，非 HTML 页面）</>
            )}
          </p>
        </div>
        <a
          href={src}
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1.5 rounded-md border border-border px-3 py-1.5 text-xs font-medium hover:bg-muted"
        >
          Open via dev proxy
          <ExternalLink className="size-3.5" />
        </a>
      </div>
      {htmlStorefront ? (
        <div className="bg-muted/30 p-2">
          <iframe
            title={`Storefront ${projectId} ${versionId}`}
            src={src}
            className="h-[min(720px,70vh)] w-full border-0 bg-[#0f1117]"
          />
        </div>
      ) : (
        <div className="space-y-2 px-4 py-6 text-sm text-muted-foreground">
          <p>
            <strong className="text-foreground">{projectId}</strong> 是 bindings/API
            演示，没有整页 HTML Storefront。请在 Dashboard 里看 KV / Queue / D1，或用
            Gateway 调 API。
          </p>
          <p className="text-xs">
            不要直接收藏 <code>/__gateway/?__cellp_host=…</code> — 那是开发代理地址。
            要看带 UI 的商店，请打开项目{" "}
            <a href="/projects/commerce-store" className="text-primary underline">
              commerce-store
            </a>
            。
          </p>
          <p className="text-xs">
            试 API：<code>curl -H &quot;Host: {hostLabel}&quot; http://127.0.0.1:8787/count</code>
          </p>
        </div>
      )}
    </div>
  );
}
