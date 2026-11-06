package domain

import (
	"fmt"
	"net/http"
)

// ProblemDetails represents an RFC 9457 compliant error response.
type ProblemDetails struct {
	Type          string         `json:"type"`
	Title         string         `json:"title"`
	Status        int            `json:"status"`
	Detail        string         `json:"detail,omitempty"`
	Code          string         `json:"code,omitempty"`
	Instance      string         `json:"instance,omitempty"`
	RequestID     string         `json:"request_id,omitempty"`
	InvalidParams []InvalidParam `json:"invalid_params,omitempty"`
}

// NewProblem creates a new ProblemDetails object with defaults based on status code.
func NewProblem(status int, detail string, code string, instance string, requestID string) ProblemDetails {
	return ProblemDetails{
		Type:      "about:blank",
		Title:     http.StatusText(status),
		Status:    status,
		Detail:    detail,
		Code:      code,
		Instance:  instance,
		RequestID: requestID,
	}
}

// NewValidationProblem creates a ProblemDetails object for validation errors.
func NewValidationProblem(detail string, params []InvalidParam, instance string, requestID string) ProblemDetails {
	prob := NewProblem(http.StatusBadRequest, detail, "VALIDATION_FAILED", instance, requestID)
	prob.Type = "https://datatracker.ietf.org/doc/html/rfc9457"
	prob.InvalidParams = params
	if prob.Detail == "" {
		prob.Detail = fmt.Sprintf("%d validation error(s) occurred", len(params))
	}
	return prob
}
