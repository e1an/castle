package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server  ServerConfig `yaml:"server"  json:"server"`
	Cameras []Camera     `yaml:"cameras" json:"cameras"`
	Record  RecordConfig `yaml:"record"  json:"record"`
	Detect  DetectConfig `yaml:"detect"  json:"detect"`
	Notify  NotifyConfig `yaml:"notify"  json:"notify"`
}

type ServerConfig struct {
	Host    string `yaml:"host"     json:"host"`
	Port    int    `yaml:"port"     json:"port"`
	TLSCert string `yaml:"tls_cert" json:"tls_cert"`
	TLSKey  string `yaml:"tls_key"  json:"tls_key"`
}

// LabelConfig holds per-label detection filtering thresholds for a camera.
type LabelConfig struct {
	MinScore float64 `yaml:"min_score,omitempty" json:"min_score,omitempty"`
	MinArea  int     `yaml:"min_area,omitempty"  json:"min_area,omitempty"`
}

// CameraDetect holds per-camera detection overrides.
// Nil pointer fields mean "use the global default" (enabled when a model is loaded).
type CameraDetect struct {
	MotionThreshold    *float64               `yaml:"motion_threshold,omitempty"     json:"motion_threshold,omitempty"`
	EnableObjectDetect *bool                  `yaml:"enable_object_detect,omitempty" json:"enable_object_detect,omitempty"`
	EnableFaceDetect   *bool                  `yaml:"enable_face_detect,omitempty"   json:"enable_face_detect,omitempty"`
	MinObjectScore     *float64               `yaml:"min_object_score,omitempty"     json:"min_object_score,omitempty"`
	Labels             map[string]LabelConfig `yaml:"labels,omitempty"               json:"labels,omitempty"`
}

type Camera struct {
	ID              string        `yaml:"id"                         json:"id"`
	Name            string        `yaml:"name"                       json:"name"`
	URL             string        `yaml:"url"                        json:"url"`
	Enable          bool          `yaml:"enable"                     json:"enable"`
	CooldownSeconds int           `yaml:"cooldown_seconds,omitempty" json:"cooldown_seconds,omitempty"`
	Detect          *CameraDetect `yaml:"detect,omitempty"           json:"detect,omitempty"`
}

type RecordConfig struct {
	Path            string `yaml:"path"             json:"path"`
	SegmentDuration int    `yaml:"segment_duration" json:"segment_duration"`
	RetentionDays   int    `yaml:"retention_days"   json:"retention_days"`
	ContinuousMode  bool   `yaml:"continuous_mode"  json:"continuous_mode"`
}

type DetectConfig struct {
	MotionThreshold float64 `yaml:"motion_threshold" json:"motion_threshold"`
	MinObjectScore  float64 `yaml:"min_object_score" json:"min_object_score"`
	ModelPath       string  `yaml:"model_path"       json:"model_path"`
	FaceModelPath   string  `yaml:"face_model_path"  json:"face_model_path"`
}

type NotifyConfig struct {
	WebhookURL      string `yaml:"webhook_url"       json:"webhook_url"`
	NtfyTopic       string `yaml:"ntfy_topic"        json:"ntfy_topic"`
	VAPIDPublicKey  string `yaml:"vapid_public_key"  json:"-"`
	VAPIDPrivateKey string `yaml:"vapid_private_key" json:"-"`
}

func Load(path string) (*Config, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	cfg := &Config{}
	if err := yaml.NewDecoder(f).Decode(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

func Save(path string, cfg *Config) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	return enc.Encode(cfg)
}

func Default() *Config {
	return &Config{
		Server: ServerConfig{Host: "0.0.0.0", Port: 8080},
		Record: RecordConfig{
			Path:            "/recordings",
			SegmentDuration: 10,
			RetentionDays:   7,
			ContinuousMode:  false,
		},
		Detect: DetectConfig{
			MotionThreshold: 0.02,
			MinObjectScore:  0.5,
			ModelPath:       "/models/yolov8n.onnx",
			FaceModelPath:   "/models/yolov8n-face.onnx",
		},
	}
}
