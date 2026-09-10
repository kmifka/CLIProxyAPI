package executor

import (
	"net/http"
	"testing"
)

func TestParseCodexRequestModelRecognisesAllThreeMarkers(t *testing.T) {
	priorityBody := []byte(`{"model":"gpt-5.5","service_tier":"priority"}`)
	plainBody := []byte(`{"model":"gpt-5.5"}`)
	fastHeader := http.Header{"X-Llm-Proxy-Codex-Fast": []string{"1"}}

	for _, tc := range []struct {
		name     string
		model    string
		headers  http.Header
		payload  []byte
		wantBase string
		wantFast bool
	}{
		{"front proxy header", "gpt-5.5", fastHeader, plainBody, "gpt-5.5", true},
		{"retired -fast alias is just a model name now", "gpt-5.5-fast", nil, plainBody, "gpt-5.5-fast", false},
		{"native service_tier", "gpt-5.5", nil, priorityBody, "gpt-5.5", true},
		{"native, mixed case", "gpt-5.5", nil, []byte(`{"service_tier":"Priority"}`), "gpt-5.5", true},
		{"plain request", "gpt-5.5", nil, plainBody, "gpt-5.5", false},
		{"other tier", "gpt-5.5", nil, []byte(`{"service_tier":"flex"}`), "gpt-5.5", false},
		{"empty payload", "gpt-5.5", nil, nil, "gpt-5.5", false},
		{"malformed payload", "gpt-5.5", nil, []byte(`not json`), "gpt-5.5", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, fast := parseCodexRequestModel(tc.model, tc.headers, tc.payload)
			if base != tc.wantBase {
				t.Errorf("base = %q, want %q", base, tc.wantBase)
			}
			if fast != tc.wantFast {
				t.Errorf("fast = %v, want %v", fast, tc.wantFast)
			}
		})
	}
}

// The routing decision must agree with the parse, or a request would be marked
// priority without being put on the websocket path that actually makes it fast.
func TestCodexFastRequestedAgreesWithParse(t *testing.T) {
	cases := []struct {
		model   string
		headers http.Header
		payload []byte
	}{
		{"gpt-5.5", http.Header{"X-Llm-Proxy-Codex-Fast": []string{"1"}}, nil},
		{"gpt-5.5", nil, []byte(`{"service_tier":"priority"}`)},
		{"gpt-5.5", nil, []byte(`{"service_tier":"default"}`)},
		{"gpt-5.5", nil, nil},
	}
	for _, c := range cases {
		_, fast := parseCodexRequestModel(c.model, c.headers, c.payload)
		routed := codexFastRequested(headerOrEmpty(c.headers), c.payload)
		if fast != routed {
			t.Errorf("model=%q payload=%s: parse says %v, routing says %v", c.model, c.payload, fast, routed)
		}
	}
}

func headerOrEmpty(h http.Header) http.Header {
	if h == nil {
		return http.Header{}
	}
	return h
}
