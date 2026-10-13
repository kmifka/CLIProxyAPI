package home

import (
	"testing"

	"github.com/redis/go-redis/v9/maintnotifications"
)

func TestHomeTransportDisablesRedisMaintenanceNegotiation(t *testing.T) {
	c := &Client{}
	options, err := c.redisOptionsLocked("127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	if options.MaintNotificationsConfig == nil || options.MaintNotificationsConfig.Mode != maintnotifications.ModeDisabled {
		t.Fatal("Home RESP transport must not negotiate Redis maintenance notifications across timeout clones")
	}
}
