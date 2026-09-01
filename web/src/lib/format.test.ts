import { describe, expect, it } from "vitest";
import {
  gatewayBrowseUrl,
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
});
