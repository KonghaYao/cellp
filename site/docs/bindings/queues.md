# Queues

Queues are celld queues: producers from the Worker, consumers on the same script. cellp wraps the operator CLI.

## Branching

Child versions **branch** the queue from the parent. Workflow-like in-flight work is still **not** a Workflow branch (see [Workflows](/bindings/workflows)).

## Worker

Use the Cloudflare-shaped `env.QUEUE.send(...)` / consumer handler as supported by celld. Declare `queues` in wrangler. Names inherit on branch.

## Operator API

```
GET  …/queues
GET  …/queues/{name}
POST …/queues/{name}/peek
POST …/queues/{name}/pause
POST …/queues/{name}/resume
POST …/queues/{name}/redrive
POST …/queues/{name}/purge
```

Dashboard → Storage → Queues. Purge is destructive; preview branches make it safer to experiment than on production.

## Gaps

celld Queues are **partial** vs Cloudflare. Confirm batching, retries, and DLQ behavior against celld compat before you bet a billing pipeline on it.

cellp does **not** run a separate Kafka/NATS. The queue is inside the version’s runtime/storage.
