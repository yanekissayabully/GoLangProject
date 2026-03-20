package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/drivebai/backend/internal/models"
)

type ErrorResponse struct {
	Error *models.APIError `json:"error"`
}

type SuccessResponse struct {
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
}

func WriteJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func WriteError(w http.ResponseWriter, status int, apiErr *models.APIError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{Error: apiErr})
}

func WriteSuccess(w http.ResponseWriter, status int, message string, data interface{}) {
	response := SuccessResponse{
		Message: message,
		Data:    data,
	}
	WriteJSON(w, status, response)
}

func DecodeJSON(r *http.Request, v interface{}) error {
	return json.NewDecoder(r.Body).Decode(v)
}
