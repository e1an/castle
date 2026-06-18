import { useEffect, useState, useCallback } from "react";
import { fetchEvents, fetchConfig } from "./api";
import { EventRow } from "./components/EventRow";
import { ClipPlayer } from "./components/ClipPlayer";
import { LiveView } from "./components/LiveView";
import { ConfigPanel } from "./components/ConfigPanel";
import { AddCameraWizard } from "./components/AddCameraWizard";
import type { Event } from "./types";
import "./App.css";

const POLL_MS = 10_000;
type View = "live" | "config";

export default function App() {
  const [view, setView] = useState<View>("live");
  const [showWizard, setShowWizard] = useState(false);
  const [events, setEvents] = useState<Event[]>([]);
  const [configCameras, setConfigCameras] = useState<string[]>([]);
  const [selected, setSelected] = useState<Event | null>(null);
  const [cameraID, setCameraID] = useState("");
  const [error, setError] = useState<string | null>(null);

  // Load enabled camera IDs from config — this is the source of truth for live view.
  const loadCameras = useCallback(async () => {
    try {
      const cfg = await fetchConfig();
      setConfigCameras(cfg.cameras.filter((c) => c.enable).map((c) => c.id));
    } catch {
      // Non-fatal — fall back to cameras derived from events.
    }
  }, []);

  const load = useCallback(async () => {
    try {
      const data = await fetchEvents(cameraID, 100);
      setEvents(data ?? []);
      setError(null);
    } catch (e) {
      setError(String(e));
    }
  }, [cameraID]);

  useEffect(() => {
    loadCameras();
    load();
    const id = setInterval(load, POLL_MS);
    return () => clearInterval(id);
  }, [load, loadCameras]);

  // Cameras for the dropdown: prefer config list, fall back to cameras seen in events.
  const eventCameras = Array.from(new Set(events.map((e) => e.CameraID))).sort();
  const cameras = configCameras.length > 0 ? configCameras : eventCameras;
  const activeCam = cameraID || cameras[0] || "";

  function openWizard() { setShowWizard(true); }
  function closeWizard() { setShowWizard(false); }
  function wizardDone() {
    setShowWizard(false);
    loadCameras(); // Refresh camera list immediately after adding one.
    setView("live");
  }

  return (
    <div className="layout">
      <header className="header">
        <span className="header__logo">Castle</span>
        <nav className="header__nav">
          {view === "live" && (
            <>
              <select
                value={cameraID}
                onChange={(e) => { setCameraID(e.target.value); setSelected(null); }}
                className="header__select"
              >
                <option value="">All cameras</option>
                {cameras.map((c) => (
                  <option key={c} value={c}>{c}</option>
                ))}
              </select>
              {selected && (
                <button onClick={() => setSelected(null)} className="header__back">← Live</button>
              )}
              <button onClick={load} className="header__btn">Refresh</button>
            </>
          )}
          <button
            className={`header__btn ${view === "config" ? "header__btn--active" : ""}`}
            onClick={() => setView(view === "config" ? "live" : "config")}
          >
            Config
          </button>
          <button className="header__btn header__btn--accent" onClick={openWizard}>
            + Camera
          </button>
        </nav>
      </header>

      {view === "live" ? (
        <main className="main">
          <section className="player-pane">
            {selected ? (
              <ClipPlayer event={selected} />
            ) : activeCam ? (
              <LiveView cameraID={activeCam} />
            ) : (
              <p className="player-pane__empty">No cameras configured. Use + Camera to add one.</p>
            )}
          </section>

          <aside className="sidebar">
            <h2 className="sidebar__title">Events</h2>
            {error && <p className="sidebar__error">{error}</p>}
            {events.length === 0 && !error && (
              <p className="sidebar__empty">No events yet.</p>
            )}
            <div className="event-list">
              {events.map((e) => (
                <EventRow
                  key={e.ID}
                  event={e}
                  selected={selected?.ID === e.ID}
                  onClick={() => setSelected(e)}
                />
              ))}
            </div>
          </aside>
        </main>
      ) : (
        <main className="main main--full">
          <ConfigPanel onAddCamera={openWizard} onSaved={loadCameras} />
        </main>
      )}

      {showWizard && (
        <AddCameraWizard onDone={wizardDone} onCancel={closeWizard} />
      )}
    </div>
  );
}
