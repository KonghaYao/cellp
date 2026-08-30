# Cron

Cron triggers are declared in wrangler (`triggers.crons`) and **fired by celld**, not by cellp’s orchestrator.

## Branching

Cron **does not branch**. Each version’s script carries its own trigger list. An archived version does not run cron (the process is gone). Production cron is whatever production version declares.

## Visibility

`GET …/bindings` includes `crons`. Dashboard shows the expressions. There is no “run now” button in cellp.

## celld behavior (short)

- One handler per occurrence across the fleet
- After downtime, the most recent missed occurrence may run once
- Some cron syntax Cloudflare allows is rejected (e.g. descending ranges)

Details: [celld cron notes](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md).

## Timezones / ops

Your machines’ clock is the clock. There is no Cloudflare cron dashboard with per-trigger history. Ship logs to your aggregator if you need an audit trail.
