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
	Host string `yaml:"host" json:"host"`
	Port int    `yaml:"port" json:"port"`
}

type Camera struct {
	ID     string `yaml:"id"     json:"id"`
	Name   string `yaml:"name"   json:"name"`
	URL    string `yaml:"url"    json:"url"`
	Enable bool   `yaml:"enable" json:"enable"`
}

type RecordConfig struct {
	Path            string `yaml:"path"             json:"path"`
	SegmentDuration int    `yaml:"segment_duration" json:"segment_duration"`
	RetentionDays   int    `yaml:"retention_days"   json:"retention_days"`
	ContinuousMode  bool   `yaml:"continuous_mode"  json:"continuous_mode"`
}

type DetectConfig struct {
	MotionThreshold    float64 `yaml:"motion_threshold"     json:"motion_threshold"`
	MinObjectScore     float64 `yaml:"min_object_score"     json:"min_object_score"`
	ModelPath          string  `yaml:"model_path"           json:"model_path"`
	EnableObjectDetect bool    `yaml:"enable_object_detect" json:"enable_object_detect"`
}

type NotifyConfig struct {
	WebhookURL string `yaml:"webhook_url" json:"webhook_url"`
	NtfyTopic  string `yaml:"ntfy_topic"  json:"ntfy_topic"`
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
			MotionThreshold:    0.02,
			MinObjectScore:     0.5,
			EnableObjectDetect: false,
		},
	}
}
