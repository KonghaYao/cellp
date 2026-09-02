/**
 * cellp overlay — pi-worker Agent + R2 tools.
 *
 * Zen（OpenAI 兼容 Chat Completions）推荐配置：
 *   OPENAI_API_KEY=public
 *   OPENAI_BASE_URL=https://opencode.ai/zen/v1
 *   OPENAI_MODEL=big-pickle
 *
 * 实现：pi-ai `Agent` + `getModel`；若模型在 opencode 目录且为 openai-completions，
 * 用 OPENAI_* 覆盖 baseUrl/id；否则用 opencode 目录项 + 同一 API Key。
 * 对 opencode.ai 出站请求注入 Zen 公共 key 所需的 x-opencode-* 头（与 OpenCode CLI 一致）。
 */
import {
	Agent,
	getModel,
	createR2ReadTool,
	createR2WriteTool,
	createR2EditTool,
	createR2LsTool,
} from "pi-worker";

interface Env {
	OPENAI_API_KEY: string;
	OPENAI_BASE_URL?: string;
	OPENAI_MODEL?: string;
	FILES: R2Bucket;
}

function installZenFetchShim(apiKey: string) {
	const orig = globalThis.fetch.bind(globalThis);
	globalThis.fetch = (input: RequestInfo | URL, init?: RequestInit) => {
		const url = typeof input === "string" ? input : input instanceof URL ? input.href : input.url;
		if (!url.includes("opencode.ai")) return orig(input, init);
		const headers = new Headers(init?.headers);
		if (!headers.has("Authorization")) headers.set("Authorization", `Bearer ${apiKey}`);
		headers.set("x-opencode-client", "cli");
		headers.set("x-opencode-project", "global");
		headers.set("x-opencode-request", `msg_${crypto.randomUUID().slice(0, 8)}`);
		headers.set("x-opencode-session", `ses_${crypto.randomUUID().slice(0, 8)}`);
		return orig(input, { ...init, headers });
	};
}

function resolveModel(env: Env) {
	const modelId = env.OPENAI_MODEL?.trim() || "big-pickle";
	const baseUrl = env.OPENAI_BASE_URL?.trim()?.replace(/\/$/, "");

	const fromCatalog = getModel("opencode", modelId);
	// big-pickle 在 Zen 上走 Chat Completions（OpenAI 兼容），pi-ai 目录项是 anthropic-messages
	if (modelId === "big-pickle" || (fromCatalog && fromCatalog.api === "anthropic-messages")) {
		const tpl = getModel("opencode", "glm-5");
		if (!tpl) throw new Error("pi-ai catalog missing");
		return {
			...tpl,
			id: modelId,
			name: modelId,
			baseUrl: baseUrl || "https://opencode.ai/zen/v1",
			provider: "openai" as const,
			api: "openai-completions" as const,
		};
	}
	if (fromCatalog) {
		if (baseUrl && fromCatalog.api === "openai-completions") {
			return { ...fromCatalog, baseUrl, provider: "openai" as const };
		}
		return fromCatalog;
	}

	// 任意 Zen model id：OpenAI Chat Completions 形态
	const tpl = getModel("opencode", "glm-5");
	if (!tpl) throw new Error("pi-ai catalog missing");
	return {
		...tpl,
		id: modelId,
		name: modelId,
		baseUrl: baseUrl || "https://opencode.ai/zen/v1",
		provider: "openai" as const,
		api: "openai-completions" as const,
	};
}

function extractResponse(messages: { role: string; content?: unknown; error?: string }[]) {
	const assistant = [...messages].reverse().find((m) => m.role === "assistant");
	if (!assistant) return "";
	if (typeof assistant.content === "string") return assistant.content.trim();
	if (assistant.error) return `[error] ${assistant.error}`;
	if (!Array.isArray(assistant.content)) return "";
	return assistant.content
		.map((c: { type?: string; text?: string }) => (c?.type === "text" ? c.text ?? "" : ""))
		.join("")
		.trim();
}

export default {
	async fetch(request: Request, env: Env): Promise<Response> {
		if (request.method !== "POST") {
			return Response.json({
				usage: "POST { prompt: string }",
				config: { OPENAI_BASE_URL: env.OPENAI_BASE_URL, OPENAI_MODEL: env.OPENAI_MODEL },
			});
		}

		let body: { prompt?: string };
		try {
			body = (await request.json()) as { prompt?: string };
		} catch {
			return Response.json({ error: "Invalid JSON body" }, { status: 400 });
		}
		const { prompt } = body;
		if (!prompt?.trim()) return Response.json({ error: "Missing 'prompt'" }, { status: 400 });

		const apiKey = env.OPENAI_API_KEY?.trim() || "public";
		installZenFetchShim(apiKey);

		try {
			const model = resolveModel(env);
			const agent = new Agent({
				initialState: {
					systemPrompt:
						"You are a helpful coding assistant. Use file tools when asked to manage files.",
					model,
					thinkingLevel: "off",
					tools: [
						createR2WriteTool(env.FILES),
						createR2ReadTool(env.FILES),
						createR2LsTool(env.FILES),
						createR2EditTool(env.FILES),
					],
				},
				getApiKey: async () => apiKey,
			});

			await agent.prompt(prompt);
			const response = extractResponse(agent.state.messages);
			const toolCalls = agent.state.messages.reduce((n, m) => {
				if (m.role !== "assistant" || !Array.isArray(m.content)) return n;
				return n + m.content.filter((c: { type?: string }) => c.type === "tool_use").length;
			}, 0);

			return Response.json({
				response,
				meta: {
					turns: agent.state.messages.length,
					toolCalls,
					model: model.id,
					baseUrl: model.baseUrl,
				},
			});
		} catch (e) {
			const message = e instanceof Error ? e.message : String(e);
			return Response.json({ error: message, response: "" }, { status: 500 });
		}
	},
};
