# Opt-in static Home configuration hook (base a0101952)

```go
service, err := cliproxy.NewBuilder().
    WithStaticHomeConfig(suppliedConfig, func(r cliproxy.StaticHomeRejection) {
        // Send a revision/fingerprint notification to your application's bounded queue.
        // Return promptly. This callback is synchronous; panics are contained.
    }).
    WithConfigPath(configPath).
    Build()
```

Supply an already parsed/defaulted native config with Home enabled. The option deep-copies it immediately; Build normalizes plugin paths and applies native Home runtime overrides before managers consume the config. Each built service owns its pinned fingerprint. Runtime credentials obtained from Home are not fingerprinted: provider policy/config is static, credential dispatch remains dynamic.

The fingerprint traverses all exported full-config fields, irrespective of JSON/YAML omission tags. Candidate comparison uses native Home merge semantics: local Host/Port/TLS/Home remain pinned, native runtime-only SDK flags remain supplied, and forceHomeRuntimeConfig plus plugin normalization applies. No arbitrary lifecycle/observation barrier exemptions exist.

Run performs an initial native Home GET and rejects mismatches before Run creates its cancellation state, server, subscriber, or other runtime components. Build's existing manager/plugin construction is not transactional and remains outside this guarantee. No startup ACK or readiness receipt is introduced; application readiness must use external listener/catalog/inference probes, not OnAfterStart.

Subscriber candidates are checked before lifecycle configuration, publisher configuration, observation barrier, cancel bound, and config work enqueue. Rejection calls the application's notification and leaves the current transport/heartbeat/credential dispatch alive. Every new transport lifetime initializes lifecycle/publisher/barrier settings from the pin. A rejected reconnect GET uses a private nil queue marker to bootstrap the existing pinned config worker; the rejected candidate itself is never enqueued. Plugin sync/task/finalization retains existing native behavior for accepted/pinned config, not a new all-config atomic contract.

Opt-in builders install the public StaticHomeReadOnlyManagement middleware through the existing api.WithMiddleware seam. It rejects **all** non-GET/HEAD/OPTIONS `/v0/management` requests before native handlers, including credential/admin operations, rather than maintaining an incomplete setter allowlist. This deliberate worker-level read-only policy is broader than config setters. Ordinary builders are unaffected. The core config guard covers Home overlay staging, initial GET validation, and watcher application only; it is not a universal config transaction guard or protection against application-owned direct pointer mutation/hooks. Applications must not modify service config from OnBeforeStart.

Native Home mode already does not create a file watcher: service_lifecycle.go wraps watcher construction/start in `if !homeEnabled`. Static mode additionally ignores watcher config callbacks defensively.

Coverage: real native Go Home client against a local RESP protocol fixture using existing command/message helpers; changed subscription rejection, requested lifecycle revision, heartbeat surviving, actual RPOP credential request/response, changed reconnect GET without repin, initial mismatch before Run state mutation, callback panic containment, barrier participation, pre-Build copy, watcher rejection, and fail-closed management middleware. This is not an actual deployed Home server or an end-to-end HTTP inference qualification.

Verification completed: `go build ./...`; `go test ./... -count=1 -timeout=120s`; targeted `go test -race ./sdk/cliproxy -run TestStaticHome -count=1 -timeout=60s`; affected-package tests also pass on detached pristine base a0101952. No production writes, commits, tags, or pushes.
