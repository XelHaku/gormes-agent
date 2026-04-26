package hermes

import (
	"errors"
	"strings"
)

const (
	BedrockStaleTransportStatus      = "bedrock_stale_transport"
	BedrockNonRetryableRequestStatus = "bedrock_non_retryable_request_failure"
)

var (
	ErrBedrockConnectionClosed = errors.New("connection closed")
	ErrBedrockProtocolError    = errors.New("protocol error")
	ErrBedrockReadTimeout      = errors.New("read timeout")
	ErrBedrockUnexpectedEOF    = errors.New("unexpected EOF")
)

type BedrockRuntimeErrorKind string

const (
	BedrockRuntimeErrorAssertion          BedrockRuntimeErrorKind = "assertion"
	BedrockRuntimeErrorValidation         BedrockRuntimeErrorKind = "validation"
	BedrockRuntimeErrorAuth               BedrockRuntimeErrorKind = "auth"
	BedrockRuntimeErrorMissingCredentials BedrockRuntimeErrorKind = "missing_credentials"
	BedrockRuntimeErrorMalformedRequest   BedrockRuntimeErrorKind = "malformed_request"
)

type BedrockRuntimeError struct {
	Kind          BedrockRuntimeErrorKind
	Message       string
	SourcePackage string
}

func (e BedrockRuntimeError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return string(e.Kind)
}

type BedrockStaleErrorClassification struct {
	Stale  bool
	Status string
}

func ClassifyBedrockStaleError(err error) BedrockStaleErrorClassification {
	if err == nil {
		return BedrockStaleErrorClassification{}
	}
	if errors.Is(err, ErrBedrockConnectionClosed) ||
		errors.Is(err, ErrBedrockProtocolError) ||
		errors.Is(err, ErrBedrockReadTimeout) ||
		errors.Is(err, ErrBedrockUnexpectedEOF) {
		return bedrockStaleTransportClassification()
	}
	var runtimeErr BedrockRuntimeError
	if errors.As(err, &runtimeErr) &&
		runtimeErr.Kind == BedrockRuntimeErrorAssertion &&
		bedrockStaleLibrarySource(runtimeErr.SourcePackage) {
		return bedrockStaleTransportClassification()
	}
	if errors.As(err, &runtimeErr) && bedrockNonRetryableRequestError(runtimeErr.Kind) {
		return BedrockStaleErrorClassification{
			Status: BedrockNonRetryableRequestStatus,
		}
	}
	return BedrockStaleErrorClassification{}
}

func bedrockStaleTransportClassification() BedrockStaleErrorClassification {
	return BedrockStaleErrorClassification{
		Stale:  true,
		Status: BedrockStaleTransportStatus,
	}
}

func bedrockStaleLibrarySource(sourcePackage string) bool {
	return strings.HasPrefix(sourcePackage, "urllib3.") ||
		strings.HasPrefix(sourcePackage, "botocore.") ||
		strings.HasPrefix(sourcePackage, "boto3.")
}

func bedrockNonRetryableRequestError(kind BedrockRuntimeErrorKind) bool {
	switch kind {
	case BedrockRuntimeErrorValidation,
		BedrockRuntimeErrorAuth,
		BedrockRuntimeErrorMissingCredentials,
		BedrockRuntimeErrorMalformedRequest:
		return true
	default:
		return false
	}
}
