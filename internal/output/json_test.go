package output

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"omblego/internal/device"
)

func TestWriteJSONToWriter(t *testing.T) {
	records := [][]device.BloodPressureRecord{
		// User 1
		{
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
		},
		// User 2
		{
			{
				Timestamp: time.Date(2024, 1, 17, 9, 0, 0, 0, time.Local),
				Systolic:  118,
				Diastolic: 78,
				Pulse:     70,
				Movement:  false,
				IHB:       true,
			},
		},
	}

	var buf bytes.Buffer
	err := WriteJSONToWriter(&buf, records)
	if err != nil {
		t.Fatalf("WriteJSONToWriter error: %v", err)
	}

	// Parse the JSON
	var output UBPMOutput
	err = json.Unmarshal(buf.Bytes(), &output)
	if err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	// Check user 1
	u1, ok := output.UBPM["U1"]
	if !ok {
		t.Fatal("U1 not found in output")
	}
	if len(u1) != 2 {
		t.Errorf("U1 record count = %d, want 2", len(u1))
	}

	// Check first record format
	r := u1[0]
	if r.Date != "15.01.2024" {
		t.Errorf("Date = %s, want 15.01.2024", r.Date)
	}
	if r.Time != "08:30:00" {
		t.Errorf("Time = %s, want 08:30:00", r.Time)
	}
	if r.Sys != 120 {
		t.Errorf("Sys = %d, want 120", r.Sys)
	}
	if r.Dia != 80 {
		t.Errorf("Dia = %d, want 80", r.Dia)
	}
	if r.BPM != 72 {
		t.Errorf("BPM = %d, want 72", r.BPM)
	}
	if r.MOV != 0 {
		t.Errorf("MOV = %d, want 0", r.MOV)
	}
	if r.IHB != 0 {
		t.Errorf("IHB = %d, want 0", r.IHB)
	}
	if r.Msg != "" {
		t.Errorf("Msg = %q, want empty", r.Msg)
	}

	// Check second record (with movement)
	r = u1[1]
	if r.MOV != 1 {
		t.Errorf("MOV = %d, want 1", r.MOV)
	}

	// Check user 2
	u2, ok := output.UBPM["U2"]
	if !ok {
		t.Fatal("U2 not found in output")
	}
	if len(u2) != 1 {
		t.Errorf("U2 record count = %d, want 1", len(u2))
	}

	// Check user 2 record (with IHB)
	r = u2[0]
	if r.IHB != 1 {
		t.Errorf("IHB = %d, want 1", r.IHB)
	}
}

func TestWriteJSONToWriter_EmptyRecords(t *testing.T) {
	records := [][]device.BloodPressureRecord{
		{}, // Empty user 1
		{}, // Empty user 2
	}

	var buf bytes.Buffer
	err := WriteJSONToWriter(&buf, records)
	if err != nil {
		t.Fatalf("WriteJSONToWriter error: %v", err)
	}

	var output UBPMOutput
	err = json.Unmarshal(buf.Bytes(), &output)
	if err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	// Should have both users with empty arrays
	u1, ok := output.UBPM["U1"]
	if !ok {
		t.Fatal("U1 not found")
	}
	if len(u1) != 0 {
		t.Errorf("U1 should be empty, got %d records", len(u1))
	}

	u2, ok := output.UBPM["U2"]
	if !ok {
		t.Fatal("U2 not found")
	}
	if len(u2) != 0 {
		t.Errorf("U2 should be empty, got %d records", len(u2))
	}
}

func TestWriteJSONToWriter_SingleUser(t *testing.T) {
	records := [][]device.BloodPressureRecord{
		{
			{
				Timestamp: time.Date(2024, 6, 15, 14, 30, 45, 0, time.Local),
				Systolic:  115,
				Diastolic: 75,
				Pulse:     65,
				Movement:  false,
				IHB:       false,
			},
		},
	}

	var buf bytes.Buffer
	err := WriteJSONToWriter(&buf, records)
	if err != nil {
		t.Fatalf("WriteJSONToWriter error: %v", err)
	}

	var output UBPMOutput
	err = json.Unmarshal(buf.Bytes(), &output)
	if err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	// Should only have U1
	if len(output.UBPM) != 1 {
		t.Errorf("User count = %d, want 1", len(output.UBPM))
	}

	u1, ok := output.UBPM["U1"]
	if !ok {
		t.Fatal("U1 not found")
	}
	if len(u1) != 1 {
		t.Errorf("U1 record count = %d, want 1", len(u1))
	}
}

func TestUBPMDateFormat(t *testing.T) {
	records := [][]device.BloodPressureRecord{
		{
			{
				Timestamp: time.Date(2024, 12, 5, 9, 5, 3, 0, time.Local),
				Systolic:  120,
				Diastolic: 80,
				Pulse:     72,
			},
		},
	}

	var buf bytes.Buffer
	err := WriteJSONToWriter(&buf, records)
	if err != nil {
		t.Fatalf("WriteJSONToWriter error: %v", err)
	}

	var output UBPMOutput
	err = json.Unmarshal(buf.Bytes(), &output)
	if err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	r := output.UBPM["U1"][0]

	// Date should be DD.MM.YYYY (with leading zeros)
	if r.Date != "05.12.2024" {
		t.Errorf("Date = %s, want 05.12.2024", r.Date)
	}

	// Time should be HH:MM:SS (with leading zeros)
	if r.Time != "09:05:03" {
		t.Errorf("Time = %s, want 09:05:03", r.Time)
	}
}

func TestJSONStructure(t *testing.T) {
	records := [][]device.BloodPressureRecord{
		{
			{
				Timestamp: time.Date(2024, 1, 1, 0, 0, 0, 0, time.Local),
				Systolic:  100,
				Diastolic: 60,
				Pulse:     60,
			},
		},
	}

	var buf bytes.Buffer
	err := WriteJSONToWriter(&buf, records)
	if err != nil {
		t.Fatalf("WriteJSONToWriter error: %v", err)
	}

	// Parse as generic map to verify structure
	var raw map[string]interface{}
	err = json.Unmarshal(buf.Bytes(), &raw)
	if err != nil {
		t.Fatalf("JSON parse error: %v", err)
	}

	// Should have "UBPM" as top-level key
	ubpm, ok := raw["UBPM"]
	if !ok {
		t.Fatal("UBPM key not found at top level")
	}

	// UBPM should be an object
	ubpmMap, ok := ubpm.(map[string]interface{})
	if !ok {
		t.Fatal("UBPM is not an object")
	}

	// Should have U1
	u1, ok := ubpmMap["U1"]
	if !ok {
		t.Fatal("U1 not found under UBPM")
	}

	// U1 should be an array
	u1Arr, ok := u1.([]interface{})
	if !ok {
		t.Fatal("U1 is not an array")
	}

	if len(u1Arr) != 1 {
		t.Errorf("U1 length = %d, want 1", len(u1Arr))
	}
}

func TestUBPMRecordFields(t *testing.T) {
	record := UBPMRecord{
		Date: "15.01.2024",
		Time: "08:30:00",
		Msg:  "test message",
		Sys:  120,
		Dia:  80,
		BPM:  72,
		IHB:  1,
		MOV:  0,
	}

	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}

	// Verify JSON field names
	var parsed map[string]interface{}
	err = json.Unmarshal(data, &parsed)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	expectedFields := []string{"date", "time", "msg", "sys", "dia", "bpm", "ihb", "mov"}
	for _, field := range expectedFields {
		if _, ok := parsed[field]; !ok {
			t.Errorf("Field %q not found in JSON", field)
		}
	}
}
