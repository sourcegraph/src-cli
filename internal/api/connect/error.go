package connect

import (
	"encoding/json"
	"fmt"
)

// Error is a Connect RPC error returned for a non-2xx response. It carries the
// HTTP status line plus, when present, the structured Connect error code and
// message from the JSON body. Callers may use errors.As to inspect Code and
// Message for branching.
type Error struct {
	// Status is the HTTP status line (e.g. "400 Bad Request").
	Status string
	// Code is the Connect error code from the response body, if any (e.g.
	// "invalid_argument").
	Code string
	// Message is the Connect error message from the response body, if any.
	Message string
	// Body is the raw response body, used when the body could not be parsed as
	// a Connect error envelope.
	Body []byte
}

func (e *Error) Error() string {
	if e.Code == "" && e.Message == "" {
		return fmt.Sprintf("error: %s\n\n%s", e.Status, e.Body)
	}
	if e.Code == "" {
		return fmt.Sprintf("error: %s\n\n%s", e.Status, e.Message)
	}
	if e.Message == "" {
		return fmt.Sprintf("error: %s\n\n%s", e.Status, e.Code)
	}
	return fmt.Sprintf("error: %s\n\n%s: %s", e.Status, e.Code, e.Message)
}

// parseError builds an *Error from an HTTP status and response body. If the
// body is a Connect error envelope, Code and Message are populated; otherwise
// only Body is set.
func parseError(status string, body []byte) *Error {
	var envelope struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &envelope); err == nil && (envelope.Code != "" || envelope.Message != "") {
		return &Error{
			Status:  status,
			Code:    envelope.Code,
			Message: envelope.Message,
		}
	}
	return &Error{
		Status: status,
		Body:   body,
	}
}
