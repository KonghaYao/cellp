import type { ReactNode } from "react";
import { Link } from "react-router-dom";
import { VersionSwitcher } from "@/components/database/version-switcher";
import { Ad7Banner } from "@/components/bindings/ad7-banner";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { storageHref } from "@/lib/routes";
import type { Version } from "@/lib/cellp-api";

export function OperatorNotReadyState({
  title,
  description,
  projectId,
}: {
  title: string;
  description: string;
  projectId: string;
}) {
  return (
    <div className="space-y-4 py-16 text-center">
      <h1 className="text-2xl font-semibold">{title}</h1>
      <p className="text-muted-foreground">{description}</p>
      <p>
        <Link to={storageHref(projectId)} className="hover:underline">
          Back to storage
        </Link>
      </p>
    </div>
  );
}

export function BindingSurfaceLayout({
  projectId,
  versionId,
  title,
  crumb,
  icon,
  versionHref,
  isProd,
  isPreview,
  gitRef,
  loading,
  children,
}: {
  projectId: string;
  versionId: string;
  title: string;
  crumb: string;
  icon: ReactNode;
  versionHref: (projectId: string, versionId: string) => string;
  isProd: boolean;
  isPreview: boolean;
  gitRef?: string;
  loading: boolean;
  children: ReactNode;
}) {
  return (
    <div className="space-y-6">
      <div className="text-label-13 text-muted-foreground">
        <Link to={storageHref(projectId)} className="hover:text-foreground">
          Storage
        </Link>
        <span className="mx-2">/</span>
        <span className="text-foreground">{crumb}</span>
      </div>

      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex flex-wrap items-center gap-3">
            {icon}
            <h1 className="text-heading-24 font-semibold tracking-tight">
              {title}
            </h1>
            {isProd && <Badge variant="prod">Production</Badge>}
          </div>
          <p className="mt-1 text-copy-14 text-muted-foreground">
            Deployment <span className="font-mono">{versionId}</span>
            {gitRef ? ` · ${gitRef}` : ""}
          </p>
        </div>
        <VersionSwitcher
          projectId={projectId}
          versionId={versionId}
          versionHref={versionHref}
        />
      </div>

      {isPreview && <Ad7Banner />}

      {loading && (
        <div className="space-y-4">
          <Skeleton className="h-24 w-full" />
          <Skeleton className="h-48 w-full" />
        </div>
      )}

      {children}
    </div>
  );
}

export function isPreviewVersion(version: Version | null): boolean {
  return version?.parent_version_id != null;
}
