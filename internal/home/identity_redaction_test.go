package home

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestIdentityNeverSerializesPrincipal(t *testing.T) {
	i, e := decodeIdentityResponse([]byte(`{"authenticated":true,"provider":"cluster","principal":"secret-principal","metadata":{"token":"secret-metadata"}}`))
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(i)
	for _, s := range []string{string(b), fmt.Sprintf("%v", i), fmt.Sprintf("%+v", i), fmt.Sprintf("%#v", i)} {
		if strings.Contains(s, "secret-") {
			t.Fatal("identity leaked")
		}
	}
	if i.MatchesPrincipal("wrong") || !i.MatchesPrincipal("secret-principal") {
		t.Fatal("binding failure")
	}
}
