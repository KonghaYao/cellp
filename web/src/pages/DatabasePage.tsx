import { useCallback, useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { Database, GitBranch } from "lucide-react";
import {
  getDatabase,
  getProject,
  getVersion,
  listVersions,
  CellpApiError,
  type DatabaseMetadata,
  type DatabaseUnavailableReason,
  type Version,
} from "@/lib/cellp-api";
import { storageBrowserHref, versionHref } from "@/lib/routes";
import { BranchList } from "@/components/database/branch-list";
import { BranchTree } from "@/components/database/branch-tree";
import { CreateBranchDialog } from "@/components/database/create-branch-dialog";
import { SchemaBrowser } from "@/components/database/schema-browser";
import { SqlEditor } from "@/components/database/sql-editor";
import { TableDataViewer } from "@/components/database/table-data-viewer";
import { CopyButton } from "@/components/copy-button";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

async function loadAllVersions(projectId: string): Promise<Version[]> {
  const all: Version[] = [];
  let cursor: string | null = null;
  do {
    const page = await listVersions(projectId, { cursor });
    all.push(...page.versions);
    cursor = page.next_cursor;
  } while (cursor);
  return all;
}

type LoadErrorKind = "version_not_found" | DatabaseUnavailableReason | "server";

function classifyDatabaseError(e: unknown): {
  kind: LoadErrorKind;
  message: string;
} {
  if (e instanceof CellpApiError) {
    if (e.status === 404) {
      const code =
        typeof e.body === "object" &&
        e.body !== null &&
        "error" in e.body &&
        typeof (e.body as { error: unknown }).error === "string"
          ? (e.body as { error: string }).error
          : "";
      if (code === "version_not_found" || code === "project_not_found") {
        return { kind: "version_not_found", message: "Version not found" };
      }
      if (code === "version_not_ready" || code.includes("not_ready")) {
        return {
          kind: "not_ready",
          message: "Deployment is not ready yet — database is not available",
        };
      }
      return {
        kind: "not_found",
        message: "No database attached to this deployment",
      };
    }
    return {
      kind: "server",
      message: `${e.message} (${e.status})`,
    };
  }
  return {
    kind: "network",
    message: "Could not reach the API — check your connection and try again",
  };
}

function DatabaseErrorState({
  kind,
  message,
  projectId,
}: {
  kind: LoadErrorKind;
  message: string;
  projectId: string;
}) {
  const title =
    kind === "version_not_found"
      ? "Version not found"
      : kind === "not_ready"
        ? "Database not ready"
        : kind === "not_found"
          ? "Database not found"
          : kind === "network"
            ? "Connection error"
            : "Failed to load database";

  return (
    <div className="space-y-4 py-16 text-center">
      <h1 className="text-2xl font-semibold">{title}</h1>
      <p className="text-muted-foreground">{message}</p>
      <p>
        <Link to={`/projects/${projectId}/storage`} className="hover:underline">
          Back to storage
        </Link>
      </p>
    </div>
  );
}

export function DatabasePage() {
  const { id = "", vid = "" } = useParams<{ id: string; vid: string }>();
  const [database, setDatabase] = useState<DatabaseMetadata | null>(null);
  const [version, setVersion] = useState<Version | null>(null);
  const [versions, setVersions] = useState<Version[]>([]);
  const [prodVersionId, setProdVersionId] = useState<string | null>(null);
  const [selectedTable, setSelectedTable] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [fatalError, setFatalError] = useState<{
    kind: LoadErrorKind;
    message: string;
  } | null>(null);

  const loadData = useCallback(async () => {
    const [project, v, allVersions] = await Promise.all([
      getProject(id),
      getVersion(id, vid),
      loadAllVersions(id),
    ]);
    setVersion(v);
    setVersions(allVersions);
    setProdVersionId(project.prod_version_id);

    try {
      const db = await getDatabase(id, vid);
      setDatabase(db);
      setFatalError(null);
    } catch (e) {
      setDatabase(null);
      setFatalError(classifyDatabaseError(e));
      throw e;
    }
  }, [id, vid]);

  useEffect(() => {
    let cancelled = false;
    (async () => {
      setLoading(true);
      setSelectedTable(null);
      setFatalError(null);
      try {
        const [project, v, allVersions] = await Promise.all([
          getProject(id),
          getVersion(id, vid),
          loadAllVersions(id),
        ]);
        if (cancelled) return;
        setVersion(v);
        setVersions(allVersions);
        setProdVersionId(project.prod_version_id);

        try {
          const db = await getDatabase(id, vid);
          if (cancelled) return;
          setDatabase(db);
          setFatalError(null);
        } catch (e) {
          if (!cancelled) {
            setDatabase(null);
            setFatalError(classifyDatabaseError(e));
          }
        }
      } catch (e) {
        if (!cancelled) {
          if (e instanceof CellpApiError && e.status === 404) {
            setFatalError({
              kind: "version_not_found",
              message: "Version not found",
            });
          } else {
            setFatalError(classifyDatabaseError(e));
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

  if (!loading && fatalError) {
    return (
      <DatabaseErrorState
        kind={fatalError.kind}
        message={fatalError.message}
        projectId={id}
      />
    );
  }

  const isProd = prodVersionId === vid;

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center gap-3">
        <Database className="size-5 text-muted-foreground" />
        <h2 className="text-heading-20 font-semibold tracking-tight">
          {database?.database_name ?? "Database"}
        </h2>
        {isProd && <Badge variant="prod">Production</Badge>}
      </div>

      {loading && (
        <div className="space-y-4">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-64 w-full" />
        </div>
      )}

      {!loading && !fatalError && database && version && (
        <>
          <Card>
            <CardHeader className="pb-3">
              <CardTitle className="text-base">Connection</CardTitle>
            </CardHeader>
            <CardContent>
              <dl className="grid gap-3 text-sm sm:grid-cols-2 lg:grid-cols-4">
                <MetadataItem
                  label="Database"
                  value={database.database_name}
                  mono
                />
                <MetadataItem
                  label="Database ID"
                  value={database.database_id}
                  mono
                  copyable
                />
                <MetadataItem
                  label="Data branch"
                  value={database.data_branch}
                  mono
                  copyable
                />
                <MetadataItem
                  label="Branch method"
                  value={database.branch_method.replace("_", " ")}
                />
                {database.parent_version_id && (
                  <MetadataItem
                    label="Parent version"
                    value={database.parent_version_id}
                    mono
                    href={storageBrowserHref(id, database.parent_version_id)}
                  />
                )}
                <MetadataItem label="Git branch" value={version.git_ref || "—"} />
              </dl>
            </CardContent>
          </Card>

          <Tabs defaultValue="data">
            <TabsList>
              <TabsTrigger value="query">Query</TabsTrigger>
              <TabsTrigger value="data">Data Editor</TabsTrigger>
              <TabsTrigger value="schema">Schema</TabsTrigger>
              <TabsTrigger value="branches">Branches</TabsTrigger>
            </TabsList>

            <TabsContent value="query">
              <SqlEditor projectId={id} versionId={vid} />
            </TabsContent>

            <TabsContent value="data">
              <div className="grid gap-6 lg:grid-cols-[minmax(12rem,16rem)_1fr]">
                <SchemaBrowser
                  projectId={id}
                  versionId={vid}
                  selectedTable={selectedTable}
                  onSelectTable={setSelectedTable}
                />
                <div>
                  {selectedTable ? (
                    <TableDataViewer
                      projectId={id}
                      versionId={vid}
                      tableName={selectedTable}
                    />
                  ) : (
                    <div className="flex h-48 items-center justify-center rounded-lg border border-dashed border-border text-sm text-muted-foreground">
                      Select a table to view its data
                    </div>
                  )}
                </div>
              </div>
            </TabsContent>

            <TabsContent value="schema">
              <div className="max-w-xs">
                <SchemaBrowser
                  projectId={id}
                  versionId={vid}
                  selectedTable={selectedTable}
                  onSelectTable={setSelectedTable}
                />
              </div>
            </TabsContent>

            <TabsContent value="branches">
              <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
                <p className="text-sm text-muted-foreground">
                  {versions.length} branch{versions.length === 1 ? "" : "es"}
                </p>
                <CreateBranchDialog
                  projectId={id}
                  versions={versions}
                  defaultParentId={vid}
                  onCreated={loadData}
                />
              </div>

              <div className="grid gap-6 lg:grid-cols-2">
                <BranchTree
                  projectId={id}
                  versionId={vid}
                  prodVersionId={prodVersionId}
                  versions={versions}
                />
                <Card>
                  <CardHeader className="pb-3">
                    <CardTitle className="flex items-center gap-2 text-base">
                      <GitBranch className="size-4" />
                      Current branch
                    </CardTitle>
                  </CardHeader>
                  <CardContent className="space-y-3 text-sm">
                    <dl className="grid gap-2">
                      <div className="flex justify-between gap-4">
                        <dt className="text-muted-foreground">Version</dt>
                        <dd className="font-mono">{vid}</dd>
                      </div>
                      <div className="flex justify-between gap-4">
                        <dt className="text-muted-foreground">Parent</dt>
                        <dd className="font-mono">
                          {version.parent_version_id ? (
                            <Link
                              to={versionHref(id, version.parent_version_id)}
                              className="hover:underline"
                            >
                              {version.parent_version_id}
                            </Link>
                          ) : (
                            "—"
                          )}
                        </dd>
                      </div>
                      <div className="flex justify-between gap-4">
                        <dt className="text-muted-foreground">Git ref</dt>
                        <dd>{version.git_ref || "—"}</dd>
                      </div>
                    </dl>
                    <Link
                      to={versionHref(id, vid)}
                      className="inline-flex items-center rounded-md border border-border bg-card px-3 py-2 text-sm transition-colors hover:bg-muted"
                    >
                      Version details
                    </Link>
                  </CardContent>
                </Card>
              </div>

              <div className="mt-6">
                <h3 className="mb-3 text-sm font-medium text-muted-foreground">
                  All branches
                </h3>
                <BranchList
                  projectId={id}
                  currentVersionId={vid}
                  prodVersionId={prodVersionId}
                  versions={versions}
                  onRefresh={loadData}
                />
              </div>
            </TabsContent>
          </Tabs>
        </>
      )}
    </div>
  );
}

function MetadataItem({
  label,
  value,
  mono,
  copyable,
  href,
}: {
  label: string;
  value: string;
  mono?: boolean;
  copyable?: boolean;
  href?: string;
}) {
  return (
    <div>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="mt-0.5 flex items-center gap-1">
        {href ? (
          <Link
            to={href}
            className={mono ? "font-mono text-xs hover:underline" : "hover:underline"}
          >
            {value}
          </Link>
        ) : (
          <span className={mono ? "font-mono text-xs" : undefined}>{value}</span>
        )}
        {copyable && <CopyButton value={value} label={`Copy ${label}`} />}
      </dd>
    </div>
  );
}
