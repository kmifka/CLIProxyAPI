package helps

import "testing"

// Invalid JSON numeric dimensions must never be coerced into observed integers.
// Raw sanitized protocol fixture, not a provider price/capture claim.
func TestCacheObservationRejectsFractionalAndOverflowCounters(t *testing.T) {
	for _, raw := range []string{"1.5", "9223372036854775808", "1e30"} {
		d := ParseClaudeUsage([]byte(`{"usage":{"input_tokens":10,"output_tokens":2,"cache_read_input_tokens":` + raw + `}}`))
		if d.CacheReadObserved != nil {
			t.Fatalf("malformed %s became observed integer %d", raw, *d.CacheReadObserved)
		}
	}
}
