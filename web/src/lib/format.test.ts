import { describe, expect, it, vi } from "vitest";
import {
  gatewayBrowseUrl,
  ingressDisplayUrl,
  previewBrowseUrl,
  previewOpenUrl,
  productionOpenUrl,
  resolveProdUrl,
} from "./format";

describe("format AD-12 browse URLs", () => {
  it("previewOpenUrl for demo-app opens Dashboard version page", () => {
    const url = previewOpenUrl(
      "demo-app",
      "v1",
      "http://v1.demo-app.ingress.local/",
    );
    expect(url).toBe("/projects/demo-app/versions/v1");
    expect(url).not.toContain("__gateway");
  });

  it("productionOpenUrl for demo-app opens Dashboard project page", () => {
    const url = productionOpenUrl(
      "demo-app",
      "http://demo-app.ingress.local/",
    );
    expect(url).toBe("/projects/demo-app");
  });

  it("previewBrowseUrl for commerce-store uses __cellp_host in dev", () => {
    const url = previewBrowseUrl(
      "commerce-store",
      "v1",
      "http://v1.commerce-store.ingress.local/",
    );
    expect(url).toContain("/__gateway");
    expect(url).toContain("__cellp_host=v1.commerce-store.ingress.local");
  });

  it("resolveProdUrl for commerce-store uses gateway proxy", () => {
    const url = resolveProdUrl(
      "commerce-store",
      "http://commerce-store.ingress.local/",
      null,
    );
    expect(url).toContain("__cellp_host=commerce-store.ingress.local");
  });

  it("gatewayBrowseUrl still builds proxy for API probes", () => {
    const url = gatewayBrowseUrl("demo-app.ingress.local", "/count");
    expect(url).toContain("__cellp_host=demo-app.ingress.local");
    expect(url).toContain("/count");
  });

  it("ingressDisplayUrl appends gateway port in dev", () => {
    const url = ingressDisplayUrl(
      "demo-app",
      "v1",
      "http://v1.demo-app.ingress.local/",
    );
    expect(url).toContain(":8787");
  });
});

describe("ingress magic DNS", () => {
  it("ingressUsesMagicDns for nip.io base", async () => {
    vi.stubEnv("VITE_CELLP_INGRESS_BASE_DOMAIN", "192-168-1-10.nip.io");
    vi.resetModules();
    const mod = await import("./format");
    expect(mod.ingressBaseDomain()).toBe("192-168-1-10.nip.io");
    expect(mod.ingressUsesMagicDns()).toBe(true);
    vi.unstubAllEnvs();
    vi.resetModules();
  });
});
