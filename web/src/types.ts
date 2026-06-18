export interface Event {
  ID: number;
  CameraID: string;
  Type: "motion" | "object";
  Label: string;
  Score: number;
  ClipPath: string;
  OccurredAt: string; // ISO 8601
}

export interface Camera {
  id: string;
  name: string;
  url: string;
  enable: boolean;
}

export interface ServerConfig {
  host: string;
  port: number;
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
  enable_object_detect: boolean;
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
