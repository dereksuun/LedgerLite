package outbox

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/google/uuid"
)

type Event struct {
	ID            uuid.UUID       `json:"id"`
	AggregateType string          `json:"aggregate_type"`
	AggregateID   uuid.UUID       `json:"aggregate_id"`
	EventType     string          `json:"event_type"`
	Payload       json.RawMessage `json:"payload"`
	CreatedAt     time.Time       `json:"created_at"`
}

type Publisher interface {
	Publish(ctx context.Context, event Event) error
}

type LoggerPublisher struct {
	Logger *log.Logger
}

func (p *LoggerPublisher) Publish(ctx context.Context, event Event) error {
	_ = ctx
	if p == nil || p.Logger == nil {
		return nil
	}
	p.Logger.Printf(
		"outbox publish id=%s type=%s aggregate=%s aggregate_id=%s payload=%s",
		event.ID, event.EventType, event.AggregateType, event.AggregateID, string(event.Payload),
	)
	return nil
}
