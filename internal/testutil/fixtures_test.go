package testutil_test

import (
	"testing"
	"time"

	"omblego/internal/testutil"
)

func TestRecordGenerator_GenerateRecords(t *testing.T) {
	gen := testutil.NewRecordGenerator().WithSeed(42)

	records := gen.GenerateRecords(5)

	if len(records) != 5 {
		t.Errorf("Expected 5 records, got %d", len(records))
	}

	// Check records have valid values
	for i, r := range records {
		if r.Systolic < 100 || r.Systolic >= 140 {
			t.Errorf("Record %d: systolic %d out of range [100, 140)", i, r.Systolic)
		}
		if r.Diastolic < 60 || r.Diastolic >= 90 {
			t.Errorf("Record %d: diastolic %d out of range [60, 90)", i, r.Diastolic)
		}
		if r.Pulse < 60 || r.Pulse >= 100 {
			t.Errorf("Record %d: pulse %d out of range [60, 100)", i, r.Pulse)
		}
	}
}

func TestRecordGenerator_Reproducible(t *testing.T) {
	gen1 := testutil.NewRecordGenerator().WithSeed(12345)
	gen2 := testutil.NewRecordGenerator().WithSeed(12345)

	records1 := gen1.GenerateRecords(3)
	records2 := gen2.GenerateRecords(3)

	for i := range records1 {
		if records1[i].Systolic != records2[i].Systolic {
			t.Errorf("Record %d: systolic mismatch with same seed", i)
		}
	}
}

func TestRecordGenerator_UserRecords(t *testing.T) {
	gen := testutil.NewRecordGenerator().
		WithUserCount(2).
		WithSeed(42)

	records := gen.GenerateUserRecords(3)

	if len(records) != 2 {
		t.Errorf("Expected 2 users, got %d", len(records))
	}

	for u, userRecords := range records {
		if len(userRecords) != 3 {
			t.Errorf("User %d: expected 3 records, got %d", u, len(userRecords))
		}
	}
}

func TestFixedRecord(t *testing.T) {
	timestamp := time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC)
	record := testutil.FixedRecord(timestamp, 125, 82, 72)

	if record.Timestamp != timestamp {
		t.Errorf("Timestamp mismatch")
	}
	if record.Systolic != 125 {
		t.Errorf("Expected systolic 125, got %d", record.Systolic)
	}
	if record.Diastolic != 82 {
		t.Errorf("Expected diastolic 82, got %d", record.Diastolic)
	}
	if record.Pulse != 72 {
		t.Errorf("Expected pulse 72, got %d", record.Pulse)
	}
}

func TestSampleRecords(t *testing.T) {
	records := testutil.SampleRecords()

	if len(records) != 1 {
		t.Errorf("Expected 1 user, got %d", len(records))
	}

	if len(records[0]) != 3 {
		t.Errorf("Expected 3 records, got %d", len(records[0]))
	}
}

func TestSampleRecordsTwoUsers(t *testing.T) {
	records := testutil.SampleRecordsTwoUsers()

	if len(records) != 2 {
		t.Errorf("Expected 2 users, got %d", len(records))
	}

	if len(records[0]) != 2 || len(records[1]) != 2 {
		t.Error("Expected 2 records per user")
	}
}
