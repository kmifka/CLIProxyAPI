package executor

import (
	"context"
	"encoding/json"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	cliproxyauth "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/auth"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"io"
	"net/http"
	"strings"
	"testing"
)

type countedHomeBody struct {
	reads     int
	remaining int
}

func (b *countedHomeBody) Read(p []byte) (int, error) {
	b.reads++
	if b.remaining == 0 {
		return 0, io.EOF
	}
	n := len(p)
	if n > b.remaining {
		n = b.remaining
	}
	b.remaining -= n
	return n, nil
}
func (*countedHomeBody) Close() error { return nil }
func TestHomeHTTPBodyBound(t *testing.T) {
	for _, declared := range []int64{-1, 65} {
		t.Run(string(rune(declared+100)), func(t *testing.T) {
			b := &countedHomeBody{remaining: 100000}
			r := &http.Request{Body: b, ContentLength: declared}
			_, err := readHomeUltrafastHTTPBody(r, 64)
			if err == nil {
				t.Fatal("over-limit accepted")
			}
			if declared > 64 && b.reads != 0 {
				t.Fatal("declared oversize read")
			}
			if declared < 0 && b.remaining != 100000-65 {
				t.Fatalf("unbounded read: %d", b.remaining)
			}
		})
	}
	for _, size := range []int{0, 63, 64} {
		r := &http.Request{Body: io.NopCloser(strings.NewReader(strings.Repeat("x", size))), ContentLength: -1}
		b, err := readHomeUltrafastHTTPBody(r, 64)
		if err != nil || len(b) != size {
			t.Fatalf("within cap: %d %v", len(b), err)
		}
	}
}
func TestHomeRefreshReplacementFailsClosed(t *testing.T) {
	cfg := &config.Config{}
	cfg.Home.Enabled = true
	e := NewCodexExecutor(cfg)
	old := codexOAuthTestAuth("https://example.invalid")
	raw, err := json.Marshal(old)
	if err != nil {
		t.Fatal(err)
	}
	var replaced cliproxyauth.Auth
	if err = json.Unmarshal(raw, &replaced); err != nil {
		t.Fatal(err)
	}
	replaced.Metadata["homeUltrafastValidated"] = true
	for _, a := range []*cliproxyauth.Auth{&replaced, replaced.Clone()} {
		req := cliproxyexecutor.Request{Model: "gpt-6.1-sol", Payload: []byte(`{"model":"gpt-6.1-sol","service_tier":"ultrafast"}`)}
		if _, err = e.prepareHomeUltrafast(a, req, cliproxyexecutor.Options{}); err == nil {
			t.Fatal("replacement reused entitlement")
		}
		req.Payload = []byte(`{"model":"gpt-6.1-sol"}`)
		if _, err = e.prepareHomeUltrafast(a, req, cliproxyexecutor.Options{}); err != nil {
			t.Fatal("ordinary replacement blocked", err)
		}
	}
	_ = context.Background()
}
