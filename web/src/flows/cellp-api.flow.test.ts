/**
 * API 客户端心流：createProject / promoteVersion 请求形状（无浏览器）
 */
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { createProject, promoteVersion } from "@/lib/cellp-api";

describe("心流：cellp-api 请求契约", () => {
  const fetchMock = vi.fn();

  beforeEach(() => {
    vi.stubEnv("VITE_CELLP_API_URL", "http://cellpd.test");
    vi.stubEnv("VITE_CELLP_ADMIN_TOKEN", "test-token");
    vi.stubGlobal("fetch", fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("createProject POST /v1/projects", async () => {
    fetchMock.mockResolvedValue({
      ok: true,
      status: 201,
      text: async () =>
        JSON.stringify({
          id: "my-shop",
          prod_version_id: null,
          created_at: "2026-01-01T00:00:00.000Z",
        }),
    });

    const project = await createProject({ id: "my-shop", git_remote: "https://git.example/repo" });

    expect(project.id).toBe("my-shop");
    expect(fetchMock).toHaveBeenCalledOnce();
    const [url, init] = fetchMock.mock.calls[0] as [string, RequestInit];
    expect(url).toBe("http://cellpd.test/v1/projects");
    expect(init.method).toBe("POST");
    const headers = init.headers as Headers;
    expect(headers.get("Authorization")).toBe("Bearer test-token");
    expect(JSON.parse(String(init.body))).toEqual({
      id: "my-shop",
      git_remote: "https://git.example/repo",
    });
  });

  it("promoteVersion POST …/promote", async () => {
    fetchMock
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        text: async () =>
          JSON.stringify({
            status: "promoted",
            prod_version_id: "v2",
            prod_url: "http://gateway/demo-app/",
          }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        text: async () =>
          JSON.stringify({
            id: "demo-app",
            prod_version_id: "v2",
            created_at: "2026-01-01T00:00:00.000Z",
          }),
      });

    await promoteVersion("demo-app", "v2");

    const promoteCall = fetchMock.mock.calls.find((c) =>
      String(c[0]).includes("/promote"),
    );
    expect(promoteCall).toBeDefined();
    expect(promoteCall![0]).toBe(
      "http://cellpd.test/v1/projects/demo-app/versions/v2/promote",
    );
    expect((promoteCall![1] as RequestInit).method).toBe("POST");
  });
});
