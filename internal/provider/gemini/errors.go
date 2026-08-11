package gemini

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/zenodea/zaino/internal/x/httpx"
)

type APIError struct {
	StatusCode int
	Status     string
	Message    string
	Body       []byte

	retryAfter time.Duration
}

func (e *APIError) Error() string {
	if e.Status != "" {
		return fmt.Sprintf("gemini: %d %s: %s", e.StatusCode, e.Status, e.Message)
	}
	return fmt.Sprintf("gemini: %d: %s", e.StatusCode, e.Message)
}

func (e *APIError) Retryable() bool { return httpx.StatusRetryable(e.StatusCode) }

func (e *APIError) RetryAfter() time.Duration { return e.retryAfter }

func parseAPIError(status int, body []byte) *APIError {
	e := &APIError{StatusCode: status, Body: body}

	var envelope struct {
		Error struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
			Status  string `json:"status"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && envelope.Error.Message != "" {
		e.Status = envelope.Error.Status
		e.Message = envelope.Error.Message
		return e
	}

	e.Message = string(body)
	if e.Message == "" {
		e.Message = http.StatusText(status)
	}
	return e
}
