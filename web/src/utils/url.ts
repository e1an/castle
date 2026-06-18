/** Returns the URL with username and password replaced by bullets. */
export function maskUrl(url: string): string {
  try {
    const u = new URL(url);
    if (u.username) {
      u.username = "••••";
      u.password = u.password ? "••••" : "";
    }
    return u.toString();
  } catch {
    return url;
  }
}
