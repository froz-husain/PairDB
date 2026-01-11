package service

import "github.com/devrev/pairdb/coordinator/internal/model"

// WriteResult represents the result of a write operation
type WriteResult struct {
	Success      bool
	Key          string
	VectorClock  model.VectorClock
	ReplicaCount int32
	Consistency  string
	IsDuplicate  bool
	ErrorMessage string
}

// ReadResult represents the result of a read operation
type ReadResult struct {
	Success      bool
	Key          string
	Value        []byte
	VectorClock  model.VectorClock
	ErrorMessage string
}
