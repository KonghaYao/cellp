import { CLIENT_HTML } from "./client.js";

export { FxSession } from "./session.js";

function authorized(url, expected) {
	const given = url.searchParams.get("key") ?? "";
	if (!expected || given.length !== expected.length) return false;
	let diff = 0;
	for (let i = 0; i < expected.length; i++) diff |= given.charCodeAt(i) ^ expected.charCodeAt(i);
	return diff === 0;
}

export default {
	async fetch(request, env) {
		const url = new URL(request.url);

		if (!authorized(url, env.ACCESS_KEY)) {
			return new Response("unauthorized — append ?key=<ACCESS_KEY>", { status: 401 });
		}

		// cellp：无 WebSocket 时用 HTTP 多轮（见 dev/examples/support-fx-on-workers/README.md）
		if (url.pathname === "/api/health" && request.method === "GET") {
			return Response.json({
				ok: true,
				modes: ["http-prompt", "websocket-tui"],
				note: "cellp local stack: prefer POST /api/prompt; WebSocket /session may 502",
			});
		}

		if (url.pathname === "/api/prompt" && request.method === "POST") {
			const name = url.searchParams.get("id") || "cellp-http";
			const stub = env.FX_SESSION.get(env.FX_SESSION.idFromName(name));
			const headers = new Headers(request.headers);
			headers.set("X-Cellp-Mode", "http-prompt");
			return stub.fetch(
				new Request("https://fx-session.internal/http-prompt", {
					method: "POST",
					headers,
					body: request.body,
				}),
			);
		}

		if (url.pathname !== "/" && url.pathname !== "/session") {
			return new Response("not found", { status: 404 });
		}

		if (url.pathname === "/session") {
			const name = url.searchParams.get("id") || crypto.randomUUID();
			const stub = env.FX_SESSION.get(env.FX_SESSION.idFromName(name));
			return stub.fetch(request);
		}

		return new Response(CLIENT_HTML, {
			headers: { "content-type": "text/html; charset=utf-8" },
		});
	},
};
