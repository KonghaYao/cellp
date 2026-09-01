import { CopyButton } from "@/components/copy-button";
import {
  gatewayPublicPort,
  ingressBaseDomain,
  ingressDisplayUrl,
  ingressLanHostsLine,
  ingressUsesMagicDns,
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
  const magicDns = ingressUsesMagicDns();
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
      <p className="font-medium text-foreground">
        {magicDns ? "局域网 Preview（nip.io / 免 hosts）" : "局域网 Preview / Prod（Gateway）"}
      </p>
      <p className="mt-1">
        Gateway 按 <span className="font-mono">Host</span> 选版本，默认端口{" "}
        <span className="font-mono">{port}</span>。cellpd 监听{" "}
        <span className="font-mono">0.0.0.0:{port}</span>，同事需能访问你机器的该端口。
      </p>
      {magicDns ? (
        <p className="mt-2">
          Base domain <span className="font-mono">{ingressBaseDomain()}</span>
          ：公网 DNS 会把 Host 解析到你的局域网 IP，同事
          <strong className="font-medium text-foreground">不必改 /etc/hosts</strong>
          （需能解析 nip.io；公司网若拦截可改回{" "}
          <span className="font-mono">ingress.local</span> + hosts）。
        </p>
      ) : null}
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
      {!magicDns ? (
        <>
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
        </>
      ) : (
        <p className="mt-2">
          同事浏览器直接打开上列 URL（含 <span className="font-mono">:{port}</span>
          ）。本机启用 nip.io：{" "}
          <span className="font-mono">./dev/scripts/ingress-nip-enable.sh</span>
        </p>
      )}
      <p className="mt-2">
        然后在浏览器打开上列带 <span className="font-mono">:{port}</span>{" "}
        的 URL（不要用裸 IP，除非 Host 头正确）。
      </p>
    </div>
  );
}
