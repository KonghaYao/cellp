#!/usr/bin/env node
/**
 * Inject HTTP prompt mode into fx-on-workers FxSession (cellp overlay).
 * Usage: patch-session.mjs <path/to/session.js>
 */
import fs from "node:fs";

const file = process.argv[2];
if (!file) {
	console.error("usage: patch-session.mjs <session.js>");
	process.exit(1);
}

let s = fs.readFileSync(file, "utf8");

if (s.includes("X-Cellp-Mode")) {
	console.log("patch-session: already patched");
	process.exit(0);
}

const fetchOld = `  async fetch(request) {
    if (request.headers.get("Upgrade") !== "websocket") {
      return new Response("expected websocket", { status: 426 });
    }`;

const fetchNew = `  #httpCapture = null;

  async fetch(request) {
    if (request.headers.get("X-Cellp-Mode") === "http-prompt") {
      return this.#handleHttpPrompt(request);
    }

    if (request.headers.get("Upgrade") !== "websocket") {
      return new Response("expected websocket", { status: 426 });
    }`;

if (!s.includes(fetchOld)) {
	console.error("patch-session: fetch() anchor not found — upstream session.js changed?");
	process.exit(1);
}
s = s.replace(fetchOld, fetchNew);

const insertBefore = `  #teardown() {`;
const httpMethods = `  async #handleHttpPrompt(request) {
    if (request.method !== "POST") {
      return new Response("POST JSON { prompt: string }", { status: 405 });
    }
    let body;
    try {
      body = await request.json();
    } catch {
      return Response.json({ error: "invalid json" }, { status: 400 });
    }
    const prompt = String(body?.prompt ?? "").trim();
    if (!prompt) {
      return Response.json({ error: "missing prompt" }, { status: 400 });
    }
    const waitMs = Math.min(Number(body?.waitMs) || 120_000, 180_000);

    if (this.#runtime || this.#ws) this.#teardown();

    const events = [];
    const byteChunks = [];
    this.#httpCapture = { events, byteChunks };
    this.#ws = {
      send: (data) => {
        if (typeof data === "string") {
          try {
            events.push(JSON.parse(data));
          } catch {
            events.push({ type: "text", data });
          }
        } else {
          byteChunks.push(data instanceof Uint8Array ? data : new TextEncoder().encode(String(data)));
        }
      },
    };

    this.#boot = this.#bootTerminal().catch((error) => {
      this.#event({ type: "error", message: describe(error), fatal: true });
      throw error;
    });

    try {
      await this.#boot;
      await this.#onMessage(JSON.stringify({ type: "prompt", text: prompt }));

      const deadline = Date.now() + waitMs;
      let lastCount = 0;
      while (Date.now() < deadline) {
        await new Promise((r) => setTimeout(r, 800));
        const commands = events.filter((e) => e.type === "command");
        if (commands.length > 0 && events.length === lastCount) break;
        lastCount = events.length;
        if (events.some((e) => e.fatal)) break;
      }

      const commands = events.filter((e) => e.type === "command");
      const output = byteChunks.length
        ? new TextDecoder("utf-8", { fatal: false }).decode(
            byteChunks.reduce((acc, c) => {
              const out = new Uint8Array(acc.length + c.length);
              out.set(acc, 0);
              out.set(c, acc.length);
              return out;
            }, new Uint8Array()),
          )
        : "";

      return Response.json({
        ok: !events.some((e) => e.type === "error" && e.fatal),
        prompt,
        commands,
        events,
        outputTail: output.slice(-12_000),
        meta: { mode: "http-prompt", commandCount: commands.length },
      });
    } catch (error) {
      return Response.json({ error: describe(error), events }, { status: 500 });
    } finally {
      this.#teardown();
      this.#httpCapture = null;
    }
  }

`;

if (!s.includes(insertBefore)) {
	console.error("patch-session: #teardown anchor not found");
	process.exit(1);
}
s = s.replace(insertBefore, httpMethods + insertBefore);

fs.writeFileSync(file, s);
console.log("patch-session: HTTP prompt mode injected");
