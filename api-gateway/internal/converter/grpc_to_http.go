// Package converter provides HTTP to gRPC and gRPC to HTTP conversion utilities.
package converter

import (
	pb "github.com/devrev/pairdb/api-gateway/pkg/proto"
)

// GRPCToHTTP handles conversion of gRPC responses to HTTP responses.
type GRPCToHTTP struct{}

// NewGRPCToHTTP creates a new GRPCToHTTP converter.
func NewGRPCToHTTP() *GRPCToHTTP {
	return &GRPCToHTTP{}
}

// WriteKeyValueHTTPResponse represents the HTTP response for WriteKeyValue.
type WriteKeyValueHTTPResponse struct {
	Status         string                 `json:"status"`
	Key            string                 `json:"key"`
	IdempotencyKey string                 `json:"idempotency_key"`
	VectorClock    map[string]int64       `json:"vector_clock,omitempty"`
	ReplicaCount   int32                  `json:"replica_count"`
	Consistency    string                 `json:"consistency"`
	IsDuplicate    bool                   `json:"is_duplicate"`
}

// ReadKeyValueHTTPResponse represents the HTTP response for ReadKeyValue.
type ReadKeyValueHTTPResponse struct {
	Status      string           `json:"status"`
	Key         string           `json:"key"`
	Value       string           `json:"value"`
	VectorClock map[string]int64 `json:"vector_clock,omitempty"`
}

// CreateTenantHTTPResponse represents the HTTP response for CreateTenant.
type CreateTenantHTTPResponse struct {
	Status            string `json:"status"`
	TenantID          string `json:"tenant_id"`
	ReplicationFactor int32  `json:"replication_factor"`
	CreatedAt         int64  `json:"created_at"`
}

// UpdateReplicationFactorHTTPResponse represents the HTTP response for UpdateReplicationFactor.
type UpdateReplicationFactorHTTPResponse struct {
	Status               string `json:"status"`
	TenantID             string `json:"tenant_id"`
	OldReplicationFactor int32  `json:"old_replication_factor"`
	NewReplicationFactor int32  `json:"new_replication_factor"`
	MigrationID          string `json:"migration_id,omitempty"`
	UpdatedAt            int64  `json:"updated_at"`
}

// GetTenantHTTPResponse represents the HTTP response for GetTenant.
type GetTenantHTTPResponse struct {
	Status            string `json:"status"`
	TenantID          string `json:"tenant_id"`
	ReplicationFactor int32  `json:"replication_factor"`
	CreatedAt         int64  `json:"created_at"`
	UpdatedAt         int64  `json:"updated_at"`
}

// AddStorageNodeHTTPResponse represents the HTTP response for AddStorageNode.
type AddStorageNodeHTTPResponse struct {
	Status              string `json:"status"`
	NodeID              string `json:"node_id"`
	MigrationID         string `json:"migration_id,omitempty"`
	Message             string `json:"message"`
	EstimatedCompletion int64  `json:"estimated_completion,omitempty"`
}

// RemoveStorageNodeHTTPResponse represents the HTTP response for RemoveStorageNode.
type RemoveStorageNodeHTTPResponse struct {
	Status              string `json:"status"`
	NodeID              string `json:"node_id"`
	MigrationID         string `json:"migration_id,omitempty"`
	Message             string `json:"message"`
	EstimatedCompletion int64  `json:"estimated_completion,omitempty"`
}

// MigrationProgressHTTPResponse represents the HTTP response for migration progress.
type MigrationProgressHTTPResponse struct {
	KeysMigrated int64   `json:"keys_migrated"`
	TotalKeys    int64   `json:"total_keys"`
	Percentage   float32 `json:"percentage"`
}

// GetMigrationStatusHTTPResponse represents the HTTP response for GetMigrationStatus.
type GetMigrationStatusHTTPResponse struct {
	Status              string                         `json:"status"`
	MigrationID         string                         `json:"migration_id"`
	Type                string                         `json:"type"`
	NodeID              string                         `json:"node_id"`
	MigrationStatus     string                         `json:"migration_status"`
	Progress            *MigrationProgressHTTPResponse `json:"progress,omitempty"`
	StartedAt           int64                          `json:"started_at"`
	EstimatedCompletion int64                          `json:"estimated_completion,omitempty"`
}

// StorageNodeInfoHTTPResponse represents the HTTP response for storage node info.
type StorageNodeInfoHTTPResponse struct {
	NodeID           string  `json:"node_id"`
	Host             string  `json:"host"`
	Port             int32   `json:"port"`
	Status           string  `json:"status"`
	VirtualNodes     int32   `json:"virtual_nodes"`
	KeysCount        int64   `json:"keys_count"`
	DiskUsagePercent float32 `json:"disk_usage_percent"`
}

// ListStorageNodesHTTPResponse represents the HTTP response for ListStorageNodes.
type ListStorageNodesHTTPResponse struct {
	Status string                        `json:"status"`
	Nodes  []StorageNodeInfoHTTPResponse `json:"nodes"`
}

// WriteKeyValueResponse converts a gRPC WriteKeyValueResponse to an HTTP response.
func (c *GRPCToHTTP) WriteKeyValueResponse(resp *pb.WriteKeyValueResponse) *WriteKeyValueHTTPResponse {
	return &WriteKeyValueHTTPResponse{
		Status:         getStatus(resp.Success),
		Key:            resp.Key,
		IdempotencyKey: resp.IdempotencyKey,
		VectorClock:    convertVectorClock(resp.VectorClock),
		ReplicaCount:   resp.ReplicaCount,
		Consistency:    resp.Consistency,
		IsDuplicate:    resp.IsDuplicate,
	}
}

