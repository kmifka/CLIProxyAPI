package pluginhost

import (
	"context"
	"errors"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/pluginapi"
	"testing"
)

func TestRequiredSchedulerABIErrorClassification(t *testing.T) {
	for _, required := range []bool{false, true} {
		for _, code := range []string{"plugin_panic", "plugin_fused", "scheduler_unavailable", "content_policy", "inference_error"} {
			original := rpcError{Code: code, message: "ABI failure", statusCode: 403}
			host := newHostWithRecords(capabilityRecord{id: "scheduler", plugin: pluginapi.Plugin{Capabilities: pluginapi.Capabilities{SchedulerPreference: required, Scheduler: schedulerFunc(func(context.Context, pluginapi.SchedulerPickRequest) (pluginapi.SchedulerPickResponse, error) {
				return pluginapi.SchedulerPickResponse{}, original
			})}}})
			_, _, err := host.PickAuth(context.Background(), schedulerRequest("auth-1"))
			var classified interface{ ErrorClass() string }
			marked := errors.As(err, &classified)
			want := required && (code == "plugin_panic" || code == "plugin_fused" || code == "scheduler_unavailable")
			if marked != want {
				t.Errorf("required=%v code=%s classified=%v want=%v", required, code, marked, want)
			}
			if !want && err != original {
				t.Errorf("unrelated error changed: %v", err)
			}
		}
	}
}
