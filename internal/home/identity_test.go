package home

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestIdentityResponseDoesNotPromiseRouteScope(t *testing.T) {
	got, err := decodeIdentityResponse([]byte(`{"authenticated":true,"provider":"cluster-api-key","principal":"synthetic-client","metadata":{"source":"authorization"}}`))
	if err != nil || !got.MatchesPrincipal("synthetic-client") {
		t.Fatalf("identity: %#v %v", got, err)
	}
	for _, raw := range []string{`{}`, `null`, `{"authenticated":true}`, `{"authenticated":true,"principal":"x","error":{"type":"invalid_credential"}}`, `{"authenticated":false,"error":{"type":"internal_error"}}`} {
		if _, err := decodeIdentityResponse([]byte(raw)); err == nil {
			t.Fatalf("accepted malformed/failed identity: %s", raw)
		}
	}
}

func TestIdentityRPCUsesLegacyIdentityEnvelope(t *testing.T) {
	client, _ := newRedisCommandTestClient(t, func(args []string) string {
		if len(args) == 2 && strings.EqualFold(args[0], "RPOP") {
			var payload map[string]any
			if err := json.Unmarshal([]byte(args[1]), &payload); err != nil {
				t.Error(err)
			}
			if payload["type"] != "auth-validate" {
				t.Errorf("not identity command: %#v", payload)
			}
			for _, field := range []string{"method", "path", "capability", "model"} {
				if _, ok := payload[field]; ok {
					t.Errorf("false scope field %s", field)
				}
			}
			body := `{"authenticated":true,"provider":"cluster-api-key","principal":"synthetic-client"}`
			return fmt.Sprintf("$%d\r\n%s\r\n", len(body), body)
		}
		return "-ERR unexpected command\r\n"
	})
	identity, err := client.ValidateRequestIdentity(context.Background(), http.Header{"Authorization": {"Bearer synthetic-client"}})
	if err != nil || !identity.MatchesPrincipal("synthetic-client") {
		t.Fatalf("identity RPC: %#v %v", identity, err)
	}
}

func TestIdentityValidationUnavailableFailsClosed(t *testing.T) {
	var c *Client
	if _, err := c.ValidateRequestIdentity(context.Background(), http.Header{}); err == nil {
		t.Fatal("nil Home accepted")
	}
}
