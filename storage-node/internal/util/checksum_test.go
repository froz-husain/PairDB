package util

import (
	"testing"
)

func TestComputeChecksum(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"simple", []byte("hello world")},
		{"binary", []byte{0x00, 0x01, 0x02, 0x03, 0xFF}},
		{"large", make([]byte, 10000)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checksum1 := ComputeChecksum(tt.data)
			checksum2 := ComputeChecksum(tt.data)

			if checksum1 != checksum2 {
				t.Errorf("Checksums should be deterministic: %d != %d", checksum1, checksum2)
			}
		})
	}
}

func TestValidateChecksum(t *testing.T) {
	data := []byte("test data for checksum validation")
	checksum := ComputeChecksum(data)

	if !ValidateChecksum(data, checksum) {
		t.Error("Valid checksum should pass validation")
	}

	if ValidateChecksum(data, checksum+1) {
		t.Error("Invalid checksum should fail validation")
	}

	// Test with corrupted data
	corruptedData := append([]byte{}, data...)
	corruptedData[0] ^= 0xFF
	if ValidateChecksum(corruptedData, checksum) {
		t.Error("Corrupted data should fail validation")
	}
}

func TestAppendAndStripChecksum(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"simple", []byte("hello world")},
		{"binary", []byte{0x00, 0x01, 0x02, 0x03, 0xFF}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Append checksum
			dataWithChecksum := AppendChecksum(tt.data)

			// Verify length
			if len(dataWithChecksum) != len(tt.data)+4 {
				t.Errorf("Expected length %d, got %d", len(tt.data)+4, len(dataWithChecksum))
			}

			// Strip and validate
			recoveredData, valid := ValidateAndStripChecksum(dataWithChecksum)
			if !valid {
				t.Error("Checksum validation failed")
			}

			// Verify data matches
			if len(recoveredData) != len(tt.data) {
				t.Errorf("Data length mismatch: expected %d, got %d", len(tt.data), len(recoveredData))
			}

			for i := range tt.data {
				if recoveredData[i] != tt.data[i] {
					t.Errorf("Data mismatch at index %d: expected %d, got %d", i, tt.data[i], recoveredData[i])
				}
			}
		})
	}
}

func TestCorruptedChecksum(t *testing.T) {
	data := []byte("test data")
	dataWithChecksum := AppendChecksum(data)

	// Corrupt the checksum
	dataWithChecksum[len(dataWithChecksum)-1] ^= 0xFF

	// Should fail validation
	_, valid := ValidateAndStripChecksum(dataWithChecksum)
	if valid {
		t.Error("Corrupted checksum should fail validation")
	}
}

func TestTooShortData(t *testing.T) {
	data := []byte{0x01, 0x02}
	_, valid := ValidateAndStripChecksum(data)
	if valid {
		t.Error("Data shorter than 4 bytes should fail validation")
	}
}

func BenchmarkComputeChecksum(b *testing.B) {
	data := make([]byte, 1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ComputeChecksum(data)
	}
}

func BenchmarkAppendChecksum(b *testing.B) {
	data := make([]byte, 1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		AppendChecksum(data)
	}
}
