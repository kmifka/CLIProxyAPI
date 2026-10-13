package auth

import (
	"encoding/json"
	"testing"
)

func TestHomeUltrafastReplacementNeverInheritsPrivateMarker(t *testing.T) {
	selected := &Auth{Provider: "codex", homeUltrafastValidated: true, homeUltrafastModel: "gpt-6.1-sol", Metadata: map[string]any{"access_token": "synthetic-v1"}}
	if !selected.Clone().HomeUltrafastValidatedFor("gpt-6.1-sol") || selected.Clone().HomeUltrafastValidatedFor("other") {
		t.Fatal("request-local clone/model binding broken")
	}
	raw, err := json.Marshal(selected)
	if err != nil {
		t.Fatal(err)
	}
	var replacement Auth
	if err = json.Unmarshal(raw, &replacement); err != nil {
		t.Fatal(err)
	}
	replacement.Metadata["access_token"] = "synthetic-v2"
	replacement.Metadata["homeUltrafastValidated"] = true
	if replacement.HomeUltrafastValidated() || replacement.Clone().HomeUltrafastValidated() {
		t.Fatal("refresh JSON replacement retained stale private entitlement")
	}
	if !selected.HomeUltrafastValidatedFor("gpt-6.1-sol") {
		t.Fatal("replacement mutated inflight snapshot")
	}
}
