import { CopyButton } from "@/components/copy-button";
import {
  gatewayPublicPort,
  ingressDisplayUrl,
  previewHost,
  prodHost,
} from "@/lib/format";

export function IngressAccessHint({
  projectId,
  versionId,
  previewUrl,
  prodUrl,
}: {
  projectId: string;
  versionId?: string;
  previewUrl?: string | null;
  prodUrl?: string | null;
}) {
  const port = gatewayPublicPort();
  const previewLabel = ingressDisplayUrl(
    projectId,
    versionId,
    previewUrl ?? undefined,
  );
  const prodLabel = ingressDisplayUrl(projectId, undefined, prodUrl ?? undefined);

  return (
    <div
      className="rounded-md border border-border bg-muted/30 px-4 py-3 text-xs text-muted-foreground"
      data-testid="ingress-lan-hint"
    >
      <p className="font-medium text-foreground">Gateway（lvh.me）</p>
      <p className="mt-1">
        按 <span className="font-mono">Host</span> 选版本，端口{" "}
        <span className="font-mono">{port}</span>。浏览器用{" "}
        <span className="font-mono">http://…lvh.me:{port}/</span>，不要用裸{" "}
        <span className="font-mono">127.0.0.1</span>。
      </p>
      <p className="mt-2 rounded border border-amber-500/30 bg-amber-500/10 px-2 py-1.5 text-foreground/90">
        Clash 需直连 <span className="font-mono">lvh.me</span>（见{" "}
        <span className="font-mono">dev/clash/README.md</span>），否则浏览器可能 502。
      </p>
      <ul className="mt-2 list-inside list-disc space-y-1 font-mono text-[11px] text-foreground/90">
        {versionId ? (
          <li>
            Preview: {previewHost(projectId, versionId)} → {previewLabel}
          </li>
        ) : null}
        <li>
          Prod: {prodHost(projectId)} → {prodLabel}
        </li>
      </ul>
      <p className="mt-2">
        换 ingress base 后：{" "}
        <span className="font-mono">./dev/scripts/ingress-host-init.sh</span>
        {" · "}
        <span className="font-mono">./dev/scripts/ingress-repromote-support.sh</span>
      </p>
      <div className="mt-2 flex flex-wrap gap-2">
        <CopyButton value={prodLabel} label="Copy prod URL" />
        {versionId ? (
          <CopyButton value={previewLabel} label="Copy preview URL" />
        ) : null}
      </div>
    </div>
  );
}
