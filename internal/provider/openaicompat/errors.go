package openaicompat

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/zenodea/zaino/internal/x/httpx"
)

type APIError struct {
	Provider   string
	StatusCode int
	Type       string
	Code       string
	Message    string
	RequestID  string
	Body       []byte

	retryAfter time.Duration
}

func (e *APIError) RetryAfter() time.Duration { return e.retryAfter }

func (e *APIError) Retryable() bool { return httpx.StatusRetryable(e.StatusCode) }

func (e *APIError) Error() string {
	kind := e.Type
	if e.Code != "" {
		kind = e.Code
	}
	switch {
	case e.StatusCode == 0:
		return fmt.Sprintf("%s: %s: %s", e.Provider, kind, e.Message)
	case e.RequestID != "":
		return fmt.Sprintf("%s: %d %s: %s (request-id %s)",
			e.Provider, e.StatusCode, kind, e.Message, e.RequestID)
	default:
		return fmt.Sprintf("%s: %d %s: %s", e.Provider, e.StatusCode, kind, e.Message)
	}
}

func parseAPIError(provider string, status int, requestID string, body []byte) *APIError {
	e := &APIError{Provider: provider, StatusCode: status, RequestID: requestID, Body: body}

	var envelope struct {
		Error struct {
			Type    string `json:"type"`
			Code    any    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Error.Message != "" {
		e.Type = envelope.Error.Type
		e.Message = envelope.Error.Message
		if code, ok := envelope.Error.Code.(string); ok {
			e.Code = code
		}
		if e.Type == "" && e.Code == "" {
			e.Type = "unknown_error"
		}
		return e
	}

	e.Type = "unknown_error"
	e.Message = string(body)
	if e.Message == "" {
		e.Message = http.StatusText(status)
	}
	return e
}
