interface DeploymentsFilterBarProps {
  branches: string[];
  branchFilter: string;
  statusFilter: string;
  hideDestroyed: boolean;
  onBranchChange: (value: string) => void;
  onStatusChange: (value: string) => void;
  onHideDestroyedChange: (value: boolean) => void;
}

const STATUSES = ["ready", "pending", "deploying", "failed", "destroyed"];

export function DeploymentsFilterBar({
  branches,
  branchFilter,
  statusFilter,
  hideDestroyed,
  onBranchChange,
  onStatusChange,
  onHideDestroyedChange,
}: DeploymentsFilterBarProps) {
  return (
    <div className="flex flex-wrap items-center gap-2 rounded-md border border-border bg-card p-2">
      <label className="flex items-center gap-2 text-label-13 text-muted-foreground">
        <span className="hidden sm:inline">Branch</span>
        <select
          value={branchFilter}
          onChange={(e) => onBranchChange(e.target.value)}
          className="h-8 rounded-md border border-border bg-background px-2 text-sm"
        >
          <option value="">All branches</option>
          {branches.map((branch) => (
            <option key={branch} value={branch}>
              {branch}
            </option>
          ))}
        </select>
      </label>

      <label className="flex items-center gap-2 text-label-13 text-muted-foreground">
        <span className="hidden sm:inline">Status</span>
        <select
          value={statusFilter}
          onChange={(e) => onStatusChange(e.target.value)}
          className="h-8 rounded-md border border-border bg-background px-2 text-sm"
        >
          <option value="">All statuses</option>
          {STATUSES.map((status) => (
            <option key={status} value={status}>
              {status}
            </option>
          ))}
        </select>
      </label>

      <label className="ml-auto flex cursor-pointer items-center gap-2 text-label-13 text-muted-foreground">
        <input
          type="checkbox"
          checked={hideDestroyed}
          onChange={(e) => onHideDestroyedChange(e.target.checked)}
          className="size-3.5 rounded border-border"
        />
        Hide destroyed
      </label>
    </div>
  );
}
