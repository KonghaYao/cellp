import { useState } from "react";
import type { KvNamespace } from "@/lib/cellp-api";
import { KvInfoCard } from "@/components/kv/kv-info";
import { KeyEditor } from "@/components/kv/key-editor";
import { KeyList } from "@/components/kv/key-list";
import { NamespacePicker } from "@/components/kv/namespace-picker";
import { PutKeyForm } from "@/components/kv/put-key-form";

interface KvBrowserProps {
  projectId: string;
  versionId: string;
  namespaces: KvNamespace[];
}

export function KvBrowser({
  projectId,
  versionId,
  namespaces,
}: KvBrowserProps) {
  const [selectedNs, setSelectedNs] = useState<string | null>(
    namespaces[0]?.id ?? null,
  );
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const [refreshToken, setRefreshToken] = useState(0);

  function bump(nextKey?: string | null) {
    setRefreshToken((n) => n + 1);
    if (nextKey !== undefined) setSelectedKey(nextKey);
  }

  if (!selectedNs) return null;

  return (
    <div className="grid gap-6 lg:grid-cols-[minmax(12rem,16rem)_1fr]">
      <NamespacePicker
        namespaces={namespaces}
        selectedId={selectedNs}
        onSelect={(id) => {
          setSelectedNs(id);
          setSelectedKey(null);
        }}
      />
      <div className="space-y-6">
        <KvInfoCard
          projectId={projectId}
          versionId={versionId}
          ns={selectedNs}
          refreshToken={refreshToken}
        />
        <KeyList
          key={`${versionId}:${selectedNs}`}
          projectId={projectId}
          versionId={versionId}
          ns={selectedNs}
          selectedKey={selectedKey}
          onSelectKey={setSelectedKey}
          refreshToken={refreshToken}
        />
        <KeyEditor
          projectId={projectId}
          versionId={versionId}
          ns={selectedNs}
          selectedKey={selectedKey}
          refreshToken={refreshToken}
          onDeleted={() => bump(null)}
          onSaved={() => bump(selectedKey)}
        />
        <PutKeyForm
          projectId={projectId}
          versionId={versionId}
          ns={selectedNs}
          onPut={(key) => bump(key)}
        />
      </div>
    </div>
  );
}
