package push

import (
	"encoding/json"
	"log"

	webpush "github.com/SherClockHolmes/webpush-go"
	"github.com/e1an/castle/internal/events"
)

type Payload struct {
	Title    string `json:"title"`
	Body     string `json:"body"`
	CameraID string `json:"camera_id"`
	URL      string `json:"url"`
	ImageURL string `json:"image_url,omitempty"`
}

type Sender struct {
	publicKey  string
	privateKey string
}

func NewSender(publicKey, privateKey string) *Sender {
	return &Sender{publicKey: publicKey, privateKey: privateKey}
}

func (s *Sender) Send(subs []events.PushSubscription, p Payload) {
	data, _ := json.Marshal(p)
	for _, sub := range subs {
		sub := sub
		go s.sendOne(data, sub)
	}
}

func (s *Sender) sendOne(data []byte, sub events.PushSubscription) {
	resp, err := webpush.SendNotification(data, &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			Auth:   sub.Auth,
			P256dh: sub.P256DH,
		},
	}, &webpush.Options{
		Subscriber:      "mailto:admin@localhost",
		VAPIDPublicKey:  s.publicKey,
		VAPIDPrivateKey: s.privateKey,
		TTL:             30,
	})
	if err != nil {
		log.Printf("push send: %v", err)
		return
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 {
		log.Printf("push send: HTTP %d for endpoint %s…", resp.StatusCode, sub.Endpoint[:min(len(sub.Endpoint), 60)])
	}
}
