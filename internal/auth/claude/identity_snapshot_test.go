package claude

import (
	"sync"
	"testing"
)

func TestCloneMetadataOwnsDevicePoolSlices(t *testing.T) {
	metadata := map[string]any{"strings": []string{"original"}, "values": []any{"original"}}
	snapshot := CloneMetadata(&metadata)
	snapshot["strings"].([]string)[0] = "changed"
	snapshot["values"].([]any)[0] = "changed"
	snapshot["new"] = true
	if metadata["strings"].([]string)[0] != "original" || metadata["values"].([]any)[0] != "original" || len(metadata) != 2 {
		t.Fatal("snapshot mutated source metadata")
	}
}

func TestCloneMetadataSynchronizesWithMetadataWriters(t *testing.T) {
	var metadata map[string]any
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			for j := 0; j < 32; j++ {
				if i%2 == 0 {
					StoreMetadataString(&metadata, "account_uuid", "account")
				} else {
					snapshot := CloneMetadata(&metadata)
					snapshot["private"] = true
				}
			}
		}(i)
	}
	close(start)
	wg.Wait()
	if _, exists := metadata["private"]; exists {
		t.Fatal("snapshot writes leaked to shared metadata")
	}
}
