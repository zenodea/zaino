package anthropic

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/zenodea/zaino/internal/x/httpx"
)

type APIError struct {
	StatusCode int
	Type       string
	Message    string
	RequestID  string
	Body       []byte

	retryAfter time.Duration
}

func (e *APIError) RetryAfter() time.Duration { return e.retryAfter }

func (e *APIError) Error() string {
	switch {
	case e.StatusCode == 0:
		return fmt.Sprintf("anthropic: %s: %s", e.Type, e.Message)
	case e.RequestID != "":
		return fmt.Sprintf("anthropic: %d %s: %s (request-id %s)",
			e.StatusCode, e.Type, e.Message, e.RequestID)
	default:
		return fmt.Sprintf("anthropic: %d %s: %s", e.StatusCode, e.Type, e.Message)
	}
}

func (e *APIError) Retryable() bool { return httpx.StatusRetryable(e.StatusCode) }

func parseAPIError(status int, requestID string, body []byte) *APIError {
	e := &APIError{StatusCode: status, RequestID: requestID, Body: body}

	var envelope struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Error.Type != "" {
		e.Type = envelope.Error.Type
		e.Message = envelope.Error.Message
		return e
	}

	e.Type = "unknown_error"
	e.Message = string(body)
	if e.Message == "" {
		e.Message = http.StatusText(status)
	}
	return e
}
