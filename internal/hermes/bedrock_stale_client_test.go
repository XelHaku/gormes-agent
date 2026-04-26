package hermes

import "testing"

func TestClassifyBedrockStaleError_TransportFailures(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "connection_closed", err: ErrBedrockConnectionClosed},
		{name: "protocol_error", err: ErrBedrockProtocolError},
		{name: "read_timeout", err: ErrBedrockReadTimeout},
		{name: "unexpected_eof", err: ErrBedrockUnexpectedEOF},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyBedrockStaleError(tt.err)
			if !got.Stale {
				t.Fatalf("ClassifyBedrockStaleError(%v).Stale = false, want true", tt.err)
			}
			if got.Status != BedrockStaleTransportStatus {
				t.Fatalf("Status = %q, want %q", got.Status, BedrockStaleTransportStatus)
			}
		})
	}
}

func TestClassifyBedrockStaleError_LibraryAssertionOnly(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		stale bool
	}{
		{
			name:  "urllib3_connectionpool",
			err:   BedrockRuntimeError{Kind: BedrockRuntimeErrorAssertion, SourcePackage: "urllib3.connectionpool"},
			stale: true,
		},
		{
			name:  "botocore_httpsession",
			err:   BedrockRuntimeError{Kind: BedrockRuntimeErrorAssertion, SourcePackage: "botocore.httpsession"},
			stale: true,
		},
		{
			name:  "application_assertion",
			err:   BedrockRuntimeError{Kind: BedrockRuntimeErrorAssertion, SourcePackage: "internal/hermes"},
			stale: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyBedrockStaleError(tt.err)
			if got.Stale != tt.stale {
				t.Fatalf("ClassifyBedrockStaleError(%v).Stale = %v, want %v", tt.err, got.Stale, tt.stale)
			}
		})
	}
}
