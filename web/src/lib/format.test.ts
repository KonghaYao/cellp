import { describe, expect, it } from "vitest";
import {
  gatewayBrowseUrl,
  previewBrowseUrl,
  prodBrowseUrl,
  resolveProdUrl,
} from "./format";

describe("format AD-12 browse URLs", () => {
  it("previewBrowseUrl uses __cellp_host in dev", () => {
    const url = previewBrowseUrl(
      "demo-app",
      "v1",
      "http://v1.demo-app.ingress.local/",
    );
    expect(url).toContain("/__gateway");
    expect(url).toContain("__cellp_host=v1.demo-app.ingress.local");
  });

  it("prodBrowseUrl uses project host", () => {
    const url = prodBrowseUrl("demo-app", "http://demo-app.ingress.local/");
    expect(url).toContain("__cellp_host=demo-app.ingress.local");
  });

  it("resolveProdUrl prefers host prod_url from API", () => {
    const url = resolveProdUrl(
      "demo-app",
      "http://demo-app.ingress.local/",
      null,
    );
    expect(url).toContain("demo-app.ingress.local");
  });

  it("deriveProdUrl does not emit /prod/ path suffix", () => {
    const url = resolveProdUrl("demo-app", null, "http://v1.demo-app.ingress.local/");
    expect(url).not.toContain("/prod/");
    expect(url).not.toContain("/demo-app/");
  });
});
