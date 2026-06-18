import type { Event } from "../types";

interface Props {
  event: Event;
  selected: boolean;
  onClick: () => void;
}

export function EventRow({ event, selected, onClick }: Props) {
  const label =
    event.Type === "object" && event.Label
      ? `${event.Label} (${Math.round(event.Score * 100)}%)`
      : "motion";

  const time = new Date(event.OccurredAt).toLocaleTimeString();
  const date = new Date(event.OccurredAt).toLocaleDateString();

  return (
    <button
      onClick={onClick}
      className={`event-row${selected ? " event-row--selected" : ""}`}
    >
      <span className="event-row__badge">{label}</span>
      <span className="event-row__cam">{event.CameraID}</span>
      <span className="event-row__time">
        {date} {time}
      </span>
    </button>
  );
}
