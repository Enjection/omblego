// Package testutil provides shared test utilities, fixtures, and helpers
// for testing omblego components.
package testutil

import (
	"math/rand"
	"time"

	"omblego/internal/device"
)

// RecordGenerator generates test blood pressure records
type RecordGenerator struct {
	baseTime  time.Time
	rng       *rand.Rand
	userCount int
}

// NewRecordGenerator creates a new record generator
func NewRecordGenerator() *RecordGenerator {
	return &RecordGenerator{
		baseTime:  time.Now().Truncate(time.Second),
		rng:       rand.New(rand.NewSource(time.Now().UnixNano())),
		userCount: 1,
	}
}

// WithBaseTime sets the base time for generated records
func (g *RecordGenerator) WithBaseTime(t time.Time) *RecordGenerator {
	g.baseTime = t.Truncate(time.Second)
	return g
}

// WithSeed sets the random seed for reproducible tests
func (g *RecordGenerator) WithSeed(seed int64) *RecordGenerator {
	g.rng = rand.New(rand.NewSource(seed))
	return g
}

// WithUserCount sets the number of users to generate records for
func (g *RecordGenerator) WithUserCount(count int) *RecordGenerator {
	g.userCount = count
	return g
}

// GenerateRecord generates a single blood pressure record
func (g *RecordGenerator) GenerateRecord(offsetMinutes int) device.BloodPressureRecord {
	return device.BloodPressureRecord{
		Timestamp: g.baseTime.Add(time.Duration(offsetMinutes) * time.Minute),
		Systolic:  100 + g.rng.Intn(40),  // 100-139
		Diastolic: 60 + g.rng.Intn(30),   // 60-89
		Pulse:     60 + g.rng.Intn(40),   // 60-99
		Movement:  g.rng.Float32() < 0.1, // 10% chance
		IHB:       g.rng.Float32() < 0.05, // 5% chance
	}
}

// GenerateRecords generates n records for a single user
func (g *RecordGenerator) GenerateRecords(n int) []device.BloodPressureRecord {
	records := make([]device.BloodPressureRecord, n)
	for i := 0; i < n; i++ {
		records[i] = g.GenerateRecord(i * 60) // 1 hour apart
	}
	return records
}

// GenerateUserRecords generates records for multiple users
func (g *RecordGenerator) GenerateUserRecords(recordsPerUser int) [][]device.BloodPressureRecord {
	result := make([][]device.BloodPressureRecord, g.userCount)
	for u := 0; u < g.userCount; u++ {
		result[u] = g.GenerateRecords(recordsPerUser)
	}
	return result
}

// FixedRecord returns a record with specific values for deterministic tests
func FixedRecord(timestamp time.Time, systolic, diastolic, pulse int) device.BloodPressureRecord {
	return device.BloodPressureRecord{
		Timestamp: timestamp,
		Systolic:  systolic,
		Diastolic: diastolic,
		Pulse:     pulse,
		Movement:  false,
		IHB:       false,
	}
}

// NormalReading returns a typical normal blood pressure reading
func NormalReading(timestamp time.Time) device.BloodPressureRecord {
	return FixedRecord(timestamp, 120, 80, 70)
}

// HighReading returns a typical high blood pressure reading
func HighReading(timestamp time.Time) device.BloodPressureRecord {
	return FixedRecord(timestamp, 150, 95, 85)
}

// LowReading returns a typical low blood pressure reading
func LowReading(timestamp time.Time) device.BloodPressureRecord {
	return FixedRecord(timestamp, 90, 60, 55)
}

// SampleRecords returns a small set of sample records for basic tests
func SampleRecords() [][]device.BloodPressureRecord {
	baseTime := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)
	return [][]device.BloodPressureRecord{
		{
			NormalReading(baseTime),
			NormalReading(baseTime.Add(time.Hour)),
			HighReading(baseTime.Add(2 * time.Hour)),
		},
	}
}

// SampleRecordsTwoUsers returns sample records for two users
func SampleRecordsTwoUsers() [][]device.BloodPressureRecord {
	baseTime := time.Date(2024, 1, 15, 8, 0, 0, 0, time.UTC)
	return [][]device.BloodPressureRecord{
		{
			NormalReading(baseTime),
			NormalReading(baseTime.Add(time.Hour)),
		},
		{
			HighReading(baseTime.Add(30 * time.Minute)),
			LowReading(baseTime.Add(90 * time.Minute)),
		},
	}
}
