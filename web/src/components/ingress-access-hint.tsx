import { CopyButton } from "@/components/copy-button";
import {
  gatewayPublicPort,
  ingressDisplayUrl,
  ingressLanHostsLine,
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
  const hostsLine = ingressLanHostsLine(projectId, versionId);

  return (
    <div
      className="rounded-md border border-border bg-muted/30 px-4 py-3 text-xs text-muted-foreground"
      data-testid="ingress-lan-hint"
    >
      <p className="font-medium text-foreground">局域网 Preview / Prod（Gateway）</p>
      <p className="mt-1">
        Gateway 按 <span className="font-mono">Host</span> 选版本，默认端口{" "}
        <span className="font-mono">{port}</span>。cellpd 监听{" "}
        <span className="font-mono">0.0.0.0:{port}</span>，同事需能访问你机器的该端口。
      </p>
      <ul className="mt-2 list-inside list-disc space-y-1 font-mono text-[11px] text-foreground/90">
        {versionId ? (
          <li>
            Preview Host: {previewHost(projectId, versionId)} → {previewLabel}
          </li>
        ) : null}
        <li>
          Prod Host: {prodHost(projectId)} → {prodLabel}
        </li>
      </ul>
      <p className="mt-2">
        同事在本机 <span className="font-mono">/etc/hosts</span> 增加一行（把 IP
        换成你的局域网地址）：
      </p>
      <div className="mt-1 flex flex-wrap items-center gap-2">
        <code className="break-all rounded bg-background px-2 py-1 font-mono text-[11px]">
          {hostsLine}
        </code>
        <CopyButton value={hostsLine} label="Copy hosts line" />
      </div>
      <p className="mt-2">
        然后在浏览器打开上列带 <span className="font-mono">:{port}</span>{" "}
        的 URL（不要用裸 <span className="font-mono">127.0.0.1</span>
        ，除非 Host 头正确）。
      </p>
    </div>
  );
}
