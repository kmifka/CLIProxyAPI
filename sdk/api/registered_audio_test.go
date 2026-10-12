package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRegisteredAudioRequestRejectsWithoutPolicy(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", nil)
	if _, err := AuthenticateRegisteredRequest(r, nil, nil); err == nil {
		t.Fatal("absent gateway policy accepted")
	}
}

func TestRegisteredAudioRequestRejectsNonAudio(t *testing.T) {
	for _, target := range []string{"/v1/chat/completions", "/v1/audio/transcriptions/", "/v1/audio/unknown"} {
		r := httptest.NewRequest(http.MethodPost, target, nil)
		called := false
		_, err := AuthenticateRegisteredRequest(r, func(*http.Request) error { called = true; return nil }, func(*RegisteredRequestIdentity) error { return nil })
		if err == nil || called {
			t.Fatalf("unregistered route accepted: %s", target)
		}
	}
	r := httptest.NewRequest(http.MethodGet, "/v1/audio/speech", nil)
	if _, err := AuthenticateRegisteredRequest(r, nil, nil); err == nil {
		t.Fatal("GET audio accepted")
	}
}
