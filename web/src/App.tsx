import { useEffect, useState, useCallback } from "react";
import { fetchEvents } from "./api";
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
  const [selected, setSelected] = useState<Event | null>(null);
  const [cameraID, setCameraID] = useState("");
  const [error, setError] = useState<string | null>(null);

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
    load();
    const id = setInterval(load, POLL_MS);
    return () => clearInterval(id);
  }, [load]);

  const cameras = Array.from(new Set(events.map((e) => e.CameraID))).sort();
  const activeCam = cameraID || cameras[0] || "cam1";

  function openWizard() { setShowWizard(true); }
  function closeWizard() { setShowWizard(false); }
  function wizardDone() { setShowWizard(false); setView("config"); }

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
            ) : (
              <LiveView cameraID={activeCam} />
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
          <ConfigPanel onAddCamera={openWizard} />
        </main>
      )}

      {showWizard && (
        <AddCameraWizard onDone={wizardDone} onCancel={closeWizard} />
      )}
    </div>
  );
}
