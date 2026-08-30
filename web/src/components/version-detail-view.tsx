import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import {
  Clock,
  Database,
  ExternalLink,
  GitBranch,
  Globe,
  Layers,
} from "lucide-react";
import {
  checkDatabaseAvailability,
  type DatabaseAvailability,
  type Version,
} from "@/lib/cellp-api";
import {
  resolveProdUrl,
  formatDateTime,
  formatDuration,
  formatRelativeTime,
  truncateSha,
} from "@/lib/format";
import {
  STATUS_TIMELINE,
  timelineIndex,
  statusLabel,
  STATUS_DOT,
} from "@/lib/status";
import { storageBrowserHref, versionHref } from "@/lib/routes";
import { CopyButton } from "@/components/copy-button";
import { StatusIndicator } from "@/components/status-indicator";
import { VersionActions } from "@/components/version-actions";
import { VersionPolling } from "@/components/version-polling";
import { EnvEditor } from "@/components/env-editor";
import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";

interface VersionDetailViewProps {
  projectId: string;
  versionId: string;
  initialVersion: Version;
  prodVersionId: string | null;
  prodUrl?: string | null;
  onRefresh?: () => void | Promise<void>;
}

export function VersionDetailView({
  projectId,
  versionId,
  initialVersion,
  prodVersionId,
  prodUrl: projectProdUrl,
  onRefresh,
}: VersionDetailViewProps) {
  return (
    <VersionPolling
      projectId={projectId}
      versionId={versionId}
      initialVersion={initialVersion}
      onRefresh={onRefresh}
    >
      {(version) => (
        <VersionDetailContent
          projectId={projectId}
          version={version}
          prodVersionId={prodVersionId}
          projectProdUrl={projectProdUrl}
          onRefresh={onRefresh}
        />
      )}
    </VersionPolling>
  );
}

