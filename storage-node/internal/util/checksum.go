package util

import (
	"hash/crc32"
)

// Checksum utilities for data integrity validation
// Uses CRC32 (IEEE polynomial) for fast checksum computation

var (
	// crc32Table is precomputed for better performance
	crc32Table = crc32.MakeTable(crc32.IEEE)
)

// ComputeChecksum computes a CRC32 checksum for the given data
// Returns a 32-bit checksum value
func ComputeChecksum(data []byte) uint32 {
	return crc32.Checksum(data, crc32Table)
}

// ValidateChecksum validates data against an expected checksum
// Returns true if the checksum matches, false otherwise
func ValidateChecksum(data []byte, expected uint32) bool {
	actual := ComputeChecksum(data)
	return actual == expected
}

// AppendChecksum appends a 4-byte checksum to the data
// Format: [data][checksum (4 bytes)]
func AppendChecksum(data []byte) []byte {
	checksum := ComputeChecksum(data)
	result := make([]byte, len(data)+4)
	copy(result, data)
	result[len(data)] = byte(checksum)
	result[len(data)+1] = byte(checksum >> 8)
	result[len(data)+2] = byte(checksum >> 16)
	result[len(data)+3] = byte(checksum >> 24)
	return result
}

// ValidateAndStripChecksum validates the checksum and returns data without checksum
// Format: [data][checksum (4 bytes)]
// Returns (data, valid) where valid indicates if checksum was correct
func ValidateAndStripChecksum(dataWithChecksum []byte) ([]byte, bool) {
	if len(dataWithChecksum) < 4 {
		return nil, false
	}

	// Extract checksum (last 4 bytes)
	dataLen := len(dataWithChecksum) - 4
	data := dataWithChecksum[:dataLen]
	expectedChecksum := uint32(dataWithChecksum[dataLen]) |
		uint32(dataWithChecksum[dataLen+1])<<8 |
		uint32(dataWithChecksum[dataLen+2])<<16 |
		uint32(dataWithChecksum[dataLen+3])<<24

	// Validate
	valid := ValidateChecksum(data, expectedChecksum)
	return data, valid
}
