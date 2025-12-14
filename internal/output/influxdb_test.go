package output

import (
	"testing"
	"time"

	"omblego/internal/device"
)

func TestSentTracker_FilterNewRecords(t *testing.T) {
	tracker := NewSentTracker()

	// Create test records with different timestamps
	now := time.Now()
	records := [][]device.BloodPressureRecord{
		{
			{Timestamp: now.Add(-3 * time.Hour), Systolic: 120, Diastolic: 80, Pulse: 70},
			{Timestamp: now.Add(-2 * time.Hour), Systolic: 125, Diastolic: 82, Pulse: 72},
			{Timestamp: now.Add(-1 * time.Hour), Systolic: 118, Diastolic: 78, Pulse: 68},
		},
	}

	// First call - all records should be new
	newRecords := tracker.FilterNewRecords(records)
	if len(newRecords[0]) != 3 {
		t.Errorf("Expected 3 new records, got %d", len(newRecords[0]))
	}

	// Update tracker with sent records
	tracker.UpdateLastSent(newRecords)

	// Second call with same records - none should be new
	newRecords = tracker.FilterNewRecords(records)
	if len(newRecords[0]) != 0 {
		t.Errorf("Expected 0 new records after sending, got %d", len(newRecords[0]))
	}

	// Add a new record
	records[0] = append(records[0], device.BloodPressureRecord{
		Timestamp: now,
		Systolic:  130,
		Diastolic: 85,
		Pulse:     75,
	})

	// Should only get the new record
	newRecords = tracker.FilterNewRecords(records)
	if len(newRecords[0]) != 1 {
		t.Errorf("Expected 1 new record, got %d", len(newRecords[0]))
	}
	if newRecords[0][0].Systolic != 130 {
		t.Errorf("Expected new record with systolic 130, got %d", newRecords[0][0].Systolic)
	}
}

func TestSentTracker_MultipleUsers(t *testing.T) {
	tracker := NewSentTracker()

	now := time.Now()
	records := [][]device.BloodPressureRecord{
		// User 1
		{
			{Timestamp: now.Add(-2 * time.Hour), Systolic: 120, Diastolic: 80, Pulse: 70},
		},
		// User 2
		{
			{Timestamp: now.Add(-1 * time.Hour), Systolic: 130, Diastolic: 85, Pulse: 75},
		},
	}

	// All should be new
	newRecords := tracker.FilterNewRecords(records)
	if len(newRecords[0]) != 1 || len(newRecords[1]) != 1 {
		t.Errorf("Expected 1 record per user, got %d and %d", len(newRecords[0]), len(newRecords[1]))
	}

	tracker.UpdateLastSent(newRecords)

	// None should be new now
	newRecords = tracker.FilterNewRecords(records)
	if len(newRecords[0]) != 0 || len(newRecords[1]) != 0 {
		t.Errorf("Expected 0 records after sending, got %d and %d", len(newRecords[0]), len(newRecords[1]))
	}
}

func TestSentTracker_EmptyRecords(t *testing.T) {
	tracker := NewSentTracker()

	// Empty records
	records := [][]device.BloodPressureRecord{}
	newRecords := tracker.FilterNewRecords(records)
	if len(newRecords) != 0 {
		t.Errorf("Expected empty result for empty input")
	}

	// Empty user records
	records = [][]device.BloodPressureRecord{{}}
	newRecords = tracker.FilterNewRecords(records)
	if len(newRecords) != 1 || len(newRecords[0]) != 0 {
		t.Errorf("Expected empty user records")
	}
}

func TestSentTracker_UpdateLastSent(t *testing.T) {
	tracker := NewSentTracker()

	now := time.Now()

	// Send older record first
	records := [][]device.BloodPressureRecord{
		{
			{Timestamp: now.Add(-2 * time.Hour), Systolic: 120, Diastolic: 80, Pulse: 70},
		},
	}
	tracker.UpdateLastSent(records)

	// Now send a mix of old and new
	records = [][]device.BloodPressureRecord{
		{
			{Timestamp: now.Add(-3 * time.Hour), Systolic: 115, Diastolic: 75, Pulse: 65}, // older
			{Timestamp: now.Add(-1 * time.Hour), Systolic: 125, Diastolic: 82, Pulse: 72}, // newer
		},
	}

	newRecords := tracker.FilterNewRecords(records)

	// Should only get the newer one
	if len(newRecords[0]) != 1 {
		t.Errorf("Expected 1 new record, got %d", len(newRecords[0]))
	}
	if newRecords[0][0].Systolic != 125 {
		t.Errorf("Expected systolic 125, got %d", newRecords[0][0].Systolic)
	}
}

func TestSentTracker_SameTimestamp(t *testing.T) {
	tracker := NewSentTracker()

	now := time.Now()

	// Two records with exact same timestamp
	records := [][]device.BloodPressureRecord{
		{
			{Timestamp: now, Systolic: 120, Diastolic: 80, Pulse: 70},
		},
	}

	newRecords := tracker.FilterNewRecords(records)
	if len(newRecords[0]) != 1 {
		t.Errorf("Expected 1 new record, got %d", len(newRecords[0]))
	}

	tracker.UpdateLastSent(newRecords)

	// Same timestamp should NOT be sent again (After returns false for equal times)
	newRecords = tracker.FilterNewRecords(records)
	if len(newRecords[0]) != 0 {
		t.Errorf("Expected 0 records for same timestamp, got %d", len(newRecords[0]))
	}
}
