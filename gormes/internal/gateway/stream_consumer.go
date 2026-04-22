package gateway

import (
	"context"

	"github.com/TrebuchetDynamics/gormes-agent/gormes/internal/kernel"
)

// StreamDeliverySink is the minimal contract for replaying one render frame to
// one resolved destination.
type StreamDeliverySink interface {
	DeliverFrame(ctx context.Context, target DeliveryTarget, frame kernel.RenderFrame) error
}

// DeliveryResult captures the outcome for one attempted fan-out target.
type DeliveryResult struct {
	Target DeliveryTarget
	Err    error
}

// GatewayStreamConsumer fans one kernel frame out to one or more delivery
// targets in a deterministic order.
type GatewayStreamConsumer struct {
	sink StreamDeliverySink
}

func NewGatewayStreamConsumer(sink StreamDeliverySink) *GatewayStreamConsumer {
	return &GatewayStreamConsumer{sink: sink}
}

func (c *GatewayStreamConsumer) FanOut(ctx context.Context, frame kernel.RenderFrame, targets []DeliveryTarget) []DeliveryResult {
	results := make([]DeliveryResult, 0, len(targets))
	if c == nil || c.sink == nil {
		return results
	}
	for _, target := range targets {
		err := c.sink.DeliverFrame(ctx, target, frame)
		results = append(results, DeliveryResult{Target: target, Err: err})
	}
	return results
}
