import { WorkflowEntrypoint } from "cloudflare:workers";

const SCHEMA = `
CREATE TABLE IF NOT EXISTS entries (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL,
  message TEXT NOT NULL,
  at INTEGER NOT NULL
);
`;

export class ReportBuilder extends WorkflowEntrypoint {
  async run(event, step) {
    return await step.do("build report", async () => {
      const response = await fetch(event.payload.url);
      if (!response.ok) throw new Error(`source answered ${response.status}`);
      const text = await response.text();
      return { bytes: text.length, lines: text.split("\n").length };
    });
  }
}

export default {
  async fetch(request, env) {
    await env.DB.exec(SCHEMA);
    const url = new URL(request.url);

    if (url.pathname === "/enqueue" && request.method === "POST") {
      let body;
      try {
        body = await request.json();
      } catch {
        body = { ping: true, at: Date.now() };
      }
      await env.TASKS.send(body);
      return Response.json({ ok: true }, { status: 202 });
    }

    if (url.pathname === "/create") {
      const instance = await env.REPORTS.create({
        params: { url: url.searchParams.get("url") ?? "https://example.com" },
      });
      return Response.json({ id: instance.id });
    }

    if (url.pathname === "/count") {
      const count = await env.DB.prepare("SELECT count(*) AS n FROM entries").first("n");
      return Response.json({ count });
    }

    return Response.json({
      service: "bindings-demo",
      routes: ["/count", "POST /enqueue", "/create?url=URL"],
    });
  },

  async scheduled(controller, env, ctx) {
    console.log("bindings-demo cron", controller.scheduledTime);
  },
};
