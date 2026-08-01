const HTML_TAG_REGEX = /<[^>]*>/g;
const CONTROL_CHARS_REGEX = /[\u0000-\u001F\u007F]/g;

/** Remove HTML tags and control characters. */
export function stripHtml(value: string): string {
  return value.replace(HTML_TAG_REGEX, "").replace(CONTROL_CHARS_REGEX, "");
}

/** Alphanumeric, underscore, hyphen; trimmed; max 64 chars by default. */
export function sanitizeUsername(value: string, maxLen = 64): string {
  return stripHtml(value)
    .replace(/[^a-zA-Z0-9_-]/g, "")
    .trim()
    .slice(0, maxLen);
}

/** Strip HTML/control chars; max 128 chars by default. */
export function sanitizePassword(value: string, maxLen = 128): string {
  return stripHtml(value).slice(0, maxLen);
}

export function maxLength(value: string, max: number): string {
  return value.slice(0, max);
}

export function validateMaxLength(value: string, max: number): boolean {
  return value.length <= max;
}

export function validateUsername(value: string): boolean {
  if (value.length === 0 || value.length > 64) return false;
  return /^[a-zA-Z0-9_-]+$/.test(value);
}

export function validatePassword(value: string): boolean {
  return value.length > 0 && value.length <= 128;
}

/** Bare hostname: strip protocol, paths, and api. prefix. */
export function sanitizeDomain(value: string, maxLen = 253): string {
  let cleaned = stripHtml(value).trim().toLowerCase();

  const schemeIdx = cleaned.indexOf("://");
  if (schemeIdx >= 0) {
    cleaned = cleaned.slice(schemeIdx + 3);
  }

  const pathIdx = cleaned.search(/[/?#]/);
  if (pathIdx >= 0) {
    cleaned = cleaned.slice(0, pathIdx);
  }

  if (cleaned.startsWith("api.")) {
    cleaned = cleaned.slice(4);
  }

  return cleaned.slice(0, maxLen);
}

/** Trim controller URL; leave scheme/port intact for optional override field. */
export function sanitizeControllerURL(value: string, maxLen = 512): string {
  return stripHtml(value).trim().replace(/\/+$/, "").slice(0, maxLen);
}
