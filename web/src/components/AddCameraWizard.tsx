import { useState } from "react";
import { fetchConfig, saveConfig, testStream } from "../api";
import { maskUrl } from "../utils/url";
import type { Camera } from "../types";

interface Props {
  onDone: () => void;
  onCancel: () => void;
}

type Step = "details" | "test" | "confirm";

function slugify(name: string) {
  return name.toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/(^-|-$)/g, "");
}

// Embed credentials into an RTSP URL: rtsp://user:pass@host/path
function buildUrl(base: string, user: string, pass: string): string {
  if (!user.trim()) return base;
  try {
    const u = new URL(base);
    u.username = encodeURIComponent(user);
    u.password = encodeURIComponent(pass);
    return u.toString();
  } catch {
    // If URL parsing fails, inject manually before the host
    const proto = base.match(/^[a-z]+:\/\//i)?.[0] ?? "rtsp://";
    const rest = base.slice(proto.length);
    const creds = pass ? `${encodeURIComponent(user)}:${encodeURIComponent(pass)}@` : `${encodeURIComponent(user)}@`;
    return `${proto}${creds}${rest}`;
  }
}


export function AddCameraWizard({ onDone, onCancel }: Props) {
  const [step, setStep] = useState<Step>("details");
  const [baseUrl, setBaseUrl] = useState("");
  const [rtspUser, setRtspUser] = useState("");
  const [rtspPass, setRtspPass] = useState("");
  const [cam, setCam] = useState<Camera>({ id: "", name: "", url: "", enable: true });
  const [idEdited, setIdEdited] = useState(false);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ ok: boolean; detail: string } | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function handleNameChange(name: string) {
    setCam((prev) => ({
      ...prev,
      name,
      id: idEdited ? prev.id : slugify(name),
    }));
  }

  function advanceToTest() {
    const fullUrl = buildUrl(baseUrl, rtspUser, rtspPass);
    setCam((prev) => ({ ...prev, url: fullUrl }));
    setStep("test");
  }

  async function handleTest() {
    setTesting(true);
    setTestResult(null);
    try {
      const result = await testStream(cam.url);
      if (result.error) {
        setTestResult({ ok: false, detail: result.error });
      } else {
        const video = result.streams?.find((s) => s.codec_type === "video");
        const detail = video
          ? `${video.codec_name.toUpperCase()} ${video.width}×${video.height}`
          : "Stream reachable";
        setTestResult({ ok: true, detail });
      }
    } catch {
      setTestResult({ ok: false, detail: "Test failed — server unreachable" });
    } finally {
      setTesting(false);
    }
  }

  async function handleSave() {
    setSaving(true);
    setError(null);
    try {
      const cfg = await fetchConfig();
      if (cfg.cameras.some((c) => c.id === cam.id)) {
        setError(`Camera ID "${cam.id}" already exists.`);
        setSaving(false);
        return;
      }
      cfg.cameras.push(cam);
      await saveConfig(cfg);
      onDone();
    } catch (e) {
      setError(String(e));
    } finally {
      setSaving(false);
    }
  }

  const detailsValid = cam.name.trim() !== "" && cam.id.trim() !== "" && baseUrl.trim() !== "";

  return (
    <div className="wizard-overlay" onClick={(e) => e.target === e.currentTarget && onCancel()}>
      <div className="wizard">
        <div className="wizard__header">
          <h2>Add Camera</h2>
          <button className="wizard__close" onClick={onCancel}>✕</button>
        </div>

        <div className="wizard__steps">
          {(["details", "test", "confirm"] as Step[]).map((s, i) => (
            <span key={s} className={`wizard__step ${step === s ? "wizard__step--active" : ""}`}>
              {i + 1}. {s.charAt(0).toUpperCase() + s.slice(1)}
            </span>
          ))}
        </div>

        <div className="wizard__body">
          {step === "details" && (
            <div className="config-grid">
              <label>
                Camera name
                <input
                  autoFocus
                  placeholder="Front Door"
                  value={cam.name}
                  onChange={(e) => handleNameChange(e.target.value)}
                />
              </label>
              <label>
                Camera ID <span className="field-note">(used in file paths)</span>
                <input
                  placeholder="front-door"
                  value={cam.id}
                  onChange={(e) => { setIdEdited(true); setCam((p) => ({ ...p, id: e.target.value })); }}
                />
              </label>
              <label className="config-grid--full">
                RTSP / stream URL <span className="field-note">(without credentials)</span>
                <input
                  placeholder="rtsp://192.168.1.100:554/stream"
                  value={baseUrl}
                  onChange={(e) => setBaseUrl(e.target.value)}
                />
              </label>
              <label>
                Username <span className="field-note">(if required)</span>
                <input
                  placeholder="admin"
                  value={rtspUser}
                  onChange={(e) => setRtspUser(e.target.value)}
                />
              </label>
              <label>
                Password
                <input
                  type="password"
                  placeholder="••••••••"
                  value={rtspPass}
                  onChange={(e) => setRtspPass(e.target.value)}
                />
              </label>
              <label className="config-grid__checkbox config-grid--full">
                <input type="checkbox" checked={cam.enable}
                  onChange={(e) => setCam((p) => ({ ...p, enable: e.target.checked }))} />
                Enable camera immediately
              </label>
            </div>
          )}

          {step === "test" && (
            <div className="wizard__test">
              <p className="wizard__test-url">{maskUrl(cam.url)}</p>
              <button className="btn btn--secondary" onClick={handleTest} disabled={testing}>
                {testing ? "Testing…" : "Test connection"}
              </button>
              {testResult && (
                <div className={`wizard__test-result ${testResult.ok ? "wizard__test-result--ok" : "wizard__test-result--err"}`}>
                  {testResult.ok ? "✓ " : "✗ "}{testResult.detail}
                </div>
              )}
              <p className="wizard__test-skip">You can skip testing and proceed to save.</p>
            </div>
          )}

          {step === "confirm" && (
            <div className="wizard__confirm">
              <div className="wizard__summary">
                <div><span>Name</span><strong>{cam.name}</strong></div>
                <div><span>ID</span><strong>{cam.id}</strong></div>
                <div><span>URL</span><strong>{maskUrl(cam.url)}</strong></div>
                <div><span>Enabled</span><strong>{cam.enable ? "Yes" : "No"}</strong></div>
              </div>
              {error && <p className="wizard__error">{error}</p>}
            </div>
          )}
        </div>

        <div className="wizard__footer">
          {step !== "details" && (
            <button className="btn btn--ghost" onClick={() => {
              if (step === "test") setStep("details");
              else if (step === "confirm") setStep("test");
            }}>
              Back
            </button>
          )}
          <span style={{ flex: 1 }} />
          <button className="btn btn--ghost" onClick={onCancel}>Cancel</button>
          {step === "details" && (
            <button className="btn btn--primary" disabled={!detailsValid} onClick={advanceToTest}>
              Next
            </button>
          )}
          {step === "test" && (
            <button className="btn btn--primary" onClick={() => setStep("confirm")}>
              Next
            </button>
          )}
          {step === "confirm" && (
            <button className="btn btn--primary" onClick={handleSave} disabled={saving}>
              {saving ? "Saving…" : "Add Camera"}
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
