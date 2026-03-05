package response

import (
	"encoding/json"
	"net/http"
)

type ErrorResponse struct {
	Error string `json:"error"`
}

// JSON writes a JSON response with the provided status code.
func JSON(w http.ResponseWriter, status int, payload interface{}) {
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
}

// Error writes a standardized error response body.
func Error(w http.ResponseWriter, status int, msg string) {
	JSON(w, status, ErrorResponse{Error: msg})
}

