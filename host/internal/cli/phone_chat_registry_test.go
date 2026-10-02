package cli

import (
	"testing"

	"github.com/hybriqtech/adbuddy/host/internal/bridge"
)

// V22: the registry of attachments forgets the oldest once it holds
// maxRegistered, which is more than the history can refer to. Putting an id
// again changes it where it is: it takes no second place.
func TestAttachmentRegistryIsBounded(t *testing.T) {
	if maxRegistered < keptChat*maxChatAttachments {
		t.Fatalf("maxRegistered %d is below what the history can refer to (%d)", maxRegistered, keptChat*maxChatAttachments)
	}
	var r attachmentRegistry
	for i := 0; i <= maxRegistered; i++ {
		r.put(bridge.Attachment{ID: fileID(i)}, "outbox/x")
	}
	if len(r.m) != maxRegistered {
		t.Fatalf("%d kept, want %d", len(r.m), maxRegistered)
	}
	if _, ok := r.get(fileID(0)); ok {
		t.Fatal("the oldest is still there")
	}
	for _, i := range []int{1, maxRegistered} {
		if _, ok := r.get(fileID(i)); !ok {
			t.Fatalf("attachment %d forgotten", i)
		}
	}
	r.put(bridge.Attachment{ID: fileID(1), Size: 7}, "inbox/y")
	if a, ok := r.get(fileID(1)); !ok || a.Size != 7 || a.Path != "inbox/y" || len(r.m) != maxRegistered {
		t.Fatalf("put again: %+v %v, %d kept", a, ok, len(r.m))
	}
	// It kept its place: it is still the oldest, and the next one forgets it.
	r.put(bridge.Attachment{ID: fileID(maxRegistered + 1)}, "outbox/z")
	if _, ok := r.get(fileID(1)); ok {
		t.Fatal("the oldest (put again) was kept")
	}
	if _, ok := r.get(fileID(2)); !ok || len(r.m) != maxRegistered {
		t.Fatalf("the second oldest forgotten, or %d kept", len(r.m))
	}
}
