/**
 * Returns the URL with credentials replaced by bullet characters.
 * Works by replacing the raw credential substring to avoid percent-encoding issues.
 */
export function maskUrl(url: string): string {
  try {
    const u = new URL(url);
    if (!u.username) return url;
    const proto = url.slice(0, url.indexOf("://") + 3);
    const atIdx = url.indexOf("@");
    if (atIdx === -1) return url;
    const rest = url.slice(atIdx + 1);
    const masked = u.password ? "••••:••••" : "••••";
    return `${proto}${masked}@${rest}`;
  } catch {
    return url;
  }
}

/**
 * Splits a URL with embedded credentials into its parts.
 */
export function parseUrl(url: string): { baseUrl: string; username: string; password: string } {
  try {
    const u = new URL(url);
    const username = decodeURIComponent(u.username);
    const password = decodeURIComponent(u.password);
    u.username = "";
    u.password = "";
    return { baseUrl: u.toString(), username, password };
  } catch {
    return { baseUrl: url, username: "", password: "" };
  }
}

/**
 * Embeds credentials into a URL: rtsp://user:pass@host/path
 */
export function buildUrl(base: string, user: string, pass: string): string {
  if (!user.trim()) return base;
  try {
    const u = new URL(base);
    u.username = encodeURIComponent(user);
    u.password = encodeURIComponent(pass);
    return u.toString();
  } catch {
    const proto = base.match(/^[a-z]+:\/\//i)?.[0] ?? "rtsp://";
    const rest = base.slice(proto.length);
    const creds = pass
      ? `${encodeURIComponent(user)}:${encodeURIComponent(pass)}@`
      : `${encodeURIComponent(user)}@`;
    return `${proto}${creds}${rest}`;
  }
}