// ReadKeyValueResponse converts a gRPC ReadKeyValueResponse to an HTTP response.
func (c *GRPCToHTTP) ReadKeyValueResponse(resp *pb.ReadKeyValueResponse) *ReadKeyValueHTTPResponse {
	return &ReadKeyValueHTTPResponse{
		Status:      getStatus(resp.Success),
		Key:         resp.Key,
		Value:       string(resp.Value),
		VectorClock: convertVectorClock(resp.VectorClock),
	}
}

// CreateTenantResponse converts a gRPC CreateTenantResponse to an HTTP response.
func (c *GRPCToHTTP) CreateTenantResponse(resp *pb.CreateTenantResponse) *CreateTenantHTTPResponse {
	return &CreateTenantHTTPResponse{
		Status:            getStatus(resp.Success),
		TenantID:          resp.TenantId,
		ReplicationFactor: resp.ReplicationFactor,
		CreatedAt:         resp.CreatedAt,
	}
}

// UpdateReplicationFactorResponse converts a gRPC UpdateReplicationFactorResponse to an HTTP response.
func (c *GRPCToHTTP) UpdateReplicationFactorResponse(resp *pb.UpdateReplicationFactorResponse) *UpdateReplicationFactorHTTPResponse {
	return &UpdateReplicationFactorHTTPResponse{
		Status:               getStatus(resp.Success),
		TenantID:             resp.TenantId,
		OldReplicationFactor: resp.OldReplicationFactor,
		NewReplicationFactor: resp.NewReplicationFactor,
		MigrationID:          resp.MigrationId,
		UpdatedAt:            resp.UpdatedAt,
	}
}

// GetTenantResponse converts a gRPC GetTenantResponse to an HTTP response.
func (c *GRPCToHTTP) GetTenantResponse(resp *pb.GetTenantResponse) *GetTenantHTTPResponse {
	return &GetTenantHTTPResponse{
		Status:            getStatus(resp.Success),
		TenantID:          resp.TenantId,
		ReplicationFactor: resp.ReplicationFactor,
		CreatedAt:         resp.CreatedAt,
		UpdatedAt:         resp.UpdatedAt,
	}
}

// AddStorageNodeResponse converts a gRPC AddStorageNodeResponse to an HTTP response.
func (c *GRPCToHTTP) AddStorageNodeResponse(resp *pb.AddStorageNodeResponse) *AddStorageNodeHTTPResponse {
	return &AddStorageNodeHTTPResponse{
		Status:              getStatus(resp.Success),
		NodeID:              resp.NodeId,
		MigrationID:         resp.MigrationId,
		Message:             resp.Message,
		EstimatedCompletion: resp.EstimatedCompletion,
	}
}

// RemoveStorageNodeResponse converts a gRPC RemoveStorageNodeResponse to an HTTP response.
func (c *GRPCToHTTP) RemoveStorageNodeResponse(resp *pb.RemoveStorageNodeResponse) *RemoveStorageNodeHTTPResponse {
	return &RemoveStorageNodeHTTPResponse{
		Status:              getStatus(resp.Success),
		NodeID:              resp.NodeId,
		MigrationID:         resp.MigrationId,
		Message:             resp.Message,
		EstimatedCompletion: resp.EstimatedCompletion,
	}
}

// GetMigrationStatusResponse converts a gRPC GetMigrationStatusResponse to an HTTP response.
func (c *GRPCToHTTP) GetMigrationStatusResponse(resp *pb.GetMigrationStatusResponse) *GetMigrationStatusHTTPResponse {
	httpResp := &GetMigrationStatusHTTPResponse{
		Status:              getStatus(resp.Success),
		MigrationID:         resp.MigrationId,
		Type:                resp.Type,
		NodeID:              resp.NodeId,
		MigrationStatus:     resp.Status,
		StartedAt:           resp.StartedAt,
		EstimatedCompletion: resp.EstimatedCompletion,
	}

	if resp.Progress != nil {
		httpResp.Progress = &MigrationProgressHTTPResponse{
			KeysMigrated: resp.Progress.KeysMigrated,
			TotalKeys:    resp.Progress.TotalKeys,
			Percentage:   resp.Progress.Percentage,
		}
	}

	return httpResp
}

// ListStorageNodesResponse converts a gRPC ListStorageNodesResponse to an HTTP response.
func (c *GRPCToHTTP) ListStorageNodesResponse(resp *pb.ListStorageNodesResponse) *ListStorageNodesHTTPResponse {
	nodes := make([]StorageNodeInfoHTTPResponse, len(resp.Nodes))
	for i, node := range resp.Nodes {
		nodes[i] = StorageNodeInfoHTTPResponse{
			NodeID:           node.NodeId,
			Host:             node.Host,
			Port:             node.Port,
			Status:           node.Status,
			VirtualNodes:     node.VirtualNodes,
			KeysCount:        node.KeysCount,
			DiskUsagePercent: node.DiskUsagePercent,
		}
	}

	return &ListStorageNodesHTTPResponse{
		Status: getStatus(resp.Success),
		Nodes:  nodes,
	}
}

// getStatus returns "success" or "error" based on the boolean value.
func getStatus(success bool) string {
	if success {
		return "success"
	}
	return "error"
}

// convertVectorClock converts a gRPC VectorClock to a map.
func convertVectorClock(vc *pb.VectorClock) map[string]int64 {
	if vc == nil || len(vc.Entries) == 0 {
		return nil
	}

	result := make(map[string]int64)
	for _, entry := range vc.Entries {
		result[entry.CoordinatorNodeId] = entry.LogicalTimestamp
	}
	return result
}

