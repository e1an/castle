import { useEffect, useState } from "react";
import { fetchConfig, saveConfig } from "../api";
import type { Config } from "../types";

interface Props {
  onAddCamera: () => void;
}

export function ConfigPanel({ onAddCamera }: Props) {
  const [cfg, setCfg] = useState<Config | null>(null);
  const [saving, setSaving] = useState(false);
  const [status, setStatus] = useState<{ ok: boolean; msg: string } | null>(null);

  useEffect(() => {
    fetchConfig().then(setCfg).catch((e) => setStatus({ ok: false, msg: String(e) }));
  }, []);

  if (!cfg) {
    return <div className="config-loading">{status ? status.msg : "Loading config…"}</div>;
  }

  function set<K extends keyof Config>(section: K, patch: Partial<Config[K]>) {
    setCfg((prev) => prev ? { ...prev, [section]: { ...(prev[section] as object), ...patch } } : prev);
  }

  async function handleSave() {
    if (!cfg) return;
    setSaving(true);
    setStatus(null);
    try {
      await saveConfig(cfg);
      setStatus({ ok: true, msg: "Saved and reloaded." });
    } catch (e) {
      setStatus({ ok: false, msg: String(e) });
    } finally {
      setSaving(false);
    }
  }

  function removeCamera(id: string) {
    setCfg((prev) => prev ? { ...prev, cameras: prev.cameras.filter((c) => c.id !== id) } : prev);
  }

  function toggleCamera(id: string) {
    setCfg((prev) => prev ? {
      ...prev,
      cameras: prev.cameras.map((c) => c.id === id ? { ...c, enable: !c.enable } : c),
    } : prev);
  }

  return (
    <div className="config-panel">
      <div className="config-panel__header">
        <h2>Configuration</h2>
        <button className="btn btn--primary" onClick={handleSave} disabled={saving}>
          {saving ? "Saving…" : "Save & Reload"}
        </button>
      </div>

      {status && (
        <div className={`config-panel__status ${status.ok ? "config-panel__status--ok" : "config-panel__status--err"}`}>
          {status.msg}
        </div>
      )}

      {/* Cameras */}
      <section className="config-section">
        <div className="config-section__title-row">
          <h3>Cameras</h3>
          <button className="btn btn--secondary" onClick={onAddCamera}>+ Add Camera</button>
        </div>
        {cfg.cameras.length === 0 && <p className="config-section__empty">No cameras configured.</p>}
        <div className="camera-list">
          {cfg.cameras.map((cam) => (
            <div key={cam.id} className="camera-row">
              <label className="camera-row__toggle">
                <input type="checkbox" checked={cam.enable} onChange={() => toggleCamera(cam.id)} />
                <span className="camera-row__name">{cam.name || cam.id}</span>
              </label>
              <span className="camera-row__url">{cam.url}</span>
              <button className="btn btn--danger btn--sm" onClick={() => removeCamera(cam.id)}>Remove</button>
            </div>
          ))}
        </div>
      </section>

      {/* Recording */}
      <section className="config-section">
        <h3>Recording</h3>
        <div className="config-grid">
          <label>
            Storage path
            <input value={cfg.record.path} onChange={(e) => set("record", { path: e.target.value })} />
          </label>
          <label>
            Segment duration (s)
            <input type="number" min={1} value={cfg.record.segment_duration}
              onChange={(e) => set("record", { segment_duration: Number(e.target.value) })} />
          </label>
          <label>
            Retention (days)
            <input type="number" min={1} value={cfg.record.retention_days}
              onChange={(e) => set("record", { retention_days: Number(e.target.value) })} />
          </label>
          <label className="config-grid__checkbox">
            <input type="checkbox" checked={cfg.record.continuous_mode}
              onChange={(e) => set("record", { continuous_mode: e.target.checked })} />
            Continuous mode (always record)
          </label>
        </div>
      </section>

      {/* Detection */}
      <section className="config-section">
        <h3>Detection</h3>
        <div className="config-grid">
          <label>
            Motion threshold
            <input type="number" step={0.001} min={0} max={1} value={cfg.detect.motion_threshold}
              onChange={(e) => set("detect", { motion_threshold: Number(e.target.value) })} />
          </label>
          <label className="config-grid__checkbox">
            <input type="checkbox" checked={cfg.detect.enable_object_detect}
              onChange={(e) => set("detect", { enable_object_detect: e.target.checked })} />
            Enable object detection (ONNX)
          </label>
          <label>
            Min object score
            <input type="number" step={0.05} min={0} max={1} value={cfg.detect.min_object_score}
              onChange={(e) => set("detect", { min_object_score: Number(e.target.value) })} />
          </label>
          <label>
            Model path
            <input value={cfg.detect.model_path}
              onChange={(e) => set("detect", { model_path: e.target.value })} />
          </label>
        </div>
      </section>

      {/* Notifications */}
      <section className="config-section">
        <h3>Notifications</h3>
        <div className="config-grid">
          <label>
            Webhook URL
            <input placeholder="https://…" value={cfg.notify.webhook_url}
              onChange={(e) => set("notify", { webhook_url: e.target.value })} />
          </label>
          <label>
            ntfy topic
            <input placeholder="https://ntfy.sh/my-alerts" value={cfg.notify.ntfy_topic}
              onChange={(e) => set("notify", { ntfy_topic: e.target.value })} />
          </label>
        </div>
      </section>

      {/* Server (read-only — requires restart) */}
      <section className="config-section">
        <h3>Server <span className="config-section__note">(changes require restart)</span></h3>
        <div className="config-grid">
          <label>
            Bind address
            <input value={cfg.server.host} onChange={(e) => set("server", { host: e.target.value })} />
          </label>
          <label>
            Port
            <input type="number" value={cfg.server.port}
              onChange={(e) => set("server", { port: Number(e.target.value) })} />
          </label>
        </div>
      </section>
    </div>
  );
}
