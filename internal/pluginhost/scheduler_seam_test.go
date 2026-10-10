package pluginhost

import (
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"testing"
)

func TestInvalidPreferenceFailsClosed(t *testing.T) {
	for _, ids := range [][]string{{}, {"unknown"}} {
		r, ok, _ := normalizeSchedulerResponse(pluginapi.SchedulerPickResponse{Handled: true, EligibleAuthIDs: ids}, pluginapi.SchedulerPickRequest{PreferenceOnly: true, Candidates: []pluginapi.SchedulerAuthCandidate{{ID: "allowed"}}})
		if !ok || !r.Handled || !r.Reject {
			t.Fatalf("invalid preference became fallback: %+v valid=%v", r, ok)
		}
	}
}
