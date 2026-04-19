package hermes

import (
	"errors"
	"testing"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want ErrorClass
	}{
		{"nil", nil, ClassUnknown},
		{"429", &HTTPError{Status: 429}, ClassRetryable},
		{"500", &HTTPError{Status: 500}, ClassRetryable},
		{"502", &HTTPError{Status: 502}, ClassRetryable},
		{"503", &HTTPError{Status: 503}, ClassRetryable},
		{"504", &HTTPError{Status: 504}, ClassRetryable},
		{"401", &HTTPError{Status: 401}, ClassFatal},
		{"403", &HTTPError{Status: 403}, ClassFatal},
		{"404", &HTTPError{Status: 404}, ClassFatal},
		{"context-length", &HTTPError{Status: 400, Body: "context length exceeded"}, ClassFatal},
		{"plain", errors.New("boom"), ClassUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Classify(c.err); got != c.want {
				t.Errorf("Classify = %v, want %v", got, c.want)
			}
		})
	}
}
