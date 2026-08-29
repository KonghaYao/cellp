import { useEffect, useState } from "react";
import { fetchVersionClient, type Version } from "@/lib/cellp-api";
import { isInProgressStatus } from "@/lib/status";

interface VersionPollingProps {
  projectId: string;
  versionId: string;
  initialVersion: Version;
  onRefresh?: () => void | Promise<void>;
  children: (version: Version) => React.ReactNode;
}

export function VersionPolling({
  projectId,
  versionId,
  initialVersion,
  onRefresh,
  children,
}: VersionPollingProps) {
  const [version, setVersion] = useState(initialVersion);

  useEffect(() => {
    setVersion(initialVersion);
  }, [initialVersion]);

  useEffect(() => {
    if (!isInProgressStatus(version.status)) return;

    const id = setInterval(async () => {
      try {
        const next = await fetchVersionClient(projectId, versionId);
        setVersion(next);
        if (!isInProgressStatus(next.status)) {
          await onRefresh?.();
        }
      } catch {
        /* keep polling on transient errors */
      }
    }, 3000);

    return () => clearInterval(id);
  }, [projectId, versionId, version.status, onRefresh]);

  return <>{children(version)}</>;
}
