package validation

import (
	"fmt"
	"strings"
	"unicode"

	"github.com/devrev/pairdb/storage-node/internal/errors"
	"github.com/devrev/pairdb/storage-node/internal/model"
)

const (
	// Size limits
	MaxKeySize   = 1024       // 1 KB
	MaxValueSize = 10 * 1024 * 1024 // 10 MB
	MaxTenantIDSize = 256     // 256 bytes

	// Vector clock limits
	MaxVectorClockEntries = 1000
	MaxCoordinatorNodeIDSize = 128
)

// Validator validates storage operations
type Validator struct {
	maxKeySize       int
	maxValueSize     int
	maxTenantIDSize  int
}

// NewValidator creates a new validator with default limits
func NewValidator() *Validator {
	return &Validator{
		maxKeySize:      MaxKeySize,
		maxValueSize:    MaxValueSize,
		maxTenantIDSize: MaxTenantIDSize,
	}
}

// NewValidatorWithLimits creates a validator with custom limits
func NewValidatorWithLimits(maxKeySize, maxValueSize, maxTenantIDSize int) *Validator {
	return &Validator{
		maxKeySize:      maxKeySize,
		maxValueSize:    maxValueSize,
		maxTenantIDSize: maxTenantIDSize,
	}
}

// ValidateWrite validates a write operation
func (v *Validator) ValidateWrite(tenantID, key string, value []byte, vectorClock model.VectorClock) error {
	// Validate tenant ID
	if err := v.ValidateTenantID(tenantID); err != nil {
		return err
	}

	// Validate key
	if err := v.ValidateKey(key); err != nil {
		return err
	}

	// Validate value
	if err := v.ValidateValue(value); err != nil {
		return err
	}

	// Validate vector clock
	if err := v.ValidateVectorClock(vectorClock); err != nil {
		return err
	}

	return nil
}

// ValidateTenantID validates a tenant ID
func (v *Validator) ValidateTenantID(tenantID string) error {
	// Check if empty
	if tenantID == "" {
		return errors.InvalidTenantID(tenantID, "tenant ID cannot be empty")
	}

	// Check size
	if len(tenantID) > v.maxTenantIDSize {
		return errors.InvalidTenantID(tenantID, fmt.Sprintf("tenant ID exceeds maximum size of %d bytes", v.maxTenantIDSize))
	}

	// Check for forbidden characters
	// Tenant ID should not contain ':' as it's used as a separator in composite keys
	if strings.Contains(tenantID, ":") {
		return errors.InvalidTenantID(tenantID, "tenant ID cannot contain ':' character")
	}

	// Check for control characters
	for _, r := range tenantID {
		if unicode.IsControl(r) {
			return errors.InvalidTenantID(tenantID, "tenant ID cannot contain control characters")
		}
	}

	// Check for null bytes (security)
	if strings.Contains(tenantID, "\x00") {
		return errors.InvalidTenantID(tenantID, "tenant ID cannot contain null bytes")
	}

	return nil
}

// ValidateKey validates a key
func (v *Validator) ValidateKey(key string) error {
	// Check if empty
	if key == "" {
		return errors.InvalidKey(key, "key cannot be empty")
	}

	// Check size
	if len(key) > v.maxKeySize {
		return errors.KeyTooLarge(len(key), v.maxKeySize)
	}

	// Check for control characters (except tab and newline which might be intentional)
	for _, r := range key {
		if unicode.IsControl(r) && r != '\t' && r != '\n' {
			return errors.InvalidKey(key, "key cannot contain control characters")
		}
	}

	// Check for null bytes (security - prevents injection attacks)
	if strings.Contains(key, "\x00") {
		return errors.InvalidKey(key, "key cannot contain null bytes")
	}

	return nil
}

// ValidateValue validates a value
func (v *Validator) ValidateValue(value []byte) error {
	// Allow nil or empty values (they're valid for tombstones)
	if value == nil {
		return nil
	}

	// Check size
	if len(value) > v.maxValueSize {
		return errors.ValueTooLarge(len(value), v.maxValueSize)
	}

	return nil
}

// ValidateVectorClock validates a vector clock
func (v *Validator) ValidateVectorClock(vc model.VectorClock) error {
	// Check number of entries
	if len(vc.Entries) > MaxVectorClockEntries {
		return errors.InvalidArgument(
			fmt.Sprintf("vector clock has too many entries: %d > %d", len(vc.Entries), MaxVectorClockEntries),
			nil,
		)
	}

	// Validate each entry
	for i, entry := range vc.Entries {
		// Check coordinator node ID
		if entry.CoordinatorNodeID == "" {
			return errors.InvalidArgument(
				fmt.Sprintf("vector clock entry %d has empty coordinator node ID", i),
				nil,
			)
		}

		if len(entry.CoordinatorNodeID) > MaxCoordinatorNodeIDSize {
			return errors.InvalidArgument(
				fmt.Sprintf("vector clock entry %d coordinator node ID exceeds maximum size of %d", i, MaxCoordinatorNodeIDSize),
				nil,
			)
		}

		// Check for control characters and null bytes
		if strings.Contains(entry.CoordinatorNodeID, "\x00") {
			return errors.InvalidArgument(
				fmt.Sprintf("vector clock entry %d coordinator node ID contains null bytes", i),
				nil,
			)
		}

		// Logical timestamp should be non-negative
		if entry.LogicalTimestamp < 0 {
			return errors.InvalidArgument(
				fmt.Sprintf("vector clock entry %d has negative logical timestamp: %d", i, entry.LogicalTimestamp),
				nil,
			)
		}
	}

	return nil
}

// SanitizeTenantID sanitizes a tenant ID by removing forbidden characters
// This is useful for handling user input that might not be perfectly formatted
func SanitizeTenantID(tenantID string) string {
	// Remove control characters and colons
	sanitized := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == ':' {
			return -1 // Remove character
		}
		return r
	}, tenantID)

	// Trim whitespace
	sanitized = strings.TrimSpace(sanitized)

	// Limit length
	if len(sanitized) > MaxTenantIDSize {
		sanitized = sanitized[:MaxTenantIDSize]
	}

	return sanitized
}

// SanitizeKey sanitizes a key by removing dangerous characters
func SanitizeKey(key string) string {
	// Remove null bytes and most control characters
	sanitized := strings.Map(func(r rune) rune {
		if r == 0 || (unicode.IsControl(r) && r != '\t' && r != '\n') {
			return -1
		}
		return r
	}, key)

	// Trim whitespace
	sanitized = strings.TrimSpace(sanitized)

	// Limit length
	if len(sanitized) > MaxKeySize {
		sanitized = sanitized[:MaxKeySize]
	}

	return sanitized
}

// EstimateWriteSize estimates the disk space needed for a write operation
// This is used by the disk manager to check available space
func EstimateWriteSize(tenantID, key string, value []byte) uint64 {
	// Rough estimation including overhead
	// Commit log: JSON serialization + metadata + checksum
	commitLogSize := len(tenantID) + len(key) + len(value) + 200 // 200 bytes for overhead

	// MemTable: in-memory structure overhead
	memTableSize := len(tenantID) + len(key) + len(value) + 100

	// Assume eventual SSTable flush (amortized)
	sstableSize := len(tenantID) + len(key) + len(value) + 50

	// Total with safety margin (20%)
	total := uint64(commitLogSize + memTableSize + sstableSize)
	return total + (total / 5)
}
