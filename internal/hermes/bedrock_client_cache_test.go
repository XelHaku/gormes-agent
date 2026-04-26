package hermes

import (
	"context"
	"fmt"
	"testing"
)

type fakeBedrockRuntimeClient struct {
	name          string
	converseErr   error
	streamErr     error
	converseCalls int
	streamCalls   int
}

func (f *fakeBedrockRuntimeClient) Converse(context.Context, BedrockRuntimeRequest) (BedrockRuntimeResponse, error) {
	f.converseCalls++
	if f.converseErr != nil {
		return BedrockRuntimeResponse{}, f.converseErr
	}
	return BedrockRuntimeResponse{}, nil
}

func (f *fakeBedrockRuntimeClient) ConverseStream(context.Context, BedrockRuntimeRequest) (BedrockRuntimeStreamResponse, error) {
	f.streamCalls++
	if f.streamErr != nil {
		return BedrockRuntimeStreamResponse{}, f.streamErr
	}
	return BedrockRuntimeStreamResponse{}, nil
}

func TestBedrockRuntimeCache_EvictsOnlyTargetRegion(t *testing.T) {
	cache := NewBedrockRuntimeCache(nil)
	east := &fakeBedrockRuntimeClient{name: "east"}
	west := &fakeBedrockRuntimeClient{name: "west"}
	cache.Put("us-east-1", east)
	cache.Put("us-west-2", west)

	if evicted := cache.InvalidateRuntimeClient("us-east-1"); !evicted {
		t.Fatal("InvalidateRuntimeClient(us-east-1) = false, want true")
	}
	if _, ok := cache.Get("us-east-1"); ok {
		t.Fatal("us-east-1 client remained cached after eviction")
	}
	gotWest, ok := cache.Get("us-west-2")
	if !ok {
		t.Fatal("us-west-2 client was evicted with us-east-1")
	}
	if gotWest != west {
		t.Fatalf("us-west-2 client = %p, want %p", gotWest, west)
	}
}

func TestBedrockRuntimeCache_DoesNotEvictValidationOrAuthFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "validation", err: BedrockRuntimeError{Kind: BedrockRuntimeErrorValidation, Message: "ValidationException: bad request"}},
		{name: "auth", err: BedrockRuntimeError{Kind: BedrockRuntimeErrorAuth, Message: "AccessDeniedException: forbidden"}},
		{name: "missing_credentials", err: BedrockRuntimeError{Kind: BedrockRuntimeErrorMissingCredentials, Message: "bedrock credentials missing"}},
		{name: "malformed_request", err: BedrockRuntimeError{Kind: BedrockRuntimeErrorMalformedRequest, Message: "malformed request: unexpected EOF in JSON"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewBedrockRuntimeCache(nil)
			client := &fakeBedrockRuntimeClient{name: tt.name}
			cache.Put("us-east-1", client)

			if evicted := cache.EvictOnStaleError("us-east-1", tt.err); evicted {
				t.Fatalf("EvictOnStaleError(%s) = true, want false", tt.name)
			}
			got, ok := cache.Get("us-east-1")
			if !ok {
				t.Fatalf("%s error evicted the cached client", tt.name)
			}
			if got != client {
				t.Fatalf("cached client = %p, want %p", got, client)
			}
			classification := ClassifyBedrockStaleError(tt.err)
			if classification.Stale {
				t.Fatalf("ClassifyBedrockStaleError(%s).Stale = true, want false", tt.name)
			}
			if classification.Status != BedrockNonRetryableRequestStatus {
				t.Fatalf("Status = %q, want %q", classification.Status, BedrockNonRetryableRequestStatus)
			}
		})
	}
}

func TestBedrockRuntimeCache_EvictsOnConverseAndConverseStreamStaleFailures(t *testing.T) {
	tests := []struct {
		name      string
		staleErr  error
		call      func(context.Context, *BedrockRuntimeCache, string, BedrockRuntimeRequest) error
		callCount func(*fakeBedrockRuntimeClient) int
	}{
		{
			name:     "converse",
			staleErr: fmt.Errorf("converse failed: %w", ErrBedrockConnectionClosed),
			call: func(ctx context.Context, cache *BedrockRuntimeCache, region string, req BedrockRuntimeRequest) error {
				_, err := cache.Converse(ctx, region, req)
				return err
			},
			callCount: func(client *fakeBedrockRuntimeClient) int { return client.converseCalls },
		},
		{
			name:     "converse_stream",
			staleErr: fmt.Errorf("converse stream failed: %w", ErrBedrockReadTimeout),
			call: func(ctx context.Context, cache *BedrockRuntimeCache, region string, req BedrockRuntimeRequest) error {
				_, err := cache.ConverseStream(ctx, region, req)
				return err
			},
			callCount: func(client *fakeBedrockRuntimeClient) int { return client.streamCalls },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewBedrockRuntimeCache(nil)
			client := &fakeBedrockRuntimeClient{name: tt.name}
			switch tt.name {
			case "converse":
				client.converseErr = tt.staleErr
			case "converse_stream":
				client.streamErr = tt.staleErr
			}
			west := &fakeBedrockRuntimeClient{name: "west"}
			cache.Put("us-east-1", client)
			cache.Put("us-west-2", west)

			err := tt.call(context.Background(), cache, "us-east-1", BedrockRuntimeRequest{Model: "anthropic.claude-3-sonnet-20240229-v1:0"})

			if err != tt.staleErr {
				t.Fatalf("returned error = %v, want original %v", err, tt.staleErr)
			}
			if tt.callCount(client) != 1 {
				t.Fatalf("client call count = %d, want 1", tt.callCount(client))
			}
			if _, ok := cache.Get("us-east-1"); ok {
				t.Fatalf("%s stale error did not evict us-east-1", tt.name)
			}
			gotWest, ok := cache.Get("us-west-2")
			if !ok {
				t.Fatalf("%s stale error evicted us-west-2", tt.name)
			}
			if gotWest != west {
				t.Fatalf("us-west-2 client = %p, want %p", gotWest, west)
			}
		})
	}
}
