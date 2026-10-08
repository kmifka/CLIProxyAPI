package pluginhost

import (
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"testing"
)

func TestPreferenceRejectAccepted(t *testing.T) {
	r, ok, reason := normalizeSchedulerResponse(pluginapi.SchedulerPickResponse{Handled: true, Reject: true, RejectCode: "entitlement_unavailable"}, pluginapi.SchedulerPickRequest{PreferenceOnly: true})
	if !ok || !r.Reject {
		t.Fatalf("explicit rejection lost: %+v %v %s", r, ok, reason)
	}
}
