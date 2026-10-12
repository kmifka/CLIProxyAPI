# Public management-only plugin host

Baseline: `5b3f07c15caeb686d2d9e1195e470510d54538e0` (direct parent; OAuth, static Home, required scheduler ABI and Claude metadata/race corrections retained without editing).

## Public contract

```go
func (*Host) RegisterManagementRoutes(context.Context, map[string]struct{})
func (*Host) ServeManagementHTTP(http.ResponseWriter, *http.Request) bool
func (*Host) ServeResourceHTTP(http.ResponseWriter, *http.Request) bool
```

All three delegate to the existing internal/native implementation. No alternate router, cached tables, middleware, ABI loader or service is introduced. `false` means unhandled and does not write the response. Reserved route keys are `METHOD /v0/management/path`. Menu readback already exists at `RegisteredPlugins()[i].Menus`.

Lifecycle: `New`; `ApplyConfig` invokes existing plugin Init/register (or Reconfigure on a later application); then `RegisterManagementRoutes`. After each reconfiguration repeat registration. Close with existing `ShutdownAllContext(ctx)` / `ShutdownAll()`, which detaches tables and unloads libraries. This is not a new lifecycle abstraction or full effective-config ACK. Serialize application-owned reconfigure/register/close transitions; existing native guarded clients protect active calls. No `cliproxy.Service.Run`, auth-manager, file-auth watcher, executor or automatic refresh is started.

The caller MUST protect management dispatch using its existing session issuer, role/scope authority and write CSRF checks; bound request bodies before dispatch. Resources are GET-only static assets under `/v0/resource/plugins/<id>/<path>` and must never contain credentials/auth data or protected live data. The SDK cannot inspect arbitrary trusted plugin asset content to enforce this classification. Do not substitute old SDK middleware or add a second realm.

Example: `sdk/pluginhost/management_example_test.go` compiles using only public SDK imports. Its authorization placeholder deliberately fails closed.

## Verified acceptance

- RED: external-package compilation fails at all three absent public methods before implementation.
- Public empty/nil/zero-host tests, native internal pluginhost tests and targeted race: PASS.
- `go build ./...`, ordinary `go test ./...`: PASS.
- Linux amd64 CGO public SDK test binary executed in a disposable `--network none` Podman container. Real Costs library copied read-only from the existing artifact, Fleet built from the existing full resource C source/header; metadata retained (Costs1.0.0, Fleet1.0.1). Actual Init, Reconfigure, authenticated native HTTP401/200, viewer write403, public static resource200, wrong resource404, empty-host scope isolation, unload and authdir-noncreation: PASS. Synthetic read backend is transport acceptance, NOT actual PostgreSQL valuation or deployed shared-session parity.
- No Service/executors/refresh callbacks are registered by this fixture; no authdir is created. This proves the host-only lifecycle, not safety of arbitrary untrusted plugins.

### Full-race first-run failure retained honestly

Initial concurrent ordinary/full-race run failed ONLY `internal/client/codex/live.TestPionMediaRelayBridgesAudioAndDataChannel`: `media_test.go:268: upstream DataChannel was not created`. No `WARNING: DATA RACE` appeared. It is a WebRTC readiness assertion, not a Claude metadata or wrapper race. The candidate modifies only public wrappers/tests; that WebRTC subtree is unchanged from exact5b.

Matched `go test -race ./internal/client/codex/live -run TestPionMediaRelayBridgesAudioAndDataChannel -count=3` passed on both exact5b baseline and candidate. Exact5b full-race passed. Candidate full-race rerun and final full-race passed. Final uncached `go test -race -count=1 -p=2 ./...` also passed, with every package exercised again. Initial failure is not erased and cannot be called reproduced baseline failure; evidence supports transient concurrent-load flakiness, not definitive root cause. No timeout was widened or unrelated source changed.

Raw logs and fixture binaries are local untracked acceptance artifacts. No VM deployment, Home change, production tokens, platform file changes or main-branch push occurs here.
