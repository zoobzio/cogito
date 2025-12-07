package cogito

import (
	"database/sql/driver"
	"fmt"
	"strconv"
	"strings"
)

// Vector represents a pgvector embedding.
// Implements sql.Scanner and driver.Valuer for database compatibility.
type Vector []float32

// Scan implements sql.Scanner for reading vectors from the database.
func (v *Vector) Scan(src any) error {
	if src == nil {
		*v = nil
		return nil
	}

	var s string
	switch val := src.(type) {
	case []byte:
		s = string(val)
	case string:
		s = val
	default:
		return fmt.Errorf("cannot scan %T into Vector", src)
	}

	// pgvector format: [0.1,0.2,0.3]
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")

	if s == "" {
		*v = nil
		return nil
	}

	parts := strings.Split(s, ",")
	result := make([]float32, len(parts))
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 32)
		if err != nil {
			return fmt.Errorf("failed to parse vector element %d: %w", i, err)
		}
		result[i] = float32(f)
	}

	*v = result
	return nil
}

// Value implements driver.Valuer for writing vectors to the database.
func (v Vector) Value() (driver.Value, error) {
	if v == nil {
		return nil, nil
	}

	// pgvector format: [0.1,0.2,0.3]
	parts := make([]string, len(v))
	for i, f := range v {
		parts[i] = strconv.FormatFloat(float64(f), 'f', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]", nil
}

// ToFloat32 returns the underlying []float32 slice.
func (v Vector) ToFloat32() []float32 {
	return v
}

// NewVector creates a Vector from a []float32 slice.
func NewVector(f []float32) Vector {
	return Vector(f)
}
