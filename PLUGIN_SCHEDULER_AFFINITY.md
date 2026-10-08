# Plugin scheduling with native session affinity

## Integration contract

The cold/failover composition uses existing scheduler registration. Strict
preference-tier failback adds one opt-in capability, `SchedulerPreference`
(`scheduler_preference` in RPC), plus `PreferenceOnly` on SchedulerPickRequest
and `EligibleAuthIDs` on SchedulerPickResponse. No new C ABI method is required.
The host and Manager expose `SchedulerWantsPreference()` for this opt-in.
Register the application's scheduler through the existing plugin loader and
`Capabilities.Scheduler` (`scheduler` in RPC capabilities). Keep native
`routing.session-affinity: true`. Do not replace the Manager selector or install
an application-only `SetPluginScheduler` adapter: Service configuration sync
reinstalls the plugin host, and routing changes replace the native selector.
The existing Register/Reconfigure lifecycle owns policy registration on reload.

When the configured selector is `SessionAffinitySelector`, it now wraps plugin
scheduling as its request-local fallback:

1. With `scheduler_preference` enabled, a membership-only scheduler call runs
   before native lookup. It must not rotate or emit maintenance side effects.
   A handled non-empty list may narrow only supplied SDK-ready candidate IDs;
   invalid/empty/unhandled membership leaves native behavior unchanged. Snapshots
   are rechecked after callbacks. Across-priority visibility is a separate opt-in.
   Native explicit/derived/LCP affinity then looks for an eligible binding. A
   binding within the preference subset is reused; one outside it is naturally
   reselected and rebound on this request only. Without opt-in, native behavior
   is unchanged and a usable binding wins across priority tiers.
2. A cold session or genuine failover invokes the plugin with the existing
   filtered candidates (model support, kind/policy, disabled/cooldown state,
   pinned auth and retry exclusions). Default candidates remain highest-priority;
   the existing `SchedulerAcrossPriorities` opt-in remains available.
3. `AuthID` selects a supplied candidate. The host rechecks its current account,
   provider, model and eligibility state after the plugin callback; a stale pick
   falls back to current native candidates. No plugin callback runs under the
   Manager lock, so host callbacks cannot deadlock on Manager reads/updates.
4. An unhandled/invalid response falls back to the configured native strategy.
   `DelegateBuiltin: round-robin` or `fill-first` retains the existing native
   scheduler delegation. All successful fallback/delegate selections return
   through native affinity, which creates/replaces the normal binding and
   metadata. Existing `OnResult` binding/invalidation remains intact.
5. Explicit plugin errors/rejections retain their existing terminal semantics.
   Return unhandled for stale/missing application policy data when native
   fallback is wanted; do not reject merely because an old bound account is
   absent from this request's candidates.

Return unhandled for non-target providers. In mixed-provider requests the
existing request has an empty Provider and a Providers list: the application
must decide explicitly whether it owns that request. SDK code contains no
Codex/quota/ping policy.

## Affinity observation

`Manager.LookupSessionAffinity` and C ABI `host.affinity.lookup` can observe native
bindings with a scheduler installed. The lookup remains read-only: no selection,
new binding or TTL refresh. The callback still returns only the opaque
`AuthIndex`, status, timestamp and disabled/unavailable flags, not credentials.
`bound` is an observation, NOT proof that the account is eligible for the current
request. It does not encode request kind/policy, retry exclusions or all model
cooldowns; the selection path is authoritative. Ambiguous/unbound/unsupported
statuses keep their previous meanings. There is intentionally no separate bind
ABI: letting native Pick own both lookup and binding avoids two competing stores.

## Concurrency and lifecycle boundaries

Policy fallback is request-local; the shared native selector/fallback is never
mutated or temporarily replaced. Concurrent requests use native cache/matcher
synchronization and native last-writer binding semantics. This does not add
single-flight first-selection semantics, execution-time credential reservations,
or atomicity with changes occurring after the final eligibility check. A later
request/retry revalidates native candidates as before.

An unchanged routing config retains the affinity selector/cache. A routing
strategy/affinity setting change replaces it as before; registered plugin policy
survives via the existing host lifecycle, while old bindings are not promised to
survive a selector replacement. OAuth/passive ownership and executors are untouched.

## Verification

Based only on `f635f265e870d5585681731045ecca1e71e93072`.
Regression tests cover cold plugin selection, native binding reuse/lookup,
explicit and LCP bindings, single/mixed retry/disabled/cooldown/model/kind
failover, post-callback concurrent disable, builtin delegation, invalid/unhandled
fallback, rejection/error propagation, plugin registration/reconfiguration,
selector replacement and non-Codex native behavior.

Full `go test ./...` and server build passed. Full auth/pluginhost race suites
passed, and scheduler/affinity/service-injection tests passed ten race-enabled
repetitions. The complete service race suite passed one run but failed other
runs in three existing auth-sync tests (batch hook wait, auth-file replacement,
and auth-file patch model visibility). Repeating those tests on clean deployed
f635f265 reproduced the same failures. This patch does not fix or hide those
baseline flakes; do not describe the whole service race gate as reliably green.
No race detector data-race report was emitted in these failing runs.
