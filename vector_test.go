package cogito

import (
	"testing"
)

func TestVectorScan(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		expected Vector
		wantErr  bool
	}{
		{
			name:     "scan from string",
			input:    "[0.1,0.2,0.3]",
			expected: Vector{0.1, 0.2, 0.3},
			wantErr:  false,
		},
		{
			name:     "scan from bytes",
			input:    []byte("[0.5,0.6,0.7]"),
			expected: Vector{0.5, 0.6, 0.7},
			wantErr:  false,
		},
		{
			name:     "scan nil",
			input:    nil,
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "scan empty",
			input:    "[]",
			expected: nil,
			wantErr:  false,
		},
		{
			name:     "scan with spaces",
			input:    "[0.1, 0.2, 0.3]",
			expected: Vector{0.1, 0.2, 0.3},
			wantErr:  false,
		},
		{
			name:     "scan invalid type",
			input:    123,
			expected: nil,
			wantErr:  true,
		},
		{
			name:     "scan invalid number",
			input:    "[0.1,abc,0.3]",
			expected: nil,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var v Vector
			err := v.Scan(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(v) != len(tt.expected) {
				t.Errorf("length mismatch: got %d, want %d", len(v), len(tt.expected))
				return
			}

			for i := range v {
				if v[i] != tt.expected[i] {
					t.Errorf("element %d mismatch: got %f, want %f", i, v[i], tt.expected[i])
				}
			}
		})
	}
}

func TestVectorValue(t *testing.T) {
	tests := []struct {
		name     string
		input    Vector
		expected string
	}{
		{
			name:     "simple vector",
			input:    Vector{0.1, 0.2, 0.3},
			expected: "[0.1,0.2,0.3]",
		},
		{
			name:     "nil vector",
			input:    nil,
			expected: "",
		},
		{
			name:     "single element",
			input:    Vector{0.5},
			expected: "[0.5]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := tt.input.Value()
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if tt.input == nil {
				if val != nil {
					t.Errorf("expected nil, got %v", val)
				}
				return
			}

			str, ok := val.(string)
			if !ok {
				t.Errorf("expected string, got %T", val)
				return
			}

			if str != tt.expected {
				t.Errorf("got %q, want %q", str, tt.expected)
			}
		})
	}
}

func TestVectorRoundTrip(t *testing.T) {
	original := Vector{0.123, 0.456, 0.789}

	// Convert to database format
	val, err := original.Value()
	if err != nil {
		t.Fatalf("Value() error: %v", err)
	}

	// Parse back
	var parsed Vector
	if err := parsed.Scan(val); err != nil {
		t.Fatalf("Scan() error: %v", err)
	}

	// Compare
	if len(parsed) != len(original) {
		t.Fatalf("length mismatch: got %d, want %d", len(parsed), len(original))
	}

	for i := range parsed {
		if parsed[i] != original[i] {
			t.Errorf("element %d mismatch: got %f, want %f", i, parsed[i], original[i])
		}
	}
}

func TestNewVector(t *testing.T) {
	f := []float32{1.0, 2.0, 3.0}
	v := NewVector(f)

	if len(v) != 3 {
		t.Errorf("length mismatch: got %d, want 3", len(v))
	}

	for i, val := range v {
		if val != f[i] {
			t.Errorf("element %d mismatch: got %f, want %f", i, val, f[i])
		}
	}
}

func TestVectorToFloat32(t *testing.T) {
	v := Vector{1.0, 2.0, 3.0}
	f := v.ToFloat32()

	if len(f) != 3 {
		t.Errorf("length mismatch: got %d, want 3", len(f))
	}

	for i, val := range f {
		if val != v[i] {
			t.Errorf("element %d mismatch: got %f, want %f", i, val, v[i])
		}
	}
}
