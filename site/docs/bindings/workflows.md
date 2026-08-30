# Workflows

Long-running jobs. You **write a class**, declare it in wrangler, then `create()` from `fetch`.

## 1. Declare it

```jsonc
"workflows": [
  {
    "binding": "REPORTS",
    "name": "order-report",
    "class_name": "OrderReport"
  }
]
```

## 2. Implement and start

See the copy-paste in [Handlers](/build/handlers). `class_name` must match the exported class. `this.env` inside the workflow is the same binding set as the Worker.

## 3. Data / instances

**Instances do not branch.** A preview version starts with no in-flight jobs. That is so a PR cannot continue production’s half-finished work.

Dashboard → Storage → Workflows lists instances (**read-only**). No pause/resume/restart in cellp.

[Platform data](/build/data)

