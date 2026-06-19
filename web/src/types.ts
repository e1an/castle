export interface Event {
  ID: number;
  CameraID: string;
  Type: "motion" | "object";
  Label: string;
  Score: number;
  ClipPath: string;
  SnapshotPath: string;
  CropPath: string;
  OccurredAt: string; // ISO 8601
}

export interface LabelConfig {
  min_score?: number;
  min_area?: number;
}

export interface CameraDetect {
  motion_threshold?: number | null;
  enable_object_detect?: boolean | null;
  enable_face_detect?: boolean | null;
  min_object_score?: number | null;
  labels?: Record<string, LabelConfig> | null;
}

export interface Camera {
  id: string;
  name: string;
  url: string;
  enable: boolean;
  cooldown_seconds?: number;
  detect?: CameraDetect | null;
}

export interface ServerConfig {
  host: string;
  port: number;
  tls_cert?: string;
  tls_key?: string;
}

export interface RecordConfig {
  path: string;
  segment_duration: number;
  retention_days: number;
  continuous_mode: boolean;
}

export interface DetectConfig {
  motion_threshold: number;
  min_object_score: number;
  model_path: string;
  face_model_path?: string;
}

export interface NotifyConfig {
  webhook_url: string;
  ntfy_topic: string;
}

export interface Config {
  server: ServerConfig;
  cameras: Camera[];
  record: RecordConfig;
  detect: DetectConfig;
  notify: NotifyConfig;
}
