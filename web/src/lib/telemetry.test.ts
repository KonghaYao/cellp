import { describe, expect, it } from "vitest";
import { storageTelemetryHref, storageSurfaceFromPathname } from "@/lib/routes";

describe("telemetry routes", () => {
  it("builds telemetry href", () => {
    expect(storageTelemetryHref("demo", "v1")).toBe("/projects/demo/storage/v1/telemetry");
  });

  it("detects telemetry surface", () => {
    expect(storageSurfaceFromPathname("/projects/demo/storage/v1/telemetry")).toBe("telemetry");
  });
});
