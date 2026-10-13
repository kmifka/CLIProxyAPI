package pluginhost

import (
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"testing"
)

func TestConfiguredDelegateRequiresBoundedMembership(t *testing.T) {
	req := pluginapi.SchedulerPickRequest{Candidates: []pluginapi.SchedulerAuthCandidate{{ID: "a"}, {ID: "b"}}}
	for _, tc := range []struct {
		name  string
		ids   []string
		valid bool
	}{{"bounded", []string{"b"}, true}, {"empty", nil, false}, {"outside", []string{"missing"}, false}} {
		t.Run(tc.name, func(t *testing.T) {
			_, valid, _ := normalizeSchedulerResponse(pluginapi.SchedulerPickResponse{Handled: true, DelegateBuiltin: "configured", EligibleAuthIDs: tc.ids}, req)
			if valid != tc.valid {
				t.Fatalf("valid=%v want %v", valid, tc.valid)
			}
		})
	}
}
