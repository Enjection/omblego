package output

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"omblego/internal/device"
)

func TestWriteCSVToWriter(t *testing.T) {
	records := []device.BloodPressureRecord{
		{
			Timestamp: time.Date(2024, 1, 15, 8, 30, 0, 0, time.Local),
			Systolic:  120,
			Diastolic: 80,
			Pulse:     72,
			Movement:  false,
			IHB:       false,
		},
		{
			Timestamp: time.Date(2024, 1, 16, 7, 45, 30, 0, time.Local),
			Systolic:  125,
			Diastolic: 82,
			Pulse:     68,
			Movement:  true,
			IHB:       false,
		},
		{
			Timestamp: time.Date(2024, 1, 17, 9, 0, 0, 0, time.Local),
			Systolic:  118,
			Diastolic: 78,
			Pulse:     70,
			Movement:  false,
			IHB:       true,
		},
	}

	var buf bytes.Buffer
	err := WriteCSVToWriter(&buf, records)
	if err != nil {
		t.Fatalf("WriteCSVToWriter error: %v", err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Check header
	if lines[0] != "datetime,dia,sys,bpm,mov,ihb" {
		t.Errorf("Header = %q, want 'datetime,dia,sys,bpm,mov,ihb'", lines[0])
	}

	// Check number of lines (header + 3 records)
	if len(lines) != 4 {
		t.Errorf("Line count = %d, want 4", len(lines))
	}

	// Check first record
	expectedFirst := "2024-01-15 08:30:00,80,120,72,0,0"
	if lines[1] != expectedFirst {
		t.Errorf("First record = %q, want %q", lines[1], expectedFirst)
	}

	// Check second record (with movement flag)
	expectedSecond := "2024-01-16 07:45:30,82,125,68,1,0"
	if lines[2] != expectedSecond {
		t.Errorf("Second record = %q, want %q", lines[2], expectedSecond)
	}

	// Check third record (with IHB flag)
	expectedThird := "2024-01-17 09:00:00,78,118,70,0,1"
	if lines[3] != expectedThird {
		t.Errorf("Third record = %q, want %q", lines[3], expectedThird)
	}
}

func TestWriteCSVToWriter_EmptyRecords(t *testing.T) {
	var buf bytes.Buffer
	err := WriteCSVToWriter(&buf, []device.BloodPressureRecord{})
	if err != nil {
		t.Fatalf("WriteCSVToWriter error: %v", err)
	}

	output := buf.String()
	lines := strings.Split(strings.TrimSpace(output), "\n")

	// Should only have header
	if len(lines) != 1 {
		t.Errorf("Line count = %d, want 1 (header only)", len(lines))
	}
}

func TestParseCSV(t *testing.T) {
	csvData := `datetime,dia,sys,bpm,mov,ihb
2024-01-15 08:30:00,80,120,72,0,0
2024-01-16 07:45:30,82,125,68,1,0
2024-01-17 09:00:00,78,118,70,0,1`

	reader := strings.NewReader(csvData)
	records, err := parseCSV(reader)
	if err != nil {
		t.Fatalf("parseCSV error: %v", err)
	}

	if len(records) != 3 {
		t.Errorf("Record count = %d, want 3", len(records))
	}

	// Check first record
	r := records[0]
	if r.Systolic != 120 {
		t.Errorf("Record 0 Systolic = %d, want 120", r.Systolic)
	}
	if r.Diastolic != 80 {
		t.Errorf("Record 0 Diastolic = %d, want 80", r.Diastolic)
	}
	if r.Pulse != 72 {
		t.Errorf("Record 0 Pulse = %d, want 72", r.Pulse)
	}
	if r.Movement != false {
		t.Errorf("Record 0 Movement = %v, want false", r.Movement)
	}
	if r.IHB != false {
		t.Errorf("Record 0 IHB = %v, want false", r.IHB)
	}

	// Check second record (with movement)
	r = records[1]
	if r.Movement != true {
		t.Errorf("Record 1 Movement = %v, want true", r.Movement)
	}

	// Check third record (with IHB)
	r = records[2]
	if r.IHB != true {
		t.Errorf("Record 2 IHB = %v, want true", r.IHB)
	}
}

func TestMergeRecords(t *testing.T) {
	existing := []device.BloodPressureRecord{
		{
			Timestamp: time.Date(2024, 1, 15, 8, 30, 0, 0, time.Local),
			Systolic:  120,
			Diastolic: 80,
			Pulse:     72,
		},
		{
			Timestamp: time.Date(2024, 1, 16, 7, 45, 30, 0, time.Local),
			Systolic:  125,
			Diastolic: 82,
			Pulse:     68,
		},
	}

	new := []device.BloodPressureRecord{
		{
			Timestamp: time.Date(2024, 1, 16, 7, 45, 30, 0, time.Local), // Duplicate
			Systolic:  125,
			Diastolic: 82,
			Pulse:     68,
		},
		{
			Timestamp: time.Date(2024, 1, 17, 9, 0, 0, 0, time.Local), // New
			Systolic:  118,
			Diastolic: 78,
			Pulse:     70,
		},
	}

	merged := mergeRecords(existing, new)

	// Should have 3 unique records (1 duplicate removed)
	if len(merged) != 3 {
		t.Errorf("Merged count = %d, want 3", len(merged))
	}

	// Verify all timestamps are unique
	seen := make(map[int64]bool)
	for _, r := range merged {
		ts := r.Timestamp.Unix()
		if seen[ts] {
			t.Errorf("Duplicate timestamp found: %v", r.Timestamp)
		}
		seen[ts] = true
	}
}

func TestMergeRecords_AllNew(t *testing.T) {
	existing := []device.BloodPressureRecord{}

	new := []device.BloodPressureRecord{
		{Timestamp: time.Date(2024, 1, 15, 8, 30, 0, 0, time.Local), Systolic: 120},
		{Timestamp: time.Date(2024, 1, 16, 7, 45, 30, 0, time.Local), Systolic: 125},
	}

	merged := mergeRecords(existing, new)

	if len(merged) != 2 {
		t.Errorf("Merged count = %d, want 2", len(merged))
	}
}

func TestMergeRecords_AllDuplicates(t *testing.T) {
	ts := time.Date(2024, 1, 15, 8, 30, 0, 0, time.Local)

	existing := []device.BloodPressureRecord{
		{Timestamp: ts, Systolic: 120},
	}

	new := []device.BloodPressureRecord{
		{Timestamp: ts, Systolic: 120}, // Same timestamp
	}

	merged := mergeRecords(existing, new)

	if len(merged) != 1 {
		t.Errorf("Merged count = %d, want 1 (no duplicates)", len(merged))
	}
}

func TestCSVRoundTrip(t *testing.T) {
	// Create some records
	original := []device.BloodPressureRecord{
		{
			Timestamp: time.Date(2024, 1, 15, 8, 30, 0, 0, time.Local),
			Systolic:  120,
			Diastolic: 80,
			Pulse:     72,
			Movement:  false,
			IHB:       false,
		},
		{
			Timestamp: time.Date(2024, 1, 16, 7, 45, 30, 0, time.Local),
			Systolic:  125,
			Diastolic: 82,
			Pulse:     68,
			Movement:  true,
			IHB:       true,
		},
	}

	// Write to CSV
	var buf bytes.Buffer
	err := WriteCSVToWriter(&buf, original)
	if err != nil {
		t.Fatalf("WriteCSVToWriter error: %v", err)
	}

	// Read back
	parsed, err := parseCSV(strings.NewReader(buf.String()))
	if err != nil {
		t.Fatalf("parseCSV error: %v", err)
	}

	// Compare
	if len(parsed) != len(original) {
		t.Fatalf("Parsed count = %d, want %d", len(parsed), len(original))
	}

	for i := range original {
		if parsed[i].Systolic != original[i].Systolic {
			t.Errorf("Record %d Systolic = %d, want %d", i, parsed[i].Systolic, original[i].Systolic)
		}
		if parsed[i].Diastolic != original[i].Diastolic {
			t.Errorf("Record %d Diastolic = %d, want %d", i, parsed[i].Diastolic, original[i].Diastolic)
		}
		if parsed[i].Pulse != original[i].Pulse {
			t.Errorf("Record %d Pulse = %d, want %d", i, parsed[i].Pulse, original[i].Pulse)
		}
		if parsed[i].Movement != original[i].Movement {
			t.Errorf("Record %d Movement = %v, want %v", i, parsed[i].Movement, original[i].Movement)
		}
		if parsed[i].IHB != original[i].IHB {
			t.Errorf("Record %d IHB = %v, want %v", i, parsed[i].IHB, original[i].IHB)
		}
	}
}
