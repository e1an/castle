import { useEffect, useState } from "react";
import { fetchConfig, saveConfig } from "../api";
import { maskUrl, parseUrl, buildUrl } from "../utils/url";
import type { CameraDetect, Config } from "../types";
import { PushToggle } from "./PushToggle";

interface Props {
  onAddCamera: () => void;
  onSaved?: () => void;
}

interface LabelEntry {
  label: string;
  minScore: string;
  minArea: string;
}

interface CamEdit {
  name: string;
  baseUrl: string;
  username: string;
  password: string;
  cooldownSeconds: string;
  detectMotionThreshold: string;
  detectEnableOD: "" | "true" | "false";
  detectEnableFace: "" | "true" | "false";
  detectMinScore: string;
  labels: LabelEntry[];
}

export function ConfigPanel({ onAddCamera, onSaved }: Props) {
  const [cfg, setCfg] = useState<Config | null>(null);
  const [saving, setSaving] = useState(false);
  const [status, setStatus] = useState<{ ok: boolean; msg: string } | null>(null);
  const [editingId, setEditingId] = useState<string | null>(null);
  const [edit, setEdit] = useState<CamEdit>({
    name: "", baseUrl: "", username: "", password: "",
    cooldownSeconds: "",
    detectMotionThreshold: "", detectEnableOD: "", detectEnableFace: "", detectMinScore: "", labels: [],
  });

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
      onSaved?.();
    } catch (e) {
      setStatus({ ok: false, msg: String(e) });
    } finally {
      setSaving(false);
    }
  }

  function removeCamera(id: string) {
    setEditingId(null);
    setCfg((prev) => prev ? { ...prev, cameras: prev.cameras.filter((c) => c.id !== id) } : prev);
  }

  function toggleCamera(id: string) {
    setCfg((prev) => prev ? {
      ...prev,
      cameras: prev.cameras.map((c) => c.id === id ? { ...c, enable: !c.enable } : c),
    } : prev);
  }

  function startEdit(id: string) {
    const cam = cfg?.cameras.find((c) => c.id === id);
    if (!cam) return;
    const { baseUrl, username, password } = parseUrl(cam.url);
    const d = cam.detect;
    setEdit({
      name: cam.name, baseUrl, username, password,
      cooldownSeconds: cam.cooldown_seconds != null ? String(cam.cooldown_seconds) : "",
      detectMotionThreshold: d?.motion_threshold != null ? String(d.motion_threshold) : "",
      detectEnableOD: d?.enable_object_detect != null ? (d.enable_object_detect ? "true" : "false") : "",
      detectEnableFace: d?.enable_face_detect != null ? (d.enable_face_detect ? "true" : "false") : "",
      detectMinScore: d?.min_object_score != null ? String(d.min_object_score) : "",
      labels: d?.labels
        ? Object.entries(d.labels).map(([label, lc]) => ({
            label,
            minScore: lc.min_score ? String(lc.min_score) : "",
            minArea: lc.min_area ? String(lc.min_area) : "",
          }))
        : [],
    });
    setEditingId(id);
  }

  function commitEdit(id: string) {
    const url = buildUrl(edit.baseUrl, edit.username, edit.password);
    const detect = buildDetect(edit);
    const cooldown_seconds = edit.cooldownSeconds !== "" ? Number(edit.cooldownSeconds) : undefined;
    setCfg((prev) => prev != null ? {
      ...prev,
      cameras: prev.cameras.map((c) =>
        c.id === id ? { ...c, name: edit.name, url, cooldown_seconds, detect } : c
      ),
    } : prev);
    setEditingId(null);
  }

  function buildDetect(e: CamEdit): CameraDetect | undefined {
    const d: CameraDetect = {};
    if (e.detectMotionThreshold !== "") d.motion_threshold = Number(e.detectMotionThreshold);
    if (e.detectEnableOD !== "") d.enable_object_detect = e.detectEnableOD === "true";
    if (e.detectEnableFace !== "") d.enable_face_detect = e.detectEnableFace === "true";
    if (e.detectMinScore !== "") d.min_object_score = Number(e.detectMinScore);
    const validLabels = e.labels.filter((l) => l.label.trim() !== "");
    if (validLabels.length > 0) {
      d.labels = {};
      for (const { label, minScore, minArea } of validLabels) {
        d.labels[label.trim()] = {
          min_score: minScore !== "" ? Number(minScore) : undefined,
          min_area: minArea !== "" ? Number(minArea) : undefined,
        };
      }
    }
    return Object.keys(d).length > 0 ? d : undefined;
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
            <div key={cam.id} className="camera-card">
              {editingId === cam.id ? (
                <div className="camera-edit">
                  <div className="config-grid">
                    <label>
                      Name
                      <input value={edit.name} onChange={(e) => setEdit((p) => ({ ...p, name: e.target.value }))} />
                    </label>
                    <label>
                      ID <span className="field-note">(read-only)</span>
                      <input value={cam.id} disabled />
                    </label>
                    <label className="config-grid--full">
                      Stream URL <span className="field-note">(without credentials)</span>
                      <input value={edit.baseUrl} onChange={(e) => setEdit((p) => ({ ...p, baseUrl: e.target.value }))} />
                    </label>
                    <label>
                      Username
                      <input value={edit.username} onChange={(e) => setEdit((p) => ({ ...p, username: e.target.value }))} />
                    </label>
                    <label>
                      Password
                      <input type="password" value={edit.password} onChange={(e) => setEdit((p) => ({ ...p, password: e.target.value }))} />
                    </label>
                    <label>
                      Notification cooldown (s) <span className="field-note">(0 or blank = always notify)</span>
                      <input type="number" min={0} step={10}
                        value={edit.cooldownSeconds} placeholder="0"
                        onChange={(e) => setEdit((p) => ({ ...p, cooldownSeconds: e.target.value }))} />
                    </label>
                    <label>
                      Motion threshold <span className="field-note">(blank = global)</span>
                      <input type="number" step={0.001} min={0} max={1}
                        value={edit.detectMotionThreshold} placeholder="global"
                        onChange={(e) => setEdit((p) => ({ ...p, detectMotionThreshold: e.target.value }))} />
                    </label>
                    <label>
                      Object detection
                      <select value={edit.detectEnableOD}
                        onChange={(e) => setEdit((p) => ({ ...p, detectEnableOD: e.target.value as "" | "true" | "false" }))}>
                        <option value="">On (model loaded)</option>
                        <option value="true">On</option>
                        <option value="false">Off</option>
                      </select>
                    </label>
                    <label>
                      Face detection
                      <select value={edit.detectEnableFace}
                        onChange={(e) => setEdit((p) => ({ ...p, detectEnableFace: e.target.value as "" | "true" | "false" }))}>
                        <option value="">On (model loaded)</option>
                        <option value="true">On</option>
                        <option value="false">Off</option>
                      </select>
                    </label>
                    <label>
                      Min object score <span className="field-note">(blank = global)</span>
                      <input type="number" step={0.05} min={0} max={1}
                        value={edit.detectMinScore} placeholder="global"
                        onChange={(e) => setEdit((p) => ({ ...p, detectMinScore: e.target.value }))} />
                    </label>
                    <div className="config-grid--full">
                      <div className="label-list__header">
                        <span className="field-note">Label allow-list — empty means allow all labels</span>
                        <button className="btn btn--secondary btn--sm" type="button"
                          onClick={() => setEdit((p) => ({ ...p, labels: [...p.labels, { label: "", minScore: "", minArea: "" }] }))}>
                          + Add label
                        </button>
                      </div>
                      {edit.labels.length > 0 && (
                        <div className="label-list">
                          <div className="label-list__row label-list__row--head">
                            <span>Label</span><span>Min score</span><span>Min area (px²)</span><span />
                          </div>
                          {edit.labels.map((entry, i) => (
                            <div key={i} className="label-list__row">
                              <input placeholder="person" value={entry.label}
                                onChange={(e) => setEdit((p) => {
                                  const labels = [...p.labels];
                                  labels[i] = { ...labels[i], label: e.target.value };
                                  return { ...p, labels };
                                })} />
                              <input type="number" step={0.05} min={0} max={1} placeholder="0.50"
                                value={entry.minScore}
                                onChange={(e) => setEdit((p) => {
                                  const labels = [...p.labels];
                                  labels[i] = { ...labels[i], minScore: e.target.value };
                                  return { ...p, labels };
                                })} />
                              <input type="number" min={0} placeholder="none"
                                value={entry.minArea}
                                onChange={(e) => setEdit((p) => {
                                  const labels = [...p.labels];
                                  labels[i] = { ...labels[i], minArea: e.target.value };
                                  return { ...p, labels };
                                })} />
                              <button className="btn btn--ghost btn--sm" type="button"
                                onClick={() => setEdit((p) => ({ ...p, labels: p.labels.filter((_, j) => j !== i) }))}>
                                ×
                              </button>
                            </div>
                          ))}
                        </div>
                      )}
                    </div>
                  </div>
                  <div className="camera-edit__actions">
                    <button className="btn btn--primary btn--sm" onClick={() => commitEdit(cam.id)}>Done</button>
                    <button className="btn btn--ghost btn--sm" onClick={() => setEditingId(null)}>Cancel</button>
                    <button className="btn btn--danger btn--sm" onClick={() => removeCamera(cam.id)}>Remove</button>
                  </div>
                </div>
              ) : (
                <div className="camera-row">
                  <label className="camera-row__toggle">
                    <input type="checkbox" checked={cam.enable} onChange={() => toggleCamera(cam.id)} />
                    <span className="camera-row__name">{cam.name || cam.id}</span>
                  </label>
                  <span className="camera-row__url">{maskUrl(cam.url)}</span>
                  <button className="btn btn--secondary btn--sm" onClick={() => startEdit(cam.id)}>Edit</button>
                  <button className="btn btn--danger btn--sm" onClick={() => removeCamera(cam.id)}>Remove</button>
                </div>
              )}
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
          <label>
            Min object score
            <input type="number" step={0.05} min={0} max={1} value={cfg.detect.min_object_score}
              onChange={(e) => set("detect", { min_object_score: Number(e.target.value) })} />
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
        <div style={{ marginTop: "1rem" }}>
          <PushToggle />
        </div>
      </section>

      {/* Server */}
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
