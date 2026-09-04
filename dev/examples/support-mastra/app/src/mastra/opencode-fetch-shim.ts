/**
 * OpenCode Zen public key: same headers as support-pi-worker hello-agent.
 * Import before Mastra/AI SDK outbound calls.
 */
export function installOpenCodeFetchShim(): void {
  const orig = globalThis.fetch.bind(globalThis);
  globalThis.fetch = (input: RequestInfo | URL, init?: RequestInit) => {
    const url =
      typeof input === 'string'
        ? input
        : input instanceof URL
          ? input.href
          : input.url;
    if (!url.includes('opencode.ai')) {
      return orig(input, init);
    }
    const headers = new Headers(
      init?.headers ?? (input instanceof Request ? input.headers : undefined),
    );
    headers.set('x-opencode-client', 'cli');
    headers.set('x-opencode-project', 'global');
    headers.set(
      'x-opencode-request',
      `msg_${crypto.randomUUID().slice(0, 8)}`,
    );
    headers.set(
      'x-opencode-session',
      `ses_${crypto.randomUUID().slice(0, 8)}`,
    );
    return orig(input, { ...init, headers });
  };
}
