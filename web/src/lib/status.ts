/** All version statuses per DESIGN §2.5 */
export const VERSION_STATUSES = [
  "pending",
  "fetching",
  "branching",
  "preparing",
  "deploying",
  "ready",
  "draining",
  "destroyed",
  "failed",
] as const;

export type VersionStatus = (typeof VERSION_STATUSES)[number];

export const IN_PROGRESS_STATUSES: VersionStatus[] = [
  "pending",
  "fetching",
  "branching",
  "preparing",
  "deploying",
];

export function isInProgressStatus(status: string): boolean {
  return (IN_PROGRESS_STATUSES as readonly string[]).includes(status);
}

export function statusLabel(status: string): string {
  return status.charAt(0).toUpperCase() + status.slice(1);
}

export const STATUS_DOT: Record<string, string> = {
  pending: "bg-amber-500",
  fetching: "bg-sky-500",
  branching: "bg-sky-500",
  preparing: "bg-sky-500",
  deploying: "bg-sky-500 animate-pulse",
  ready: "bg-emerald-500",
  draining: "bg-orange-500",
  destroyed: "bg-zinc-400",
  failed: "bg-red-500",
};

export const STATUS_TEXT: Record<string, string> = {
  pending: "text-amber-700",
  fetching: "text-sky-700",
  branching: "text-sky-700",
  preparing: "text-sky-700",
  deploying: "text-sky-700",
  ready: "text-emerald-700",
  draining: "text-orange-700",
  destroyed: "text-zinc-500",
  failed: "text-red-600",
};

/** Timeline order for version detail view */
export const STATUS_TIMELINE: VersionStatus[] = [
  "pending",
  "fetching",
  "branching",
  "preparing",
  "deploying",
  "ready",
];

export function timelineIndex(status: string): number {
  const idx = STATUS_TIMELINE.indexOf(status as VersionStatus);
  return idx >= 0 ? idx : -1;
}
