package model

import (
	"encoding/json"
	"testing"
)

func TestFrozenDetails_Value(t *testing.T) {
	fd := FrozenDetails{
		{EntitlementID: 1, Seconds: 100},
		{EntitlementID: 2, Seconds: 200},
	}

	val, err := fd.Value()
	if err != nil {
		t.Fatalf("Value() returned error: %v", err)
	}

	var parsed []FrozenDetail
	if err := json.Unmarshal(val.([]byte), &parsed); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if len(parsed) != 2 {
		t.Fatalf("expected 2 items, got %d", len(parsed))
	}
	if parsed[0].EntitlementID != 1 || parsed[0].Seconds != 100 {
		t.Errorf("first item: got {%d, %d}, want {1, 100}", parsed[0].EntitlementID, parsed[0].Seconds)
	}
	if parsed[1].EntitlementID != 2 || parsed[1].Seconds != 200 {
		t.Errorf("second item: got {%d, %d}, want {2, 200}", parsed[1].EntitlementID, parsed[1].Seconds)
	}
}

func TestFrozenDetails_Value_Empty(t *testing.T) {
	fd := FrozenDetails{}

	val, err := fd.Value()
	if err != nil {
		t.Fatalf("Value() returned error: %v", err)
	}

	if string(val.([]byte)) != "[]" {
		t.Errorf("expected '[]', got '%s'", string(val.([]byte)))
	}
}

func TestFrozenDetails_Scan_Bytes(t *testing.T) {
	input := `[{"entitlement_id":10,"seconds":500},{"entitlement_id":20,"seconds":600}]`

	var fd FrozenDetails
	if err := fd.Scan([]byte(input)); err != nil {
		t.Fatalf("Scan() returned error: %v", err)
	}

	if len(fd) != 2 {
		t.Fatalf("expected 2 items, got %d", len(fd))
	}
	if fd[0].EntitlementID != 10 || fd[0].Seconds != 500 {
		t.Errorf("first item: got {%d, %d}, want {10, 500}", fd[0].EntitlementID, fd[0].Seconds)
	}
	if fd[1].EntitlementID != 20 || fd[1].Seconds != 600 {
		t.Errorf("second item: got {%d, %d}, want {20, 600}", fd[1].EntitlementID, fd[1].Seconds)
	}
}

func TestFrozenDetails_Scan_Nil(t *testing.T) {
	fd := FrozenDetails{{EntitlementID: 1, Seconds: 100}}

	if err := fd.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) returned error: %v", err)
	}

	if fd != nil {
		t.Errorf("expected nil after Scan(nil), got %v", fd)
	}
}

func TestFrozenDetails_Scan_WrongType(t *testing.T) {
	var fd FrozenDetails

	err := fd.Scan("not bytes")
	if err == nil {
		t.Fatal("expected error for non-[]byte input, got nil")
	}
	if err.Error() != "type assertion to []byte failed" {
		t.Errorf("unexpected error message: %s", err.Error())
	}
}

func TestFrozenDetails_RoundTrip(t *testing.T) {
	original := FrozenDetails{
		{EntitlementID: 42, Seconds: 999},
		{EntitlementID: 99, Seconds: 1},
	}

	val, err := original.Value()
	if err != nil {
		t.Fatalf("Value() returned error: %v", err)
	}

	var restored FrozenDetails
	if err := restored.Scan(val); err != nil {
		t.Fatalf("Scan() returned error: %v", err)
	}

	if len(restored) != len(original) {
		t.Fatalf("length mismatch: got %d, want %d", len(restored), len(original))
	}
	for i, item := range restored {
		if item.EntitlementID != original[i].EntitlementID {
			t.Errorf("item %d: EntitlementID got %d, want %d", i, item.EntitlementID, original[i].EntitlementID)
		}
		if item.Seconds != original[i].Seconds {
			t.Errorf("item %d: Seconds got %d, want %d", i, item.Seconds, original[i].Seconds)
		}
	}
}
