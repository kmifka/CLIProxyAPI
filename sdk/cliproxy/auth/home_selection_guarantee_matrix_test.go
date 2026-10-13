package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	internalconfig "github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	cliproxyexecutor "github.com/router-for-me/CLIProxyAPI/v8/sdk/cliproxy/executor"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestHomeUltrafastNativeEnvelopeValidation(t *testing.T) {
	var posts atomic.Int32
	var connections atomic.Int32
	provider := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { posts.Add(1); w.Write([]byte(`{}`)) }))
	provider.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			connections.Add(1)
		}
	}
	provider.StartTLS()
	defer provider.Close()
	for _, mode := range []string{"valid", "model", "tier", "index", "digest", "wire-digest", "schema", "unknown", "lifetime", "version", "metadata-spoof", "ordinary"} {
		t.Run(mode, func(t *testing.T) {
			sum := sha256.Sum256([]byte(`{"access_token":"synthetic-only"}`))
			digest := hex.EncodeToString(sum[:])
			b := map[string]any{"auth_index": "canonical", "registration_lifetime": "durable-life", "state_version": int64(7), "metadata_digest": digest, "wire_metadata_digest": digest}
			g := map[string]any{"schema": 1, "requested_model": "gpt", "requested_tier": "ultrafast"}
			for k, v := range b {
				g[k] = v
			}
			switch mode {
			case "model":
				g["requested_model"] = "other"
			case "tier":
				g["requested_tier"] = "priority"
			case "index":
				g["auth_index"] = "other"
			case "digest":
				g["metadata_digest"] = "bad"
			case "wire-digest":
				b["wire_metadata_digest"] = digest[:62] + "00"
				g["wire_metadata_digest"] = b["wire_metadata_digest"]
			case "schema":
				g["schema"] = 2
			case "unknown":
				g["authority_enabled"] = true
			case "lifetime":
				g["registration_lifetime"] = "replacement"
			case "version":
				g["state_version"] = 8
			}
			metadata := map[string]any{"access_token": "synthetic-only"}
			body := map[string]any{"model": "gpt", "provider": "codex", "auth_index": "canonical", "auth": map[string]any{"id": "fixture", "provider": "codex", "metadata": metadata}, "selection_guarantee": g, "selection_binding": b}
			if mode == "metadata-spoof" {
				delete(body, "selection_guarantee")
				metadata["selection_guarantee"] = g
				metadata["homeUltrafastValidated"] = true
			}
			if mode == "ordinary" {
				delete(body, "selection_guarantee")
				delete(body, "selection_binding")
			}
			wire, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			m := newHomeSelectionTestManager(t, &fixtureHomeDispatcher{payload: wire})
			m.RegisterExecutor(guaranteeHTTPExecutor{schedulerTestExecutor: schedulerTestExecutor{provider: "codex"}, client: provider.Client(), url: provider.URL})
			m.SetConfig(&internalconfig.Config{Home: internalconfig.HomeConfig{Enabled: true}})
			opts := cliproxyexecutor.Options{}
			if mode != "ordinary" {
				opts.Metadata = map[string]any{cliproxyexecutor.ServiceTierMetadataKey: "ultrafast"}
			}
			s, err := m.pickHomeDispatchSelection(context.Background(), "gpt", opts)
			if mode == "valid" || mode == "ordinary" {
				if err != nil || s == nil {
					t.Fatalf("valid native selection: %v", err)
				}
				defer s.End("test")
				if s.CloneAuth().HomeUltrafastValidated() != (mode == "valid") {
					t.Fatal("wrong request-local validation state")
				}
			} else if s != nil || err == nil {
				if s != nil {
					s.End("test")
				}
				t.Fatal("inconsistent guarantee admitted")
			}
			before := posts.Load()
			beforeConnections := connections.Load()
			_, execErr := m.Execute(context.Background(), []string{"codex"}, cliproxyexecutor.Request{Model: "gpt"}, opts)
			if mode == "valid" || mode == "ordinary" {
				if execErr != nil || posts.Load() != before+1 {
					t.Fatalf("HTTP positive control: %v", execErr)
				}
			} else if execErr == nil || posts.Load() != before || connections.Load() != beforeConnections {
				t.Fatal("invalid guarantee reached provider")
			}
		})
	}
}

type guaranteeHTTPExecutor struct {
	schedulerTestExecutor
	client *http.Client
	url    string
}

func (e guaranteeHTTPExecutor) Execute(ctx context.Context, a *Auth, r cliproxyexecutor.Request, o cliproxyexecutor.Options) (cliproxyexecutor.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "POST", e.url, nil)
	if err != nil {
		return cliproxyexecutor.Response{}, err
	}
	rsp, err := e.client.Do(req)
	if err != nil {
		return cliproxyexecutor.Response{}, err
	}
	rsp.Body.Close()
	return cliproxyexecutor.Response{}, nil
}
