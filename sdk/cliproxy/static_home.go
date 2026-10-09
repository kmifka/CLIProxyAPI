package cliproxy

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v8/internal/home"
	"github.com/router-for-me/CLIProxyAPI/v8/sdk/config"
)

// StaticHomeRejection identifies a rejected explicit configuration, not credentials.
// RequestedFingerprint covers the effective config; it is not a startup receipt.
type StaticHomeRejection struct {
	RequestedFingerprint    string
	LifecycleConfigRevision int64
}

type staticHomeConfig struct {
	pinned      *config.Config
	fingerprint string
	notify      func(StaticHomeRejection)
	err         error
}

// WithStaticHomeConfig opts into immutable Home config and replaces WithConfig.
// It deep-copies cfg immediately, before Build consumes it. Callbacks must return
// promptly; panics are contained. Home credential dispatch remains dynamic.
func (b *Builder) WithStaticHomeConfig(cfg *config.Config, onRejected func(StaticHomeRejection)) *Builder {
	b.staticHome = &staticHomeConfig{notify: onRejected}
	if cfg == nil {
		b.staticHome.err = fmt.Errorf("cliproxy: static Home config is required")
		return b
	}
	b.cfg = cloneStaticValue(reflect.ValueOf(cfg)).Interface().(*config.Config)
	b.staticHome.pinned = b.cfg
	if !b.cfg.Home.Enabled {
		b.staticHome.err = fmt.Errorf("cliproxy: static Home requires Home mode")
	}
	return b
}

// cloneStaticValue preserves runtime-only fields as well as nested explicit config.
func cloneStaticValue(v reflect.Value) reflect.Value {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		n := reflect.New(v.Type().Elem())
		n.Elem().Set(cloneStaticValue(v.Elem()))
		return n
	case reflect.Interface:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		n := reflect.New(v.Type()).Elem()
		n.Set(cloneStaticValue(v.Elem()))
		return n
	case reflect.Struct:
		n := reflect.New(v.Type()).Elem()
		n.Set(v)
		for i := 0; i < v.NumField(); i++ {
			if n.Field(i).CanSet() && v.Type().Field(i).IsExported() {
				n.Field(i).Set(cloneStaticValue(v.Field(i)))
			}
		}
		return n
	case reflect.Slice:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		n := reflect.MakeSlice(v.Type(), v.Len(), v.Len())
		for i := 0; i < v.Len(); i++ {
			n.Index(i).Set(cloneStaticValue(v.Index(i)))
		}
		return n
	case reflect.Map:
		if v.IsNil() {
			return reflect.Zero(v.Type())
		}
		n := reflect.MakeMapWithSize(v.Type(), v.Len())
		it := v.MapRange()
		for it.Next() {
			n.SetMapIndex(it.Key(), cloneStaticValue(it.Value()))
		}
		return n
	default:
		return v
	}
}

// Explicit field traversal deliberately ignores JSON/YAML omission tags. Every
// exported full-config field participates, including observation barriers.
func staticConfigFields(v reflect.Value) any {
	if v.Kind() == reflect.Pointer || v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		return staticConfigFields(v.Elem())
	}
	switch v.Kind() {
	case reflect.Struct:
		out := map[string]any{}
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				out[v.Type().Field(i).Name] = staticConfigFields(v.Field(i))
			}
		}
		return out
	case reflect.Slice, reflect.Array:
		if v.Kind() == reflect.Slice && v.IsNil() {
			return nil
		}
		out := make([]any, v.Len())
		for i := range out {
			out[i] = staticConfigFields(v.Index(i))
		}
		return out
	case reflect.Map:
		if v.IsNil() {
			return nil
		}
		out := map[string]any{}
		it := v.MapRange()
		for it.Next() {
			out[fmt.Sprint(it.Key().Interface())] = staticConfigFields(it.Value())
		}
		return out
	default:
		return v.Interface()
	}
}
func staticFingerprint(cfg *config.Config) (string, error) {
	raw, err := json.Marshal(staticConfigFields(reflect.ValueOf(cfg)))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(raw)), nil
}

func mergedStaticHomeConfig(remote, pinned *config.Config) *config.Config {
	merged := cloneStaticValue(reflect.ValueOf(remote)).Interface().(*config.Config)
	merged.Host, merged.Port, merged.TLS, merged.Home = pinned.Host, pinned.Port, pinned.TLS, pinned.Home
	// Home workers resolve their plugin installation directory per identity.
	// This filesystem destination is local runtime configuration, not Home policy.
	merged.Plugins.Dir = pinned.Plugins.Dir
	// Native runtime-only SDK fields cannot arrive in Home YAML.
	merged.OAuthOnlyFields = pinned.OAuthOnlyFields
	merged.CodexResponseSteering = pinned.CodexResponseSteering
	merged.CodexOrphanDelegationCompatibility = pinned.CodexOrphanDelegationCompatibility
	forceHomeRuntimeConfig(merged)
	merged.NormalizePluginsConfig()
	_ = merged.ResolvePluginsDir()
	return merged
}
func (s *Service) checkStaticHomeConfig(remote *config.Config) error {
	if s == nil || s.staticHome == nil {
		return nil
	}
	g := s.staticHome
	fp, err := staticFingerprint(mergedStaticHomeConfig(remote, g.pinned))
	if err == nil && fp == g.fingerprint {
		return nil
	}
	if g.notify != nil {
		func() {
			defer func() { _ = recover() }()
			g.notify(StaticHomeRejection{RequestedFingerprint: fp, LifecycleConfigRevision: remote.CredentialConcurrency.LifecycleConfigRevision})
		}()
	}
	return fmt.Errorf("cliproxy: static Home configuration rejected (requested %s)", fp)
}

func (s *Service) validateInitialStaticHome(ctx context.Context) error {
	if s.staticHome == nil {
		return nil
	}
	client := home.New(s.staticHome.pinned.Home)
	defer client.Close()
	raw, err := client.GetConfig(ctx)
	if err != nil {
		return err
	}
	remote, err := config.ParseConfigBytes(raw)
	if err != nil {
		return err
	}
	return s.checkStaticHomeConfig(remote)
}

// StaticHomeReadOnlyManagement is a public application middleware option. It
// fails closed for ALL non-read management requests, including future routes.
// Install with api.WithMiddleware; the Builder installs it for opt-in workers.
func StaticHomeReadOnlyManagement(c *gin.Context) {
	path := c.Request.URL.Path
	if (path == "/v0/management" || strings.HasPrefix(path, "/v0/management/")) && c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead && c.Request.Method != http.MethodOptions {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "static Home worker management is read-only"})
		return
	}
	c.Next()
}
