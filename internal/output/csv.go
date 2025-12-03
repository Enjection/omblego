package output

import (
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"

	"omblego/internal/device"
)

const (
	dateTimeFormat = "2006-01-02 15:04:05"
	csvHeader      = "datetime,dia,sys,bpm,mov,ihb"
)

// WriteCSV writes all records to CSV files (one per user)
func WriteCSV(records [][]device.BloodPressureRecord) error {
	for userIdx, userRecords := range records {
		if err := writeUserCSV(userIdx+1, userRecords); err != nil {
			return fmt.Errorf("failed to write CSV for user %d: %w", userIdx+1, err)
		}
	}
	return nil
}

// writeUserCSV writes a single user's records to a CSV file with backup
func writeUserCSV(userNum int, newRecords []device.BloodPressureRecord) error {
	filename := fmt.Sprintf("user%d.csv", userNum)

	// Read existing records if file exists
	existingRecords, err := readCSVFile(filename)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to read existing CSV: %w", err)
	}

	// If existing file exists, create a backup
	if len(existingRecords) > 0 {
		backupName := fmt.Sprintf("backup_user%d_%s.csv", userNum, time.Now().Format("2006_01_02__15_04_05"))
		if err := copyFile(filename, backupName); err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
	}

	// Merge records, avoiding duplicates based on datetime
	mergedRecords := mergeRecords(existingRecords, newRecords)

	// Sort by datetime
	sort.Slice(mergedRecords, func(i, j int) bool {
		return mergedRecords[i].Timestamp.Before(mergedRecords[j].Timestamp)
	})

	// Write merged records
	return writeCSVFile(filename, mergedRecords)
}

// readCSVFile reads records from an existing CSV file
func readCSVFile(filename string) ([]device.BloodPressureRecord, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	return parseCSV(file)
}

// parseCSV parses CSV content into records
func parseCSV(r io.Reader) ([]device.BloodPressureRecord, error) {
	reader := csv.NewReader(r)

	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	// Validate header
	if len(header) < 6 || header[0] != "datetime" {
		return nil, fmt.Errorf("invalid CSV header")
	}

	var records []device.BloodPressureRecord
	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read row: %w", err)
		}

		if len(row) < 6 {
			continue
		}

		ts, err := time.ParseInLocation(dateTimeFormat, row[0], time.Local)
		if err != nil {
			continue // Skip malformed rows
		}

		var dia, sys, bpm, mov, ihb int
		fmt.Sscanf(row[1], "%d", &dia)
		fmt.Sscanf(row[2], "%d", &sys)
		fmt.Sscanf(row[3], "%d", &bpm)
		fmt.Sscanf(row[4], "%d", &mov)
		fmt.Sscanf(row[5], "%d", &ihb)

		records = append(records, device.BloodPressureRecord{
			Timestamp: ts,
			Diastolic: dia,
			Systolic:  sys,
			Pulse:     bpm,
			Movement:  mov == 1,
			IHB:       ihb == 1,
		})
	}

	return records, nil
}

// writeCSVFile writes records to a CSV file
func writeCSVFile(filename string, records []device.BloodPressureRecord) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	return WriteCSVToWriter(file, records)
}

// WriteCSVToWriter writes records to any io.Writer
func WriteCSVToWriter(w io.Writer, records []device.BloodPressureRecord) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Write header
	if err := writer.Write([]string{"datetime", "dia", "sys", "bpm", "mov", "ihb"}); err != nil {
		return err
	}

	// Write records
	for _, rec := range records {
		mov := 0
		if rec.Movement {
			mov = 1
		}
		ihb := 0
		if rec.IHB {
			ihb = 1
		}

		row := []string{
			rec.Timestamp.Format(dateTimeFormat),
			fmt.Sprintf("%d", rec.Diastolic),
			fmt.Sprintf("%d", rec.Systolic),
			fmt.Sprintf("%d", rec.Pulse),
			fmt.Sprintf("%d", mov),
			fmt.Sprintf("%d", ihb),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}

// mergeRecords merges existing and new records, avoiding duplicates
func mergeRecords(existing, new []device.BloodPressureRecord) []device.BloodPressureRecord {
	// Create a set of new record timestamps
	newTimestamps := make(map[int64]bool)
	for _, rec := range new {
		newTimestamps[rec.Timestamp.Unix()] = true
	}

	// Start with new records
	result := make([]device.BloodPressureRecord, len(new))
	copy(result, new)

	// Add existing records that don't have matching timestamps
	for _, rec := range existing {
		if !newTimestamps[rec.Timestamp.Unix()] {
			result = append(result, rec)
		}
	}

	return result
}

// copyFile creates a copy of a file
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	// Ensure directory exists
	dir := filepath.Dir(dst)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
