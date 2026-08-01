const TOKEN_KEY = "trinityproxy_auth_token";

function isSecureContext(): boolean {
  return typeof window !== "undefined" && window.isSecureContext;
}

export function getStoredToken(): string | null {
  try {
    return localStorage.getItem(TOKEN_KEY);
  } catch {
    return null;
  }
}

export function setStoredToken(token: string): void {
  if (!isSecureContext()) return;
  try {
    localStorage.setItem(TOKEN_KEY, token);
  } catch {
    // Cookie-only auth when storage is unavailable.
  }
}

export function clearStoredToken(): void {
  try {
    localStorage.removeItem(TOKEN_KEY);
  } catch {
    // ignore
  }
}

export function prefersDarkMode(): boolean {
  return window.matchMedia("(prefers-color-scheme: dark)").matches;
}

export function applyTheme(): void {
  const root = document.documentElement;
  root.classList.toggle("dark", prefersDarkMode());
}
