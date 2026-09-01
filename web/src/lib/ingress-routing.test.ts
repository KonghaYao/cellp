import { describe, expect, it } from "vitest";
import { ingressHostToDashboardPath } from "./ingress-routing";

describe("ingressHostToDashboardPath", () => {
  it("redirects API prod host to project page", () => {
    expect(ingressHostToDashboardPath("demo-app.ingress.local")).toBe(
      "/projects/demo-app",
    );
  });

  it("redirects API preview host to version page", () => {
    expect(ingressHostToDashboardPath("v1.demo-app.ingress.local")).toBe(
      "/projects/demo-app/versions/v1",
    );
  });

  it("keeps commerce-store on gateway", () => {
    expect(
      ingressHostToDashboardPath("commerce-store.ingress.local"),
    ).toBeNull();
  });
});