function VersionDetailContent({
  projectId,
  version,
  prodVersionId,
  projectProdUrl,
  onRefresh,
}: {
  projectId: string;
  version: Version;
  prodVersionId: string | null;
  projectProdUrl?: string | null;
  onRefresh?: () => void | Promise<void>;
}) {
  const isProd = prodVersionId === version.id;
  const isPreview = version.parent_version_id != null;
  const prodUrl = resolveProdUrl(projectId, projectProdUrl, version.preview_url);
  const [databaseAvailability, setDatabaseAvailability] =
    useState<DatabaseAvailability | null>(null);
  const [parentDatabaseAvailability, setParentDatabaseAvailability] =
    useState<DatabaseAvailability | null>(null);

  useEffect(() => {
    let cancelled = false;
    if (version.status !== "ready") {
      setDatabaseAvailability(null);
      return;
    }
    (async () => {
      const result = await checkDatabaseAvailability(projectId, version.id);
      if (!cancelled) setDatabaseAvailability(result);
    })();
    return () => {
      cancelled = true;
    };
  }, [projectId, version.id, version.status]);

  useEffect(() => {
    let cancelled = false;
    const parentId = version.parent_version_id;
    if (!parentId) {
      setParentDatabaseAvailability(null);
      return;
    }
    (async () => {
      const result = await checkDatabaseAvailability(projectId, parentId);
      if (!cancelled) setParentDatabaseAvailability(result);
    })();
    return () => {
      cancelled = true;
    };
  }, [projectId, version.parent_version_id]);

  return (
    <div className="space-y-8">
      <div className="rounded-lg border border-border bg-card p-6">
        <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
          <div>
            <div className="flex flex-wrap items-center gap-3">
              <h1 className="font-mono text-2xl font-semibold tracking-tight">
                {version.id}
              </h1>
              <StatusIndicator status={version.status} />
              {isProd && <Badge variant="prod">Production</Badge>}
              {version.status === "archived" && (
                <Badge variant="secondary">Archived</Badge>
              )}
              {version.pinned && <Badge variant="outline">Pinned</Badge>}
              {isPreview && <Badge variant="secondary">Preview branch</Badge>}
            </div>
            <p className="mt-2 text-sm text-muted-foreground">
              Deployed {formatRelativeTime(version.created_at)} ·{" "}
              {formatDuration(version.created_at, version.ready_at)}
              {version.ready_at ? " to ready" : ""}
            </p>
          </div>
          <VersionActions
            projectId={projectId}
            versionId={version.id}
            status={version.status}
            isProd={isProd}
            pinned={version.pinned}
            previewUrl={version.preview_url}
            onComplete={onRefresh}
          />
        </div>

        <StatusTimeline status={version.status} className="mt-8" />

        {version.status === "archived" && (
          <div className="mt-6 rounded-md border border-border bg-muted/40 px-4 py-3 text-sm text-muted-foreground">
            <p>
              This version is <strong className="font-medium text-foreground">archived</strong>:
              the celld process is stopped and preview URLs return{" "}
              <span className="font-mono">503 version_archived</span>. Data remains in
              S3 — use <strong className="font-medium text-foreground">Wake</strong> to
              restore, or re-promote after wake for production rollback.
            </p>
            <p className="mt-2">
              Ready versions auto-archive after{" "}
              <span className="font-mono">45m</span> idle (
              <span className="font-mono">CELLP_ARCHIVE_IDLE</span>).{" "}
              <strong className="font-medium text-foreground">Pin</strong> to keep a
              long-lived QA preview; prod keeps a{" "}
              <span className="font-mono">60m</span> rollback window after promote.
            </p>
          </div>
        )}
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <MetadataSection title="Deployment" icon={<Layers className="size-4" />}>
          <MetadataRow label="Version ID" value={version.id} mono copyable />
          <MetadataRow
            label="Parent version"
            mono
            copyable={Boolean(version.parent_version_id)}
            copyValue={version.parent_version_id ?? undefined}
          >
            {version.parent_version_id ? (
              <div className="flex flex-col items-end gap-1">
                <Link
                  to={versionHref(projectId, version.parent_version_id)}
                  className="font-mono text-xs hover:underline"
                >
                  {version.parent_version_id}
                </Link>
                {parentDatabaseAvailability?.available === true && (
                  <Link
                    to={storageBrowserHref(
                      projectId,
                      version.parent_version_id,
                    )}
                    className="inline-flex items-center gap-1 text-xs text-foreground hover:underline"
                  >
                    <Database className="size-3" />
                    Parent D1
                  </Link>
                )}
              </div>
            ) : (
              <span className="font-mono text-xs">—</span>
            )}
          </MetadataRow>
          {isPreview &&
            databaseAvailability?.available === true &&
            databaseAvailability.database.branch_method && (
              <MetadataRow
                label="D1 branch method"
                value={formatBranchMethod(
                  databaseAvailability.database.branch_method,
                )}
              />
            )}
          {isPreview && (
            <MetadataRow label="Binding branch">
              <span className="text-xs">
                D1 · KV · R2 · Queue from parent
              </span>
            </MetadataRow>
          )}
          <MetadataRow label="Status" value={statusLabel(version.status)} />
          <MetadataRow
            label="Duration"
            value={formatDuration(version.created_at, version.ready_at)}
          />
        </MetadataSection>

        <MetadataSection title="Git" icon={<GitBranch className="size-4" />}>
          <MetadataRow label="Branch" value={version.git_ref || "—"} />
          <MetadataRow
            label="Commit"
            value={truncateSha(version.git_sha, 12)}
            mono
            copyable={Boolean(version.git_sha)}
            copyValue={version.git_sha}
          />
          <MetadataRow label="Data branch" mono>
            <div className="flex items-center gap-2">
              <span className="font-mono text-xs">
                {version.data_branch || "—"}
              </span>
              {version.status === "ready" &&
                databaseAvailability?.available === true && (
                  <Link
                    to={storageBrowserHref(projectId, version.id)}
                    className="inline-flex items-center gap-1 text-xs text-foreground hover:underline"
                  >
                    <Database className="size-3" />
                    Open
                  </Link>
                )}
              {version.status === "ready" &&
                databaseAvailability &&
                !databaseAvailability.available && (
                  <span className="text-xs text-muted-foreground">
                    {databaseAvailability.message}
                  </span>
                )}
            </div>
          </MetadataRow>
          {version.artifact_digest && (
            <MetadataRow
              label="Artifact digest"
              value={truncateSha(version.artifact_digest, 16)}
              mono
              copyable
              copyValue={version.artifact_digest}
            />
          )}
        </MetadataSection>

        <MetadataSection title="Timestamps" icon={<Clock className="size-4" />}>
          <MetadataRow
            label="Created"
            value={formatDateTime(version.created_at)}
          />
          <MetadataRow
            label="Updated"
            value={formatDateTime(version.updated_at)}
          />
          <MetadataRow
            label="Ready at"
            value={version.ready_at ? formatDateTime(version.ready_at) : "—"}
          />
          {version.ttl && (
            <MetadataRow label="TTL" value={formatDateTime(version.ttl)} />
          )}
        </MetadataSection>

        <MetadataSection title="URLs" icon={<Globe className="size-4" />}>
          <MetadataRow label="Preview" copyable copyValue={version.preview_url}>
            <a
              href={version.preview_url}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-1 text-sm text-foreground hover:underline"
            >
              {version.preview_url}
              <ExternalLink className="size-3 text-muted-foreground" />
            </a>
          </MetadataRow>
          <MetadataRow label="Production" copyable copyValue={prodUrl}>
            <a
              href={prodUrl}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-1 text-sm text-foreground hover:underline"
            >
              {prodUrl}
              <ExternalLink className="size-3 text-muted-foreground" />
            </a>
          </MetadataRow>
        </MetadataSection>
      </div>

      {(version.status === "ready" ||
        version.status === "archived" ||
        version.status === "pending" ||
        version.status === "deploying") && (
        <EnvEditor projectId={projectId} versionId={version.id} />
      )}

      {version.error && (
        <div className="rounded-lg border border-destructive/40 bg-destructive/10 px-4 py-3 text-sm text-red-600">
          <span className="font-medium">Error: </span>
          {version.error}
        </div>
      )}
    </div>
  );
}

