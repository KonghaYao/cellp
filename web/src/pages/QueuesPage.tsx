import { useEffect, useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { ListOrdered } from "lucide-react";
import {
  bindingsErrorMessage,
  getBindings,
  getProject,
  getVersion,
  listQueues,
  type BindingsQueue,
  type QueueListItem,
  type Version,
} from "@/lib/cellp-api";
import { storageQueuesHref } from "@/lib/routes";
import {
  BindingSurfaceLayout,
  OperatorNotReadyState,
  isPreviewVersion,
} from "@/components/bindings/binding-surface";
import { EmptyState } from "@/components/empty-state";
import {
  QueueConsole,
  mergeQueueRows,
} from "@/components/queues/queue-console";

export function QueuesPage() {
  const { id = "", vid = "" } = useParams<{ id: string; vid: string }>();
  const [version, setVersion] = useState<Version | null>(null);
  const [prodVersionId, setProdVersionId] = useState<string | null>(null);
  const [listed, setListed] = useState<QueueListItem[]>([]);
  const [bindingQueues, setBindingQueues] = useState<BindingsQueue[]>([]);
  const [loading, setLoading] = useState(true);
  const [fatal, setFatal] = useState<{ title: string; description: string } | null>(
    null,
  );

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      setFatal(null);
      try {
        const [project, v] = await Promise.all([getProject(id), getVersion(id, vid)]);
        if (cancelled) return;
        setProdVersionId(project.prod_version_id);
        setVersion(v);
        const [listedRes, bindings] = await Promise.all([
          listQueues(id, vid),
          getBindings(id, vid),
        ]);
        if (cancelled) return;
        setListed(listedRes.queues);
        setBindingQueues(bindings.queues);
      } catch (e) {
        if (!cancelled) {
          setListed([]);
          setBindingQueues([]);
          setFatal(bindingsErrorMessage(e));
          try {
            const v = await getVersion(id, vid);
            if (!cancelled) setVersion(v);
          } catch {
            /* keep version null */
          }
        }
      } finally {
        if (!cancelled) setLoading(false);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [id, vid]);

  const queues = useMemo(
    () => mergeQueueRows(listed, bindingQueues),
    [listed, bindingQueues],
  );

  if (!loading && fatal) {
    return (
      <OperatorNotReadyState
        title={fatal.title}
        description={fatal.description}
        projectId={id}
      />
    );
  }

  return (
    <BindingSurfaceLayout
      projectId={id}
      versionId={vid}
      title="Queues"
      crumb="Queues"
      icon={<ListOrdered className="size-5 text-muted-foreground" />}
      versionHref={storageQueuesHref}
      isProd={prodVersionId === vid}
      isPreview={isPreviewVersion(version)}
      gitRef={version?.git_ref}
      loading={loading}
    >
      {!loading && !fatal && queues.length === 0 && (
        <EmptyState
          title="No queues"
          description="This deployment does not declare queues in wrangler."
        />
      )}
      {!loading && !fatal && queues.length > 0 && (
        <QueueConsole projectId={id} versionId={vid} queues={queues} />
      )}
    </BindingSurfaceLayout>
  );
}
