package pluginapi

import (
	"context"
	"io"
	"net/http"
)

// InferenceRoute is optional metadata, never an unauthenticated resource route.
type InferenceRoute struct {
	Method       string `json:"method"`
	Path         string `json:"path"`
	AuthRequired bool   `json:"auth_required"`
}

// InferenceRequest contains host-sanitized transport data, not a principal or credential.
type InferenceRequest struct {
	Method        string      `json:"method"`
	Path          string      `json:"path"`
	Headers       http.Header `json:"headers"`
	ContentLength int64       `json:"content_length"`
}
type InferenceFrame struct {
	ID      string      `json:"id"`
	Data    []byte      `json:"data,omitempty"`
	Status  int         `json:"status,omitempty"`
	Headers http.Header `json:"headers,omitempty"`
	EOF     bool        `json:"eof,omitempty"`
}
type InferencePlugin interface {
	InferenceRoutes(context.Context) ([]InferenceRoute, error)
	ServeInference(context.Context, InferenceRequest, io.Reader, http.ResponseWriter) error
}

const InferenceChunkBytes = 32768
const InferenceMaxBody int64 = 128 << 20

func ValidInferenceRoute(r InferenceRoute) bool {
	if r.Method != "POST" || !r.AuthRequired {
		return false
	}
	switch r.Path {
	case "/v1/audio/transcriptions", "/v1/audio/translations", "/v1/audio/speech":
		return true
	}
	return false
}
