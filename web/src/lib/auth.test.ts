import { afterEach, describe, expect, it, vi } from "vitest";
import {
  ADMIN_TOKEN_STORAGE_KEY,
  clearStoredAdminToken,
  getStoredAdminToken,
  hasAdminToken,
  resolveAdminToken,
  setStoredAdminToken,
} from "@/lib/auth";

describe("auth token storage", () => {
  afterEach(() => {
    clearStoredAdminToken();
    vi.unstubAllEnvs();
  });

  it("prefers localStorage over dev env fallback", () => {
    vi.stubEnv("VITE_CELLP_ADMIN_TOKEN", "env-token");
    setStoredAdminToken("stored-token");
    expect(resolveAdminToken()).toBe("stored-token");
    expect(hasAdminToken()).toBe(true);
  });

  it("falls back to VITE_CELLP_ADMIN_TOKEN when localStorage empty", () => {
    vi.stubEnv("VITE_CELLP_ADMIN_TOKEN", "env-token");
    expect(getStoredAdminToken()).toBeNull();
    expect(resolveAdminToken()).toBe("env-token");
  });

  it("clearStoredAdminToken removes persisted value", () => {
    setStoredAdminToken("x");
    expect(localStorage.getItem(ADMIN_TOKEN_STORAGE_KEY)).toBe("x");
    clearStoredAdminToken();
    expect(getStoredAdminToken()).toBeNull();
  });
});
