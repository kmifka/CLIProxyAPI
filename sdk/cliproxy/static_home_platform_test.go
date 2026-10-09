package cliproxy

import (
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
	"path/filepath"
	"reflect"
	"testing"
)

func TestStaticHomePlatformResolvedLocalOverrides(t *testing.T) {
	remote, err := config.ParseConfigBytes([]byte("port: 8317\nrequest-retry: 7\nplugins:\n  enabled: false\n  dir: /home/plugins\n"))
	if err != nil {
		t.Fatal(err)
	}
	local := cloneStaticValue(reflect.ValueOf(remote)).Interface().(*config.Config)
	local.Home.Enabled = true
	local.Port = 18318
	local.Plugins.Dir = filepath.Join(t.TempDir(), "identity", "plugins")
	svc, err := NewBuilder().WithStaticHomeConfig(local, nil).WithConfigPath(filepath.Join(t.TempDir(), "config.yaml")).Build()
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.checkStaticHomeConfig(remote); err != nil {
		t.Fatalf("resolved local override rejected: %v", err)
	}
	remote.RequestRetry++
	if err := svc.checkStaticHomeConfig(remote); err == nil {
		t.Fatal("policy change accepted")
	}
}
