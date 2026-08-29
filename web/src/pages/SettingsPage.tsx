import { useParams } from "react-router-dom";

export function SettingsPage() {
  const { id = "" } = useParams<{ id: string }>();

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <h1 className="text-heading-24 font-semibold tracking-tight">Settings</h1>
        <p className="mt-1 text-copy-14 text-muted-foreground">
          Project configuration for <span className="font-mono">{id}</span>
        </p>
      </div>
      <div className="rounded-md border border-dashed border-border px-6 py-12 text-center text-sm text-muted-foreground">
        General settings coming soon — environment variables, domains, and git
        integration will appear here.
      </div>
    </div>
  );
}
