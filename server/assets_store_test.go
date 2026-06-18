package server

import (
	"strings"
	"testing"
	"time"
)

func TestAssetStoreGetExpiresIdleAsset(t *testing.T) {
	t.Parallel()

	store, err := newAssetStore()
	if err != nil {
		t.Fatalf("new asset store: %v", err)
	}
	defer store.close()

	store.ttl = time.Nanosecond
	record, err := store.create("prompt.wav", "audio/wav", strings.NewReader("voice"))
	if err != nil {
		t.Fatalf("create asset: %v", err)
	}

	time.Sleep(time.Millisecond)
	if _, ok := store.get(record.ID); ok {
		t.Fatalf("asset %q still available after ttl", record.ID)
	}
	if _, ok := store.records[record.ID]; ok {
		t.Fatalf("asset %q still tracked after ttl", record.ID)
	}
}
