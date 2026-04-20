package web

import (
	"fmt"
	"net/http"
)

// ErrorData holds data for the error page template.
type ErrorData struct {
	Code    int
	Message string
}

// renderError renders a styled error page for user-facing requests.
func (h *Handler) renderError(w http.ResponseWriter, code int, message string) {
	w.WriteHeader(code)
	data := PageData{
		Title:   fmt.Sprintf("%d", code),
		Content: ErrorData{Code: code, Message: message},
	}
	h.preparePageData(&data)
	t := h.templates["templates/error.html"]
	if t != nil {
		t.ExecuteTemplate(w, "layout", data)
	} else {
		http.Error(w, message, code)
	}
}
