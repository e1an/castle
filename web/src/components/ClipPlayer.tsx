import { useEffect, useRef } from "react";
import Hls from "hls.js";
import { recordingURL } from "../api";
import type { Event } from "../types";

interface Props {
  event: Event;
}

export function ClipPlayer({ event }: Props) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const hlsRef = useRef<Hls | null>(null);

  useEffect(() => {
    const video = videoRef.current;
    if (!video || !event.ClipPath) return;

    const url = recordingURL(event.ClipPath);
    const isHLS = url.endsWith(".m3u8");

    if (hlsRef.current) {
      hlsRef.current.destroy();
      hlsRef.current = null;
    }

    if (isHLS && Hls.isSupported()) {
      const hls = new Hls();
      hls.loadSource(url);
      hls.attachMedia(video);
      hlsRef.current = hls;
    } else {
      video.src = url;
    }
    video.play().catch(() => {});

    return () => {
      hlsRef.current?.destroy();
    };
  }, [event.ClipPath]);

  const label =
    event.Type === "object" && event.Label
      ? `${event.Label} — ${Math.round(event.Score * 100)}% confidence`
      : "Motion event";

  return (
    <div className="clip-player">
      <div className="clip-player__meta">
        <strong>{label}</strong>
        <span>{event.CameraID}</span>
        <span>{new Date(event.OccurredAt).toLocaleString()}</span>
      </div>
      <video
        ref={videoRef}
        controls
        muted
        playsInline
        className="clip-player__video"
      />
    </div>
  );
}
