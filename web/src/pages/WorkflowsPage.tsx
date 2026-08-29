import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import { Workflow } from "lucide-react";
import {
  bindingsErrorMessage,
  getBindings,
  getProject,
  getVersion,
  listWorkflows,
  type Version,
  type WorkflowListItem,
} from "@/lib/cellp-api";
import { storageWorkflowsHref } from "@/lib/routes";
import {
  BindingSurfaceLayout,
  OperatorNotReadyState,
  isPreviewVersion,
} from "@/components/bindings/binding-surface";
import { EmptyState } from "@/components/empty-state";
import { WorkflowPanel } from "@/components/workflows/workflow-panel";

export function WorkflowsPage() {
  const { id = "", vid = "" } = useParams<{ id: string; vid: string }>();
  const [version, setVersion] = useState<Version | null>(null);
  const [prodVersionId, setProdVersionId] = useState<string | null>(null);
  const [workflows, setWorkflows] = useState<WorkflowListItem[]>([]);
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
        const listed = await listWorkflows(id, vid);
        if (cancelled) return;
        if (listed.workflows.length > 0) {
          setWorkflows(listed.workflows);
        } else {
          const bindings = await getBindings(id, vid);
          if (cancelled) return;
          setWorkflows(
            bindings.workflows.map((w) => ({
              binding: w.binding,
              workflow_name: w.name,
              class_name: w.class_name,
            })),
          );
        }
      } catch (e) {
        if (!cancelled) {
          setWorkflows([]);
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
      title="Workflows"
      crumb="Workflows"
      icon={<Workflow className="size-5 text-muted-foreground" />}
      versionHref={storageWorkflowsHref}
      isProd={prodVersionId === vid}
      isPreview={isPreviewVersion(version)}
      gitRef={version?.git_ref}
      loading={loading}
    >
      {!loading && !fatal && workflows.length === 0 && (
        <EmptyState
          title="No workflows"
          description="This deployment does not declare workflows in wrangler."
        />
      )}
      {!loading && !fatal && workflows.length > 0 && (
        <WorkflowPanel projectId={id} versionId={vid} workflows={workflows} />
      )}
    </BindingSurfaceLayout>
  );
}
