// Producer-only queue Worker (no queue() consumer).
// celld constraint: a consumer script cannot also export fetch().
export default {
  async fetch(request, env) {
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
    return new Response("queue-producer: POST /enqueue\n", { status: 200 });
  },
};
