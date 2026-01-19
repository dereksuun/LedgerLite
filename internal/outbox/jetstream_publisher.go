package outbox

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type JetStreamPublisher struct {
	log           *log.Logger
	nc            *nats.Conn
	js            jetstream.JetStream
	stream        string
	subjectPrefix string
}

func NewJetStreamPublisher(ctx context.Context, natsURL, stream, subjectPrefix string, logger *log.Logger) (*JetStreamPublisher, error) {
	if stream == "" {
		stream = "LEDGERLITE"
	}
	if subjectPrefix == "" {
		subjectPrefix = "ledgerlite.events"
	}

	nc, err := nats.Connect(
		natsURL,
		nats.Name("ledgerlite-outbox-worker"),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(500*time.Millisecond),
	)
	if err != nil {
		return nil, fmt.Errorf("nats connect: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		_ = nc.Drain()
		nc.Close()
		return nil, fmt.Errorf("jetstream ctx: %w", err)
	}

	// Garante o Stream (idempotente)
	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      stream,
		Subjects:  []string{subjectPrefix + ".>"},
		Storage:   jetstream.FileStorage,
		Retention: jetstream.LimitsPolicy,
		// Janela de dedup (MsgID)
		Duplicates: 10 * time.Minute,
	})
	if err != nil {
		_ = nc.Drain()
		nc.Close()
		return nil, fmt.Errorf("create stream: %w", err)
	}

	return &JetStreamPublisher{
		log:           logger,
		nc:            nc,
		js:            js,
		stream:        stream,
		subjectPrefix: subjectPrefix,
	}, nil
}

func (p *JetStreamPublisher) Publish(ctx context.Context, event Event) error {
	subj := p.subjectPrefix + "." + sanitizeSubjectToken(event.EventType)

	body, err := json.Marshal(event) // publica envelope completo
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}

	// MsgID -> dedup do JetStream (evita duplicar evento se você republish por falha)
	_, err = p.js.Publish(ctx, subj, body,
		jetstream.WithMsgID(event.ID.String()),
		jetstream.WithExpectStream(p.stream),
	)
	if err != nil {
		return fmt.Errorf("js publish: %w", err)
	}

	if p.log != nil {
		p.log.Printf("jetstream published outbox_id=%s subject=%s", event.ID, subj)
	}
	return nil
}

func (p *JetStreamPublisher) Close() {
	if p == nil || p.nc == nil {
		return
	}
	_ = p.nc.Drain()
	p.nc.Close()
}

var invalidToken = regexp.MustCompile(`[^a-zA-Z0-9_-]`)

func sanitizeSubjectToken(s string) string {
	s = strings.TrimSpace(s)
	s = invalidToken.ReplaceAllString(s, "_")
	if s == "" {
		return "unknown"
	}
	return s
}
