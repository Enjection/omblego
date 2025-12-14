// Package omblego provides a public API for syncing blood pressure data
// from Omron BLE blood pressure monitors.
package omblego

import (
	"time"

	"omblego/internal/device"
)

// BloodPressureRecord represents a single blood pressure reading.
type BloodPressureRecord struct {
	// Timestamp is when the measurement was taken
	Timestamp time.Time

	// Systolic is the systolic pressure in mmHg
	Systolic int

	// Diastolic is the diastolic pressure in mmHg
	Diastolic int

	// Pulse is the heart rate in beats per minute
	Pulse int

	// Movement indicates if movement was detected during measurement
	Movement bool

	// IHB indicates if an irregular heartbeat was detected
	IHB bool
}

// FromInternal converts an internal record to a public record.
func FromInternal(r device.BloodPressureRecord) BloodPressureRecord {
	return BloodPressureRecord{
		Timestamp: r.Timestamp,
		Systolic:  r.Systolic,
		Diastolic: r.Diastolic,
		Pulse:     r.Pulse,
		Movement:  r.Movement,
		IHB:       r.IHB,
	}
}

// ToInternal converts a public record to an internal record.
func (r BloodPressureRecord) ToInternal() device.BloodPressureRecord {
	return device.BloodPressureRecord{
		Timestamp: r.Timestamp,
		Systolic:  r.Systolic,
		Diastolic: r.Diastolic,
		Pulse:     r.Pulse,
		Movement:  r.Movement,
		IHB:       r.IHB,
	}
}

// FromInternalSlice converts a slice of internal records to public records.
func FromInternalSlice(records []device.BloodPressureRecord) []BloodPressureRecord {
	result := make([]BloodPressureRecord, len(records))
	for i, r := range records {
		result[i] = FromInternal(r)
	}
	return result
}

// FromInternalUserRecords converts per-user internal records to public records.
func FromInternalUserRecords(userRecords [][]device.BloodPressureRecord) [][]BloodPressureRecord {
	result := make([][]BloodPressureRecord, len(userRecords))
	for i, records := range userRecords {
		result[i] = FromInternalSlice(records)
	}
	return result
}

// UserRecords represents records organized by user.
// Index 0 is User 1, index 1 is User 2, etc.
type UserRecords [][]BloodPressureRecord

// TotalCount returns the total number of records across all users.
func (ur UserRecords) TotalCount() int {
	count := 0
	for _, records := range ur {
		count += len(records)
	}
	return count
}

// UserCount returns the number of users with records.
func (ur UserRecords) UserCount() int {
	count := 0
	for _, records := range ur {
		if len(records) > 0 {
			count++
		}
	}
	return count
}

// Latest returns the most recent record across all users, or nil if no records.
func (ur UserRecords) Latest() *BloodPressureRecord {
	var latest *BloodPressureRecord
	for _, records := range ur {
		for i := range records {
			if latest == nil || records[i].Timestamp.After(latest.Timestamp) {
				latest = &records[i]
			}
		}
	}
	return latest
}
