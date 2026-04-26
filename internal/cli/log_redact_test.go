package cli

import (
	"bytes"
	"testing"
)

func TestRedactLine_BearerToken(t *testing.T) {
	in := []byte("Authorization: Bearer abc123def456")
	want := []byte("Authorization: Bearer [REDACTED]")

	got, count := RedactLine(in)

	if count != 1 {
		t.Fatalf("RedactLine count = %d, want 1", count)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("RedactLine got %q, want %q", got, want)
	}
}

func TestRedactLine_ApiKeyEqualsValue(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
		want []byte
	}{
		{
			name: "api key query value",
			in:   []byte("api_key=sk-prod-XYZ"),
			want: []byte("api_key=[REDACTED]"),
		},
		{
			name: "x api key header",
			in:   []byte("x-api-key: sk-test-abcdefghijklmnopqrstuvwxyz"),
			want: []byte("x-api-key: [REDACTED]"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, count := RedactLine(tt.in)
			if count != 1 {
				t.Fatalf("RedactLine count = %d, want 1", count)
			}
			if !bytes.Equal(got, tt.want) {
				t.Fatalf("RedactLine got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRedactLine_TelegramBotToken(t *testing.T) {
	in := []byte("telegram=12345:ABCDEFGHabcdefgh1234567890")
	want := []byte("telegram=[REDACTED]")

	got, count := RedactLine(in)

	if count != 1 {
		t.Fatalf("RedactLine count = %d, want 1", count)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("RedactLine got %q, want %q", got, want)
	}
}

func TestRedactLine_SlackTokens(t *testing.T) {
	tests := []struct {
		name string
		in   []byte
	}{
		{name: "bot", in: []byte("token=xoxb-123456789012-abcdefghijk")},
		{name: "user", in: []byte("token=xoxp-123456789012-abcdefghijk")},
		{name: "app", in: []byte("token=xoxs-123456789012-abcdefghijk")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, count := RedactLine(tt.in)
			want := []byte("token=[REDACTED]")
			if count != 1 {
				t.Fatalf("RedactLine count = %d, want 1", count)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("RedactLine got %q, want %q", got, want)
			}
		})
	}
}

func TestRedactLine_OpenAIStyleKey(t *testing.T) {
	in := []byte("short=sk-short long=sk-1234567890abcdefg")
	want := []byte("short=sk-short long=[REDACTED]")

	got, count := RedactLine(in)

	if count != 1 {
		t.Fatalf("RedactLine count = %d, want 1", count)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("RedactLine got %q, want %q", got, want)
	}
}

func TestRedactLine_NoMatchPreservesInput(t *testing.T) {
	in := []byte("INFO gateway: request completed status=200")

	got, count := RedactLine(in)

	if count != 0 {
		t.Fatalf("RedactLine count = %d, want 0", count)
	}
	if !bytes.Equal(got, in) {
		t.Fatalf("RedactLine got %q, want original %q", got, in)
	}
}
