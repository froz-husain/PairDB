package model

import "time"

// Tenant represents tenant configuration
type Tenant struct {
	TenantID          string
	ReplicationFactor int
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Version           int64 // For optimistic locking
}
