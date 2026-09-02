package auth

import (
	"errors"
	"os"
	"strings"
	"sync"
)

// Passive mode lets an instance consume existing credentials without ever
// maintaining them.
//
// It exists because OAuth refresh tokens rotate: a refresh returns a new token
// and invalidates the old one server-side. Two instances sharing an auth
// directory therefore cannot both refresh - the second one presents a token the
// provider has already retired, and the credential dies. Nothing in this
// codebase locks the auth directory, so the only safe arrangement is that
// exactly one instance maintains a given credential set at a time.
//
// A canary needs to exercise the real credentials before it takes traffic, so
// it runs passive: it reads the tokens the active instance maintains, follows
// their updates through the existing auth directory watcher, and never writes
// or refreshes anything itself. Killing it leaves nothing behind - which is the
// whole point, because a rotation cannot be undone locally. The damage would be
// at the provider, not on disk.
//
// Read from the environment rather than the config file so that both instances
// can mount the same configuration and differ in exactly one variable. One
// fewer thing that can drift apart.
const passiveEnvVar = "CLIPROXY_AUTH_PASSIVE"

var (
	passiveOnce sync.Once
	passiveMode bool
)

// PassiveMode reports whether this process must not maintain credentials.
// The value is read once: it decides a safety property, so it must not change
// underneath a request that already checked it.
func PassiveMode() bool {
	passiveOnce.Do(func() {
		switch strings.ToLower(strings.TrimSpace(os.Getenv(passiveEnvVar))) {
		case "1", "true", "yes", "on":
			passiveMode = true
		}
	})
	return passiveMode
}

// errPassiveRefresh is returned instead of performing a refresh. Callers treat
// it like any other refresh failure, which is correct: from their point of view
// the credential simply could not be renewed here.
var errPassiveRefresh = errors.New("auth refresh suppressed: passive mode")
