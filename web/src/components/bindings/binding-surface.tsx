import type { ReactNode } from "react";
import { Link } from "react-router-dom";
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
  title,
  icon,
  isProd,
  isPreview,
  gitRef,
  loading,
  children,
}: {
  projectId: string;
  versionId: string;
  title: string;
  crumb?: string;
  icon: ReactNode;
  versionHref?: (projectId: string, versionId: string) => string;
  isProd: boolean;
  isPreview: boolean;
  gitRef?: string;
  loading: boolean;
  children: ReactNode;
}) {
  return (
    <div className="space-y-6">
      <div className="flex flex-col gap-4 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <div className="flex flex-wrap items-center gap-3">
            {icon}
            <h2 className="text-heading-20 font-semibold tracking-tight">
              {title}
            </h2>
            {isProd && <Badge variant="prod">Production</Badge>}
          </div>
          {gitRef ? (
            <p className="mt-1 text-copy-14 text-muted-foreground">{gitRef}</p>
          ) : null}
        </div>
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
