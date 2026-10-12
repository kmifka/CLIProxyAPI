package api

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/home"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// A RESP fixture runs the real Home client; it is not real Home enrollment.
func realtimeIdentityFixture(t *testing.T) (*atomic.Int32, *atomic.Int32) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	var identity, dispatch atomic.Int32
	var wg sync.WaitGroup
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, e := ln.Accept()
			if e != nil {
				return
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer c.Close()
				rd := bufio.NewReader(c)
				for {
					line, e := rd.ReadString('\n')
					if e != nil {
						return
					}
					n, e := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "*")))
					if e != nil {
						return
					}
					args := make([]string, n)
					for k := range args {
						line, e = rd.ReadString('\n')
						if e != nil {
							return
						}
						size, e := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "$")))
						if e != nil {
							return
						}
						b := make([]byte, size+2)
						if _, e = io.ReadFull(rd, b); e != nil {
							return
						}
						args[k] = string(b[:size])
					}
					switch strings.ToLower(args[0]) {
					case "hello":
						io.WriteString(c, "%1\r\n$5\r\nproto\r\n:3\r\n")
					case "client":
						io.WriteString(c, "+OK\r\n")
					case "rpop":
						var p struct {
							Type    string            `json:"type"`
							Headers map[string]string `json:"headers"`
						}
						json.Unmarshal([]byte(args[1]), &p)
						if p.Type != "auth-validate" {
							dispatch.Add(1)
							io.WriteString(c, "-ERR dispatch forbidden\r\n")
							continue
						}
						identity.Add(1)
						body := `{"error":{"type":"invalid_credential"}}`
						if p.Headers["authorization"] == "Bearer fixture-principal" {
							body = `{"authenticated":true,"provider":"cluster-api-key","principal":"fixture-principal"}`
						}
						fmt.Fprintf(c, "$%d\r\n%s\r\n", len(body), body)
					default:
						io.WriteString(c, "-ERR unexpected command\r\n")
					}
				}
			}()
		}
	}()
	port := ln.Addr().(*net.TCPAddr).Port
	client := home.New(config.HomeConfig{Enabled: true, Host: "127.0.0.1", Port: port, DisableClusterDiscovery: true})
	previous := home.Current()
	home.SetCurrent(client)
	t.Cleanup(func() { home.SetCurrent(previous); client.Close(); ln.Close(); <-done; wg.Wait() })
	return &identity, &dispatch
}

func TestRealtimeNativeIdentity(t *testing.T) {
	count, dispatch := realtimeIdentityFixture(t)
	for _, path := range []string{"/v1/realtime", "/live/gemini"} {
		for _, credential := range []string{"fixture-principal", "invalid", ""} {
			r := httptest.NewRequest("GET", path+"?model=approved", nil)
			if credential != "" {
				r.Header.Set("Authorization", "Bearer "+credential)
			}
			phase := 0
			got, err := AuthenticateRegisteredRealtimeRequest(r, func(q *http.Request) error {
				if phase != 0 || q != r || q.URL.Query().Get("model") != "approved" {
					t.Fatal("policy order/request")
				}
				phase = 1
				return nil
			}, func(i *RegisteredRequestIdentity) error {
				if phase != 1 || !i.MatchesPrincipal("fixture-principal") || i.MatchesPrincipal("wrong") {
					t.Fatal("opaque identity binding")
				}
				phase = 2
				return nil
			})
			if credential == "fixture-principal" {
				if err != nil || got == nil || phase != 2 {
					t.Fatalf("native identity: %v", err)
				}
			} else {
				var e *RegisteredRequestError
				if !errors.As(err, &e) || e.Status != 401 || got != nil || phase != 1 {
					t.Fatalf("invalid native identity: %v phase=%d", err, phase)
				}
			}
		}
	}
	if count.Load() != 6 || dispatch.Load() != 0 {
		t.Fatalf("identity=%d inference RPOP=%d", count.Load(), dispatch.Load())
	}
}

func TestRealtimeRejectsBeforePolicy(t *testing.T) {
	count, dispatch := realtimeIdentityFixture(t)
	for _, tc := range []struct {
		method, target string
		status         int
	}{
		{"POST", "/v1/realtime?model=x", 403}, {"GET", "/unknown?model=x", 403},
		{"GET", "/v1/realtime", 400}, {"GET", "/v1/realtime?model=", 400}, {"GET", "/v1/realtime?model=x&model=y", 400},
		{"GET", "/v1/realtime?model=x&api_key=secret", 400}, {"GET", "/v1/realtime?model=%zz", 400}, {"GET", "/v1/realtime?model=x&unknown=y", 400},
		{"GET", "/v1/realtime?model=x#fragment", 403}, {"GET", "/v1/%72ealtime?model=x", 403},
	} {
		r := httptest.NewRequest(tc.method, tc.target, nil)
		_, err := AuthenticateRegisteredRealtimeRequest(r, func(*http.Request) error { t.Fatal("invalid request reached policy"); return nil }, func(*RegisteredRequestIdentity) error { t.Fatal("invalid request reached bind"); return nil })
		var e *RegisteredRequestError
		if !errors.As(err, &e) || e.Status != tc.status {
			t.Errorf("%s: %v", tc.target, err)
		}
	}
	if count.Load() != 0 || dispatch.Load() != 0 {
		t.Fatal("invalid requests reached Home")
	}
}

func TestRealtimePublishedAudioBoundary(t *testing.T) {
	for _, path := range []string{"/v1/realtime?model=fixture", "/live/gemini?model=fixture"} {
		called := false
		_, err := AuthenticateRegisteredRequest(httptest.NewRequest("GET", path, nil), func(*http.Request) error { called = true; return nil }, func(*RegisteredRequestIdentity) error { return nil })
		var e *RegisteredRequestError
		if !errors.As(err, &e) || e.Status != 403 || called {
			t.Fatalf("audio boundary changed: %v", err)
		}
	}
}

func TestRealtimeCanonicalPolicy(t *testing.T) {
	denied := errors.New("operator model denied")
	for _, path := range []string{"/v1/realtime", "/live/gemini"} {
		r := httptest.NewRequest("GET", path+"?model=fixture%2Fapproved", nil)
		called := false
		_, err := AuthenticateRegisteredRealtimeRequest(r, func(got *http.Request) error {
			called = true
			if got != r || got.Method != "GET" || got.URL.Path != path || got.URL.Query().Get("model") != "fixture/approved" {
				t.Fatal("original request lost")
			}
			return denied
		}, func(*RegisteredRequestIdentity) error { t.Fatal("binding before authorization"); return nil })
		if !called || err != denied {
			t.Fatalf("policy not reached: %v", err)
		}
	}
}
