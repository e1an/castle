import { useEffect, useRef } from "react";
import Hls from "hls.js";

interface Props {
  cameraID: string;
}

export function LiveView({ cameraID }: Props) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const hlsRef = useRef<Hls | null>(null);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;

    const url = `/recordings/${cameraID}/live.m3u8`;

    hlsRef.current?.destroy();

    if (Hls.isSupported()) {
      const hls = new Hls({
        liveSyncDurationCount: 2,       // stay 2 segments behind live edge
        liveMaxLatencyDurationCount: 5, // resync if >5 segments behind
        maxBufferLength: 8,
        maxMaxBufferLength: 15,
        lowLatencyMode: false,          // off — this camera isn't LL-HLS
      });

      hls.loadSource(url);
      hls.attachMedia(video);

      hls.on(Hls.Events.MANIFEST_PARSED, () => {
        video.play().catch(() => {});
      });

      // On fatal error, destroy and reload after a short pause.
      hls.on(Hls.Events.ERROR, (_e, data) => {
        if (data.fatal) {
          setTimeout(() => {
            hls.stopLoad();
            hls.loadSource(url);
            hls.startLoad();
          }, 3000);
        }
      });

      hlsRef.current = hls;
    } else if (video.canPlayType("application/vnd.apple.mpegurl")) {
      video.src = url;
      video.play().catch(() => {});
    }

    return () => {
      hlsRef.current?.destroy();
    };
  }, [cameraID]);

  return (
    <div className="live-view">
      <div className="live-view__label">LIVE — {cameraID}</div>
      <video
        ref={videoRef}
        muted
        playsInline
        autoPlay
        className="live-view__video"
      />
    </div>
  );
}
