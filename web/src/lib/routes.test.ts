import { describe, expect, it } from "vitest";
import {
  bindingsHref,
  storageBrowserHref,
  storageKvHref,
  storageSurfaceFromPathname,
  versionHref,
  versionHrefForStoragePathname,
} from "@/lib/routes";

describe("routes", () => {
  it("builds version and storage paths with encoded segments", () => {
    expect(versionHref("demo-app", "v1")).toBe("/projects/demo-app/versions/v1");
    expect(storageBrowserHref("demo-app", "v1")).toBe(
      "/projects/demo-app/storage/v1/browser",
    );
    expect(versionHref("a/b", "v:1")).toBe(
      "/projects/a%2Fb/versions/v%3A1",
    );
    expect(storageKvHref("demo-app", "v2")).toBe(
      "/projects/demo-app/storage/v2/kv",
    );
  });

  it("bindingsHref opens storage browser when version is set", () => {
    expect(bindingsHref("demo-app")).toBe("/projects/demo-app/storage");
    expect(bindingsHref("demo-app", "v-preview")).toBe(
      "/projects/demo-app/storage/v-preview/browser",
    );
  });

  it("detects storage surface from pathname without false positives", () => {
    expect(
      storageSurfaceFromPathname("/projects/p/storage/v1/browser"),
    ).toBe("browser");
    expect(
      storageSurfaceFromPathname("/projects/p/storage/my-kv-branch/browser"),
    ).toBe("browser");
    expect(storageSurfaceFromPathname("/projects/p/storage/v1/kv")).toBe("kv");
    expect(storageSurfaceFromPathname("/projects/p/storage/v1/queues")).toBe(
      "queues",
    );
    expect(
      storageSurfaceFromPathname("/projects/p/storage/v1/workflows"),
    ).toBe("workflows");
  });

  it("versionHrefForStoragePathname keeps the current binding tab", () => {
    expect(
      versionHrefForStoragePathname("demo-app", "v2", "/projects/demo-app/storage/v1/kv"),
    ).toBe("/projects/demo-app/storage/v2/kv");
    expect(
      versionHrefForStoragePathname(
        "demo-app",
        "v2",
        "/projects/demo-app/storage/v1-with-kv/browser",
      ),
    ).toBe("/projects/demo-app/storage/v2/browser");
  });
});
