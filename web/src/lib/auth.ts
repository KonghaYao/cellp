/** Browser-persisted admin token (Docker / production login). */
export const ADMIN_TOKEN_STORAGE_KEY = "cellp_admin_token";

export function getStoredAdminToken(): string | null {
  try {
    return localStorage.getItem(ADMIN_TOKEN_STORAGE_KEY);
  } catch {
    return null;
  }
}

export function setStoredAdminToken(token: string): void {
  localStorage.setItem(ADMIN_TOKEN_STORAGE_KEY, token.trim());
}

export function clearStoredAdminToken(): void {
  localStorage.removeItem(ADMIN_TOKEN_STORAGE_KEY);
}

/** localStorage first; VITE_CELLP_ADMIN_TOKEN is dev-only fallback. */
export function resolveAdminToken(): string {
  const stored = getStoredAdminToken()?.trim();
  if (stored) return stored;
  return import.meta.env.VITE_CELLP_ADMIN_TOKEN?.trim() ?? "";
}

export function hasAdminToken(): boolean {
  return resolveAdminToken().length > 0;
}
