package hermes

import (
	"context"
	"errors"
	"sync"
)

var ErrBedrockRuntimeClientMissing = errors.New("bedrock runtime client missing")

type BedrockRuntimeRequest struct {
	Model string
}

type BedrockRuntimeResponse struct{}

type BedrockRuntimeStreamResponse struct{}

type BedrockRuntimeClient interface {
	Converse(context.Context, BedrockRuntimeRequest) (BedrockRuntimeResponse, error)
	ConverseStream(context.Context, BedrockRuntimeRequest) (BedrockRuntimeStreamResponse, error)
}

type BedrockRuntimeClientFactory func(region string) (BedrockRuntimeClient, error)

type BedrockRuntimeCache struct {
	mu        sync.Mutex
	clients   map[string]BedrockRuntimeClient
	newClient BedrockRuntimeClientFactory
}

func NewBedrockRuntimeCache(newClient BedrockRuntimeClientFactory) *BedrockRuntimeCache {
	return &BedrockRuntimeCache{
		clients:   make(map[string]BedrockRuntimeClient),
		newClient: newClient,
	}
}

func (c *BedrockRuntimeCache) Put(region string, client BedrockRuntimeClient) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clients[region] = client
}

func (c *BedrockRuntimeCache) Get(region string) (BedrockRuntimeClient, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	client, ok := c.clients[region]
	return client, ok
}

func (c *BedrockRuntimeCache) InvalidateRuntimeClient(region string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	_, ok := c.clients[region]
	delete(c.clients, region)
	return ok
}

func (c *BedrockRuntimeCache) EvictOnStaleError(region string, err error) bool {
	if !ClassifyBedrockStaleError(err).Stale {
		return false
	}
	return c.InvalidateRuntimeClient(region)
}

func (c *BedrockRuntimeCache) Converse(ctx context.Context, region string, req BedrockRuntimeRequest) (BedrockRuntimeResponse, error) {
	client, err := c.runtimeClient(region)
	if err != nil {
		return BedrockRuntimeResponse{}, err
	}
	out, err := client.Converse(ctx, req)
	if err != nil {
		c.EvictOnStaleError(region, err)
	}
	return out, err
}

func (c *BedrockRuntimeCache) ConverseStream(ctx context.Context, region string, req BedrockRuntimeRequest) (BedrockRuntimeStreamResponse, error) {
	client, err := c.runtimeClient(region)
	if err != nil {
		return BedrockRuntimeStreamResponse{}, err
	}
	out, err := client.ConverseStream(ctx, req)
	if err != nil {
		c.EvictOnStaleError(region, err)
	}
	return out, err
}

func (c *BedrockRuntimeCache) runtimeClient(region string) (BedrockRuntimeClient, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if client, ok := c.clients[region]; ok {
		return client, nil
	}
	if c.newClient == nil {
		return nil, ErrBedrockRuntimeClientMissing
	}
	client, err := c.newClient(region)
	if err != nil {
		return nil, err
	}
	c.clients[region] = client
	return client, nil
}
