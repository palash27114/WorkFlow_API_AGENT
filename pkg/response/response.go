package response

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse is the standard API error body. Code helps clients identify error type.
type ErrorResponse struct {
	Error string `json:"error"`
	Code  string `json:"code,omitempty"`
}

// JSON writes a JSON response with the provided status code.
func JSON(w http.ResponseWriter, status int, payload interface{}) {
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
}

// Error writes a standardized error response with only a message (no code).
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, ErrorResponse{Error: msg})
}

// ErrorWithCode writes a standardized error response with message and code for the viewer to understand what went wrong.
func ErrorWithCode(w http.ResponseWriter, status int, code, msg string) {
	JSON(w, status, ErrorResponse{Error: msg, Code: code})
}

// Error codes returned in API responses so the viewer can identify what went wrong.
const (
	CodeInvalidRequest          = "INVALID_REQUEST"
	CodeValidationError         = "VALIDATION_ERROR"
	CodeNotFound                = "NOT_FOUND"
	CodeCircularDependency      = "CIRCULAR_DEPENDENCY"
	CodeDependencyAlreadyExists = "DEPENDENCY_ALREADY_EXISTS"
	CodeInvalidStatusTransition = "INVALID_STATUS_TRANSITION"
	CodeBlockedByDependencies   = "BLOCKED_BY_DEPENDENCIES"
	CodeDeadlineConstraint      = "DEADLINE_CONSTRAINT"
	CodeTaskNotInProject        = "TASK_NOT_IN_PROJECT"
	CodeInternalError           = "INTERNAL_ERROR"
	CodeEmptyResult             = "EMPTY_RESULT"
)

