import type { Config, Event } from "./types";

const BASE = import.meta.env.VITE_API_URL ?? "";

export async function fetchEvents(cameraID = "", limit = 50): Promise<Event[]> {
  const params = new URLSearchParams({ limit: String(limit) });
  if (cameraID) params.set("camera_id", cameraID);
  const res = await fetch(`${BASE}/api/events?${params}`);
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json();
}

export function recordingURL(clipPath: string): string {
  return `${BASE}/recordings/${clipPath}`;
}

export async function fetchConfig(): Promise<Config> {
  const res = await fetch(`${BASE}/api/config`);
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
  return res.json();
}

export async function saveConfig(cfg: Config): Promise<void> {
  const res = await fetch(`${BASE}/api/config`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(cfg),
  });
  if (!res.ok) {
    const text = await res.text();
    throw new Error(text || `${res.status} ${res.statusText}`);
  }
}

export async function reloadServer(): Promise<void> {
  const res = await fetch(`${BASE}/api/reload`, { method: "POST" });
  if (!res.ok) throw new Error(`${res.status} ${res.statusText}`);
}

export async function testStream(url: string): Promise<{ streams?: { codec_type: string; codec_name: string; width?: number; height?: number }[]; error?: string }> {
  const res = await fetch(`${BASE}/api/test-stream`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ url }),
  });
  return res.json();
}
