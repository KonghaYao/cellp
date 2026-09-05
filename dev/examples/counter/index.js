const topLevelGreeting = process.env.GREETING ?? null;
const topLevelCounter = process.env.COUNTER ?? null;

export class Counter {
  constructor(state, env) {
    this.state = state;
    this.env = env;
  }
  async fetch(request) {
    let n = (await this.state.storage.get("n")) ?? 0;
    n++;
    await this.state.storage.put("n", n);
    return new Response(
      JSON.stringify({
        n,
        version: this.env.VERSION_ID ?? "unknown",
        project: this.env.PROJECT_ID ?? "unknown",
        greeting: this.env.GREETING ?? null,
        topLevelGreeting,
        topLevelCounter,
        url: request.url,
      }),
      { status: 200, headers: { "content-type": "application/json" } },
    );
  }
}

export default {
  async fetch(request, env) {
    const id = env.COUNTER.idFromName(env.VERSION_ID ?? "default");
    return env.COUNTER.get(id).fetch(request);
  },
};
