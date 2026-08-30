# Workflows

Workflows are long-running celld workflows. cellp **lists instances**. It does not pause, resume, or restart them.

## Branching

**Workflow instances do not branch.** A child version starts with no in-flight workflows. That is intentional: a preview should not continue production’s half-finished jobs.

The **script** that *defines* workflows comes from the child artifact.

## Worker

Use celld’s Workflows API as documented in celld. Declare `workflows` in wrangler.

## Operator API

```
GET …/workflows
GET …/workflows/{name}/instances
```

Dashboard is the same list. Copy ids; do not expect control buttons.

## When you need control

Operate via your Worker (idempotent steps, your own cancel flags) or wait for a future celld workflow CLI. cellp will not add a second orchestrator on top.
