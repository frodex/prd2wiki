package api

import (
	"encoding/json"
	"net/http"
)

// writeJSON sets Content-Type to application/json, writes status, and encodes data.
func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
