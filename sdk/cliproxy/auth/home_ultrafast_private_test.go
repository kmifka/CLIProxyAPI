package auth

import "testing"

func TestHomeUltrafastPrivateMarkerExactModel(t *testing.T) {
	a := &Auth{homeUltrafastValidated: true, homeUltrafastModel: "gpt-6.1-sol", Metadata: map[string]any{"homeUltrafastValidated": true}}
	if !a.Clone().HomeUltrafastValidatedFor("gpt-6.1-sol") || a.Clone().HomeUltrafastValidatedFor("gpt-other") {
		t.Fatal("private model binding lost or broadened")
	}
	spoof := &Auth{Metadata: a.Metadata, Attributes: map[string]string{"home_ultrafast_validated": "true"}}
	if spoof.HomeUltrafastValidatedFor("gpt-6.1-sol") {
		t.Fatal("caller metadata minted validation")
	}
	a.homeUltrafastValidated = false
	if a.HomeUltrafastValidatedFor("gpt-6.1-sol") {
		t.Fatal("false validation admitted")
	}
}
