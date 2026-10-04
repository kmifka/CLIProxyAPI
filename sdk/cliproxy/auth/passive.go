package auth

import (
	"errors"
	"os"
	"strings"
	"sync"
)

// Passive mode allows a candidate to use existing credentials for inference
// without ever rotating an OAuth refresh token. Refresh tokens can be single
// use, so sharing refresh ownership would revoke the active owner's token.
const passiveEnvVar = "CLIPROXY_AUTH_PASSIVE"

var (
	passiveOnce sync.Once
	passiveMode bool
)

func PassiveMode() bool {
	passiveOnce.Do(func() {
		switch strings.ToLower(strings.TrimSpace(os.Getenv(passiveEnvVar))) {
		case "1", "true", "yes", "on":
			passiveMode = true
		}
	})
	return passiveMode
}

var errPassiveRefresh = errors.New("auth refresh suppressed: passive mode")
