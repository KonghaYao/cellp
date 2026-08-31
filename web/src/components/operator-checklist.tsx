import { ChevronDown, ExternalLink } from "lucide-react";
import { Link } from "react-router-dom";
import type { Version } from "@/lib/cellp-api";
import {
  deploymentsHref,
  inspectHref,
  platformHref,
  storageHref,
  versionHref,
} from "@/lib/routes";

/** Matches site/docs/get-started/operator-journey.md#operator-checklist */
export const OPERATOR_JOURNEY_DOC_HREF =
  "https://github.com/KonghaYao/cellp/blob/main/site/docs/get-started/operator-journey.md#operator-checklist";

export function findPromoteCandidate(
  versions: Version[],
  prodVersionId: string | null,
): Version | null {
  for (const v of versions) {
    if (v.status === "ready" && v.id !== prodVersionId) {
      return v;
    }
  }
  return null;
}

function ChecklistRow({
  done,
  emphasize,
  title,
  success,
  failureHint,
  children,
}: {
  done: boolean;
  emphasize?: boolean;
  title: string;
  success: string;
  failureHint: string;
  children?: React.ReactNode;
}) {
  return (
    <div
      className={
        emphasize
          ? "rounded-md border border-primary/30 bg-primary/5 px-3 py-3"
          : "rounded-md border border-border/60 px-3 py-3"
      }
    >
      <div className="flex items-start gap-2">
        <span
          aria-hidden
          className={
            done
              ? "mt-0.5 size-4 shrink-0 rounded-full bg-emerald-500/20 text-center text-xs leading-4 text-emerald-700 dark:text-emerald-400"
              : "mt-0.5 size-4 shrink-0 rounded-full border border-muted-foreground/40"
          }
        >
          {done ? "✓" : ""}
        </span>
        <div className="min-w-0 flex-1 space-y-1">
          <p className="font-medium">{title}</p>
          <p className="text-muted-foreground">
            {done ? success : failureHint}
          </p>
          {children}
        </div>
      </div>
    </div>
  );
}

export function OperatorChecklist({
  projectId,
  prodVersionId,
  versions,
}: {
  projectId: string;
  prodVersionId: string | null;
  versions: Version[];
}) {
  const hasAnyVersion = versions.length > 0;
  const hasReady = versions.some((v) => v.status === "ready");
  const promoteTarget = findPromoteCandidate(versions, prodVersionId);
  const defaultOpen = !hasAnyVersion || promoteTarget != null;

  return (
    <details
      data-testid="operator-checklist"
      className="group rounded-md border border-border bg-card"
      open={defaultOpen}
    >
      <summary className="flex cursor-pointer list-none items-center justify-between gap-2 px-4 py-3 text-label-14 font-medium [&::-webkit-details-marker]:hidden">
        <span>Operator checklist</span>
        <ChevronDown className="size-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-180" />
      </summary>
      <div className="space-y-3 border-t border-border px-4 py-4 text-sm">
        <p className="text-muted-foreground">
          Platform operator closed loop (Bearer admin token; no login UI).{" "}
          <a
            href={OPERATOR_JOURNEY_DOC_HREF}
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1 font-medium text-foreground hover:underline"
          >
            Full checklist in docs
            <ExternalLink className="size-3" />
          </a>
        </p>

        <ChecklistRow
          done
          title="Open project in Dashboard"
          success="Overview loaded for this project id."
          failureHint="Register via Projects → New project or first CLI deploy."
        />

        <ChecklistRow
          done={hasAnyVersion}
          emphasize={!hasAnyVersion}
          title="Deploy a version (CLI or CI)"
          success="At least one deployment record exists."
          failureHint={`No versions yet. From your Worker repo: cellp dev --project ${projectId}. The Dashboard does not upload Worker bundles.`}
        >
          {!hasAnyVersion ? (
            <p
              className="mt-2 font-medium text-foreground"
              data-testid="operator-checklist-deploy-first"
            >
              Deploy first — then refresh this page.
            </p>
          ) : null}
        </ChecklistRow>

        <ChecklistRow
          done={hasReady}
          title="Preview reaches ready"
          success="A version is ready; use its preview URL on the gateway."
          failureHint="Poll Deployments until status is ready, or check version error in API."
        />

        <ChecklistRow
          done={hasReady}
          title="Inspect bindings & routes"
          success="Use Inspect, Deployments, Storage, and Platform pages."
          failureHint="Need a ready version before Storage browsers are useful."
        >
          <div className="mt-2 flex flex-wrap gap-2">
            <Link
              to={inspectHref(projectId)}
              className="inline-flex h-7 items-center rounded-md border border-border px-2.5 text-xs font-medium hover:bg-muted"
            >
              Inspect
            </Link>
            <Link
              to={deploymentsHref(projectId)}
              className="inline-flex h-7 items-center rounded-md border border-border px-2.5 text-xs font-medium hover:bg-muted"
            >
              Deployments
            </Link>
            <Link
              to={storageHref(projectId)}
              className="inline-flex h-7 items-center rounded-md border border-border px-2.5 text-xs font-medium hover:bg-muted"
            >
              Storage
            </Link>
            <Link
              to={platformHref(projectId)}
              className="inline-flex h-7 items-center rounded-md border border-border px-2.5 text-xs font-medium hover:bg-muted"
            >
              Platform
            </Link>
          </div>
        </ChecklistRow>

        <ChecklistRow
          done={!!prodVersionId && !promoteTarget}
          emphasize={promoteTarget != null}
          title="Promote to production"
          success={
            prodVersionId
              ? `Production points at ${prodVersionId}.`
              : "Production pointer set after promote."
          }
          failureHint="Promote a ready preview when satisfied; promote switches prod pointer (does not merge prod writes after fork)."
        >
          {promoteTarget ? (
            <Link
              to={versionHref(projectId, promoteTarget.id)}
              data-testid="operator-checklist-promote-link"
              className="mt-2 inline-flex h-8 items-center rounded-md bg-primary px-3 text-xs font-medium text-primary-foreground hover:bg-primary/90"
            >
              Promote {promoteTarget.id} on version page
            </Link>
          ) : null}
        </ChecklistRow>
      </div>
    </details>
  );
}
