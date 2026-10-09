package helps

import "testing"

func TestClaudeCachePresenceAndTiers(t *testing.T) {
	d := ParseClaudeUsage([]byte(`{"usage":{"input_tokens":2,"output_tokens":3,"cache_read_input_tokens":100,"cache_creation_input_tokens":20,"cache_creation":{"ephemeral_5m_input_tokens":7,"ephemeral_1h_input_tokens":13}}}`))
	if d.InputTokenSemantics != "exclusive" || d.TokenProvenance != "claude-messages:v1" || d.CacheReadObserved == nil || *d.CacheReadObserved != 100 || d.CacheCreationObserved == nil || *d.CacheCreationObserved != 20 || d.CacheCreation5mObserved == nil || *d.CacheCreation5mObserved != 7 || d.CacheCreation1hObserved == nil || *d.CacheCreation1hObserved != 13 || d.TotalTokens != 125 {
		t.Fatalf("lost independent dimensions: %+v", d)
	}
	d = ParseClaudeUsage([]byte(`{"usage":{"input_tokens":2,"output_tokens":3,"cache_read_input_tokens":0}}`))
	if d.CacheReadObserved == nil || *d.CacheReadObserved != 0 || d.CacheCreationObserved != nil {
		t.Fatalf("unknown collapsed to zero: %+v", d)
	}
}
func TestClaudeStreamCacheObservationsSurviveOutputOnlyDelta(t *testing.T) {
	start := ParseClaudeUsage([]byte(`{"usage":{"input_tokens":2,"output_tokens":1,"cache_read_input_tokens":100,"cache_creation_input_tokens":20}}`))
	end := ParseClaudeUsage([]byte(`{"usage":{"output_tokens":3}}`))
	d := MergeStreamUsageDetail(start, end)
	if d.CacheReadObserved == nil || *d.CacheReadObserved != 100 || d.CacheCreationObserved == nil || *d.CacheCreationObserved != 20 {
		t.Fatalf("stream cache observations lost: %+v", d)
	}
	zero := ParseClaudeUsage([]byte(`{"usage":{"output_tokens":3,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`))
	d = MergeStreamUsageDetail(start, zero)
	if d.CacheReadObserved == nil || *d.CacheReadObserved != 0 || d.CacheCreationObserved == nil || *d.CacheCreationObserved != 0 || d.CacheReadTokens != 0 || d.CacheCreationTokens != 0 || d.TotalTokens != 5 {
		t.Fatalf("explicit zero replaced by old observations: %+v", d)
	}
}

func TestOpenAIInclusiveCachePresence(t *testing.T) {
	d := ParseOpenAIUsage([]byte(`{"usage":{"prompt_tokens":100,"completion_tokens":4,"prompt_tokens_details":{"cached_tokens":80,"cache_write_tokens":10}}}`))
	if d.InputTokenSemantics != "inclusive" || d.TokenProvenance != "openai-usage:v1" || d.CacheReadObserved == nil || *d.CacheReadObserved != 80 || d.CacheCreationObserved == nil || *d.CacheCreationObserved != 10 || d.TotalTokens != 104 {
		t.Fatalf("lost inclusive dimensions: %+v", d)
	}
}
