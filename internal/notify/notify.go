package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Payload struct {
	CameraID  string    `json:"camera_id"`
	EventType string    `json:"event_type"`
	Label     string    `json:"label,omitempty"`
	Score     float64   `json:"score,omitempty"`
	ClipPath  string    `json:"clip_path,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

type Notifier struct {
	webhookURL string
	ntfyURL    string // full URL including topic, e.g. https://ntfy.sh/my-topic
	client     *http.Client
}

func New(webhookURL, ntfyURL string) *Notifier {
	return &Notifier{
		webhookURL: webhookURL,
		ntfyURL:    ntfyURL,
		client:     &http.Client{Timeout: 5 * time.Second},
	}
}

func (n *Notifier) Send(p Payload) {
	if n.webhookURL != "" {
		go n.sendWebhook(p)
	}
	if n.ntfyURL != "" {
		go n.sendNtfy(p)
	}
}

func (n *Notifier) sendWebhook(p Payload) {
	body, _ := json.Marshal(p)
	resp, err := n.client.Post(n.webhookURL, "application/json", bytes.NewReader(body))
	if err != nil {
		return
	}
	resp.Body.Close()
}

func (n *Notifier) sendNtfy(p Payload) {
	msg := fmt.Sprintf("[%s] %s detected", p.CameraID, eventSummary(p))
	req, err := http.NewRequest("POST", n.ntfyURL, bytes.NewBufferString(msg))
	if err != nil {
		return
	}
	req.Header.Set("Title", "Castle Alert")
	req.Header.Set("Priority", "high")
	resp, err := n.client.Do(req)
	if err != nil {
		return
	}
	resp.Body.Close()
}

func eventSummary(p Payload) string {
	if p.Label != "" {
		return fmt.Sprintf("%s (%.0f%%)", p.Label, p.Score*100)
	}
	return p.EventType
}
