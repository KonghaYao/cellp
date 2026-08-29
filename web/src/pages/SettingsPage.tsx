import { Link, useParams } from "react-router-dom";
import {
  ArrowLeft,
  BookOpen,
  Globe,
  GitBranch,
  KeyRound,
  Settings,
} from "lucide-react";
import { projectOverviewHref } from "@/lib/routes";

const FUTURE_SECTIONS = [
  {
    icon: KeyRound,
    title: "Environment variables",
    description: "Manage secrets and config per deployment.",
  },
  {
    icon: Globe,
    title: "Domains",
    description: "Custom domains and routing rules.",
  },
  {
    icon: GitBranch,
    title: "Git integration",
    description: "Connect remotes and deploy hooks.",
  },
] as const;

export function SettingsPage() {
  const { id = "" } = useParams<{ id: string }>();

  return (
    <div className="mx-auto max-w-2xl space-y-6">
      <div>
        <Link
          to={projectOverviewHref(id)}
          className="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground transition-colors hover:text-foreground"
        >
          <ArrowLeft className="size-3.5" />
          Back to overview
        </Link>
        <h1 className="text-heading-24 font-semibold tracking-tight">Settings</h1>
        <p className="mt-1 text-copy-14 text-muted-foreground">
          Project configuration for <span className="font-mono">{id}</span>
        </p>
      </div>

      <div className="rounded-md border border-border bg-card p-6">
        <div className="flex items-start gap-3">
          <div className="flex size-10 shrink-0 items-center justify-center rounded-md border border-border bg-muted">
            <Settings className="size-5 text-muted-foreground" />
          </div>
          <div>
            <h2 className="text-label-14 font-medium">Coming soon</h2>
            <p className="mt-1 text-sm text-muted-foreground">
              Project settings are not available yet. These sections are planned
              for a future release:
            </p>
          </div>
        </div>

        <ul className="mt-6 space-y-3">
          {FUTURE_SECTIONS.map(({ icon: Icon, title, description }) => (
            <li
              key={title}
              className="flex items-start gap-3 rounded-md border border-dashed border-border px-4 py-3"
            >
              <Icon className="mt-0.5 size-4 shrink-0 text-muted-foreground" />
              <div>
                <p className="text-sm font-medium">{title}</p>
                <p className="text-sm text-muted-foreground">{description}</p>
              </div>
            </li>
          ))}
        </ul>
      </div>

      <p className="text-sm text-muted-foreground">
        Need help today?{" "}
        <a
          href="https://github.com/cursor/cellp#readme"
          target="_blank"
          rel="noreferrer"
          className="inline-flex items-center gap-1 font-medium text-foreground hover:underline"
        >
          <BookOpen className="size-3.5" />
          Read the cellp docs
        </a>
      </p>
    </div>
  );
}
