import { useMemo } from "react";
import { Link } from "react-router-dom";
import { GitBranch } from "lucide-react";
import type { Version } from "@/lib/cellp-api";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";

interface BranchTreeProps {
  projectId: string;
  versionId: string;
  prodVersionId: string | null;
  versions: Version[];
  className?: string;
}

interface TreeNode {
  version: Version;
  children: TreeNode[];
}

function buildTree(versions: Version[]): TreeNode[] {
  const byId = new Map(versions.map((v) => [v.id, v]));
  const childrenByParent = new Map<string | null, Version[]>();

  for (const version of versions) {
    const parentId =
      version.parent_version_id && byId.has(version.parent_version_id)
        ? version.parent_version_id
        : null;
    const siblings = childrenByParent.get(parentId) ?? [];
    siblings.push(version);
    childrenByParent.set(parentId, siblings);
  }

  function toNodes(parentId: string | null): TreeNode[] {
    const siblings = childrenByParent.get(parentId) ?? [];
    return siblings
      .sort((a, b) => {
        const aTime = a.created_at ? new Date(a.created_at).getTime() : 0;
        const bTime = b.created_at ? new Date(b.created_at).getTime() : 0;
        return aTime - bTime;
      })
      .map((version) => ({
        version,
        children: toNodes(version.id),
      }));
  }

  return toNodes(null);
}

import { storageBrowserHref } from "@/lib/routes";

function BranchNode({
  projectId,
  node,
  versionId,
  prodVersionId,
  depth,
}: {
  projectId: string;
  node: TreeNode;
  versionId: string;
  prodVersionId: string | null;
  depth: number;
}) {
  const { version, children } = node;
  const isCurrent = version.id === versionId;
  const isProd = prodVersionId === version.id;

  return (
    <li className="relative">
      <div
        className="flex items-start gap-2"
        style={{ paddingLeft: depth > 0 ? `${depth * 1.25}rem` : 0 }}
      >
        {depth > 0 && (
          <span
            aria-hidden
            className="absolute left-0 top-3 h-px w-4 bg-border"
            style={{ marginLeft: `${(depth - 1) * 1.25}rem` }}
          />
        )}
        <Link
          to={storageBrowserHref(projectId, version.id)}
          className={cn(
            "group flex min-w-0 flex-1 items-center gap-2 rounded-md border px-3 py-2 transition-colors",
            isCurrent
              ? "border-primary/30 bg-accent"
              : "border-border bg-card hover:bg-muted/50",
            isProd && !isCurrent && "border-emerald-200/80",
          )}
        >
          <GitBranch className="size-3.5 shrink-0 text-muted-foreground" />
          <div className="min-w-0 flex-1">
            <div className="flex flex-wrap items-center gap-2">
              <span className="truncate font-mono text-sm">{version.id}</span>
              {isProd && <Badge variant="prod">Production</Badge>}
              {isCurrent && (
                <Badge variant="outline" className="text-foreground">
                  Current
                </Badge>
              )}
            </div>
            <p className="truncate text-xs text-muted-foreground">
              {version.git_ref || "—"}
            </p>
          </div>
          <span className="shrink-0 text-xs capitalize text-muted-foreground">
            {version.status}
          </span>
        </Link>
      </div>
      {children.length > 0 && (
        <ul className="mt-1 space-y-1 border-l border-border/70 pl-3">
          {children.map((child) => (
            <BranchNode
              key={child.version.id}
              projectId={projectId}
              node={child}
              versionId={versionId}
              prodVersionId={prodVersionId}
              depth={depth + 1}
            />
          ))}
        </ul>
      )}
    </li>
  );
}

export function BranchTree({
  projectId,
  versionId,
  prodVersionId,
  versions,
  className,
}: BranchTreeProps) {
  const roots = useMemo(() => buildTree(versions), [versions]);

  if (versions.length === 0) {
    return (
      <Card className={className}>
        <CardContent className="py-8 text-center text-sm text-muted-foreground">
          No versions to display
        </CardContent>
      </Card>
    );
  }

  return (
    <Card className={className}>
      <CardHeader className="pb-3">
        <CardTitle className="text-base">Branch hierarchy</CardTitle>
      </CardHeader>
      <CardContent>
        <ul className="space-y-1">
          {roots.map((node) => (
            <BranchNode
              key={node.version.id}
              projectId={projectId}
              node={node}
              versionId={versionId}
              prodVersionId={prodVersionId}
              depth={0}
            />
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}
