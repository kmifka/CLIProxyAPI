package pluginhost

import (
	"encoding/json"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"testing"
)

func TestSchedulerPreferenceRPCContract(t *testing.T) {
	caps := rpcCapabilitiesFromPlugin(pluginapi.Plugin{Capabilities: pluginapi.Capabilities{Scheduler: schedulerFunc(nil), SchedulerPreference: true}})
	if !caps.SchedulerPreference {
		t.Fatal("registration lost preference capability")
	}
	for _, name := range []string{"EligibleAuthIDs", "eligible_auth_ids"} {
		var r pluginapi.SchedulerPickResponse
		if err := json.Unmarshal([]byte(`{"Handled":true,"`+name+`":["a"]}`), &r); err != nil {
			t.Fatal(err)
		}
		req := pluginapi.SchedulerPickRequest{PreferenceOnly: true, Candidates: []pluginapi.SchedulerAuthCandidate{{ID: "a"}}}
		if _, ok, _ := normalizeSchedulerResponse(r, req); !ok {
			t.Fatalf("valid subset rejected: %+v", r)
		}
		r.EligibleAuthIDs = []string{"absent"}
		if _, ok, _ := normalizeSchedulerResponse(r, req); ok {
			t.Fatal("invented membership accepted")
		}
		r.EligibleAuthIDs = nil
		if _, ok, _ := normalizeSchedulerResponse(r, req); ok {
			t.Fatal("empty subset accepted")
		}
	}
}
