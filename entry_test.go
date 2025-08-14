package flash

import "testing"

func TestEntryNewReexport(t *testing.T) {
	if New() == nil {
		t.Fatalf("New returned nil")
	}
}
