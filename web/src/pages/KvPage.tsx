import { useEffect, useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { Layers } from "lucide-react";
import {
  bindingsErrorMessage,
  getBindings,
  getProject,
  getVersion,
  listKvNamespaces,
  type Bindings,
  type KvNamespace,
  type Version,
} from "@/lib/cellp-api";
import { storageKvHref } from "@/lib/routes";
import {
  BindingSurfaceLayout,
  OperatorNotReadyState,
  isPreviewVersion,
} from "@/components/bindings/binding-surface";
import { EmptyState } from "@/components/empty-state";
import { KvBrowser } from "@/components/kv/kv-browser";

export function KvPage() {
  const { id = "", vid = "" } = useParams<{ id: string; vid: string }>();
  const [version, setVersion] = useState<Version | null>(null);
  const [prodVersionId, setProdVersionId] = useState<string | null>(null);
  const [bindings, setBindings] = useState<Bindings | null>(null);
  const [namespaces, setNamespaces] = useState<KvNamespace[]>([]);
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
        const [project, v] = await Promise.all([
          getProject(id),
          getVersion(id, vid),
        ]);
        if (cancelled) return;
        setProdVersionId(project.prod_version_id);
        setVersion(v);
        const [b, ns] = await Promise.all([
          getBindings(id, vid),
          listKvNamespaces(id, vid),
        ]);
        if (cancelled) return;
        setBindings(b);
        setNamespaces(ns.namespaces);
      } catch (e) {
        if (!cancelled) {
          setBindings(null);
          setNamespaces([]);
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

  const nsList = useMemo(() => {
    if (namespaces.length) return namespaces;
    return (bindings?.kv ?? []).map((k) => ({ id: k.id, binding: k.binding }));
  }, [namespaces, bindings]);

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
      title="KV"
      crumb="KV"
      icon={<Layers className="size-5 text-muted-foreground" />}
      versionHref={storageKvHref}
      isProd={prodVersionId === vid}
      isPreview={isPreviewVersion(version)}
      gitRef={version?.git_ref}
      loading={loading}
    >
      {!loading && !fatal && nsList.length === 0 && (
        <EmptyState
          title="No KV namespaces"
          description="This deployment does not declare kv_namespaces in wrangler."
        />
      )}
      {!loading && !fatal && nsList.length > 0 && (
        <KvBrowser
          key={`${id}:${vid}`}
          projectId={id}
          versionId={vid}
          namespaces={nsList}
        />
      )}
    </BindingSurfaceLayout>
  );
}
