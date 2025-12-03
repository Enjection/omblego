package output

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"omblego/internal/device"
)

// UBPMRecord represents a single record in UBPM format
type UBPMRecord struct {
	Date string `json:"date"`
	Time string `json:"time"`
	Msg  string `json:"msg"`
	Sys  int    `json:"sys"`
	Dia  int    `json:"dia"`
	BPM  int    `json:"bpm"`
	IHB  int    `json:"ihb"`
	MOV  int    `json:"mov"`
}

// UBPMOutput represents the full UBPM JSON structure
type UBPMOutput struct {
	UBPM map[string][]UBPMRecord `json:"UBPM"`
}

// WriteJSON writes all records to a UBPM-compatible JSON file
func WriteJSON(records [][]device.BloodPressureRecord) error {
	return WriteJSONFile("ubpm.json", records)
}

// WriteJSONFile writes records to a specific JSON file
func WriteJSONFile(filename string, records [][]device.BloodPressureRecord) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create JSON file: %w", err)
	}
	defer file.Close()

	return WriteJSONToWriter(file, records)
}

// WriteJSONToWriter writes records to any io.Writer in UBPM format
func WriteJSONToWriter(w io.Writer, records [][]device.BloodPressureRecord) error {
	output := UBPMOutput{
		UBPM: make(map[string][]UBPMRecord),
	}

	for userIdx, userRecords := range records {
		userKey := fmt.Sprintf("U%d", userIdx+1)
		ubpmRecords := make([]UBPMRecord, 0, len(userRecords))

		for _, rec := range userRecords {
			ihb := 0
			if rec.IHB {
				ihb = 1
			}
			mov := 0
			if rec.Movement {
				mov = 1
			}

			ubpmRecords = append(ubpmRecords, UBPMRecord{
				Date: rec.Timestamp.Format("02.01.2006"), // DD.MM.YYYY
				Time: rec.Timestamp.Format("15:04:05"),   // HH:MM:SS
				Msg:  "",
				Sys:  rec.Systolic,
				Dia:  rec.Diastolic,
				BPM:  rec.Pulse,
				IHB:  ihb,
				MOV:  mov,
			})
		}

		output.UBPM[userKey] = ubpmRecords
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "    ")
	return encoder.Encode(output)
}
