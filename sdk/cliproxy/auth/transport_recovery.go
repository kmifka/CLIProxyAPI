package auth

import "time"

// legacyFailureRetryAfter derives a deadline from the original failure time,
// never the selection clock. It repairs only wire-classified request failures;
// quota, policy/auth restrictions and undiagnosed/manual holds remain unchanged.
func legacyFailureRetryAfter(unavailable bool, quota QuotaState, retry time.Time, last *Error, updated time.Time) time.Time {
	if !unavailable || quota.Exceeded || !retry.IsZero() || !quota.NextRecoverAt.IsZero() || last == nil || updated.IsZero() {
		return retry
	}
	switch last.HTTPStatus {
	case 408, 500, 502, 503, 504, 520, 521, 522, 523, 524, 525, 526:
		return updated.Add(transientErrorCooldown)
	default:
		return retry
	}
}