function StatusTimeline({
  status,
  className,
}: {
  status: string;
  className?: string;
}) {
  const currentIdx = timelineIndex(status);
  const isFailed = status === "failed";
  const isTerminal =
    status === "destroyed" || status === "draining" || isFailed;

  return (
    <div className={cn("flex items-center gap-0", className)}>
      {STATUS_TIMELINE.map((step, i) => {
        const done = currentIdx >= i && !isFailed;
        const active = status === step;
        const dot = STATUS_DOT[step] ?? "bg-zinc-500";

        return (
          <div key={step} className="flex flex-1 items-center">
            <div className="flex flex-col items-center gap-1.5">
              <span
                className={cn(
                  "size-2.5 rounded-full",
                  done || active ? dot : "bg-muted",
                  active && "ring-2 ring-ring ring-offset-2 ring-offset-card",
                )}
              />
              <span
                className={cn(
                  "hidden text-[10px] uppercase tracking-wide sm:block",
                  done || active
                    ? "text-foreground"
                    : "text-muted-foreground",
                )}
              >
                {step}
              </span>
            </div>
            {i < STATUS_TIMELINE.length - 1 && (
              <div
                className={cn(
                  "mx-1 h-px flex-1",
                  currentIdx > i && !isFailed ? "bg-border" : "bg-muted",
                )}
              />
            )}
          </div>
        );
      })}
      {isTerminal && (
        <div className="ml-4 flex flex-col items-center gap-1.5">
          <span className={cn("size-2.5 rounded-full", STATUS_DOT[status])} />
          <span className="hidden text-[10px] uppercase tracking-wide sm:block">
            {status}
          </span>
        </div>
      )}
    </div>
  );
}

function MetadataSection({
  title,
  icon,
  children,
}: {
  title: string;
  icon: React.ReactNode;
  children: React.ReactNode;
}) {
  return (
    <div className="rounded-lg border border-border bg-card p-5">
      <div className="mb-4 flex items-center gap-2 text-sm font-medium">
        {icon}
        {title}
      </div>
      <dl className="space-y-3">{children}</dl>
    </div>
  );
}

function formatBranchMethod(method: string): string {
  return method.replace(/_/g, " ");
}

function MetadataRow({
  label,
  value,
  mono,
  copyable,
  copyValue,
  children,
}: {
  label: string;
  value?: string;
  mono?: boolean;
  copyable?: boolean;
  copyValue?: string;
  children?: React.ReactNode;
}) {
  const copy = copyValue ?? value;

  return (
    <div className="flex items-start justify-between gap-4 text-sm">
      <dt className="shrink-0 text-muted-foreground">{label}</dt>
      <dd className="flex items-center gap-1 text-right">
        {children ?? (
          <span className={mono ? "font-mono text-xs" : undefined}>{value}</span>
        )}
        {copyable && copy && <CopyButton value={copy} label={`Copy ${label}`} />}
      </dd>
    </div>
  );
}
