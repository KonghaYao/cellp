package registry

import (
	"testing"
	"time"
)

func TestCursorRoundTrip(t *testing.T) {
	createdAt := time.Date(2026, 8, 29, 12, 0, 0, 123456789, time.UTC)
	id := "demo"
	cursor := EncodeCursor(createdAt, id)
	gotAt, gotID, err := DecodeCursor(cursor)
	if err != nil {
		t.Fatal(err)
	}
	if !gotAt.Equal(createdAt) || gotID != id {
		t.Fatalf("round trip: got %v %q want %v %q", gotAt, gotID, createdAt, id)
	}
}

func TestDecodeCursorInvalid(t *testing.T) {
	if _, _, err := DecodeCursor("not-valid"); err == nil {
		t.Fatal("expected error for invalid cursor")
	}
	if _, _, err := DecodeCursor(""); err == nil {
		t.Fatal("expected error for empty cursor")
	}
}
