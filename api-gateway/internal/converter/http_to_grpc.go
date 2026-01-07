// Package converter provides HTTP to gRPC and gRPC to HTTP conversion utilities.
package converter

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
	pb "github.com/devrev/pairdb/api-gateway/pkg/proto"
)

// HTTPToGRPC handles conversion of HTTP requests to gRPC requests.
type HTTPToGRPC struct{}

// NewHTTPToGRPC creates a new HTTPToGRPC converter.
func NewHTTPToGRPC() *HTTPToGRPC {
	return &HTTPToGRPC{}
}

// WriteKeyValueHTTPRequest represents the HTTP request body for WriteKeyValue.
type WriteKeyValueHTTPRequest struct {
	TenantID    string `json:"tenant_id"`
	Key         string `json:"key"`
	Value       string `json:"value"`
	Consistency string `json:"consistency,omitempty"`
}

// ReadKeyValueHTTPRequest represents the HTTP query parameters for ReadKeyValue.
type ReadKeyValueHTTPRequest struct {
	TenantID    string
	Key         string
	Consistency string
}

// CreateTenantHTTPRequest represents the HTTP request body for CreateTenant.
type CreateTenantHTTPRequest struct {
	TenantID          string `json:"tenant_id"`
	ReplicationFactor int32  `json:"replication_factor,omitempty"`
}

// UpdateReplicationFactorHTTPRequest represents the HTTP request body for UpdateReplicationFactor.
type UpdateReplicationFactorHTTPRequest struct {
	ReplicationFactor int32 `json:"replication_factor"`
}

// AddStorageNodeHTTPRequest represents the HTTP request body for AddStorageNode.
type AddStorageNodeHTTPRequest struct {
	NodeID       string `json:"node_id"`
	Host         string `json:"host"`
	Port         int32  `json:"port"`
	VirtualNodes int32  `json:"virtual_nodes,omitempty"`
}

// RemoveStorageNodeHTTPRequest represents the HTTP request body for RemoveStorageNode.
type RemoveStorageNodeHTTPRequest struct {
	Force bool `json:"force,omitempty"`
}

// WriteKeyValueRequest converts an HTTP request to a gRPC WriteKeyValueRequest.
func (c *HTTPToGRPC) WriteKeyValueRequest(r *http.Request) (*pb.WriteKeyValueRequest, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	defer r.Body.Close()

	var httpReq WriteKeyValueHTTPRequest
	if err := json.Unmarshal(body, &httpReq); err != nil {
		return nil, fmt.Errorf("failed to parse request body: %w", err)
	}

	if httpReq.Key == "" {
		return nil, fmt.Errorf("key is required")
	}

	if httpReq.TenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}

	// Get idempotency key from header
	idempotencyKey := r.Header.Get("Idempotency-Key")

	// Default consistency level
	consistency := httpReq.Consistency
	if consistency == "" {
		consistency = "quorum"
	}

	// Validate consistency level
	if !isValidConsistency(consistency) {
		return nil, fmt.Errorf("invalid consistency level: %s (must be one, quorum, or all)", consistency)
	}

	return &pb.WriteKeyValueRequest{
		TenantId:       httpReq.TenantID,
		Key:            httpReq.Key,
		Value:          []byte(httpReq.Value),
		Consistency:    consistency,
		IdempotencyKey: idempotencyKey,
	}, nil
}

// ReadKeyValueRequest converts an HTTP request to a gRPC ReadKeyValueRequest.
func (c *HTTPToGRPC) ReadKeyValueRequest(r *http.Request) (*pb.ReadKeyValueRequest, error) {
	query := r.URL.Query()

	key := query.Get("key")
	if key == "" {
		return nil, fmt.Errorf("key query parameter is required")
	}

	tenantID := query.Get("tenant_id")
	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id query parameter is required")
	}

	// Default consistency level
	consistency := query.Get("consistency")
	if consistency == "" {
		consistency = "quorum"
	}

	// Validate consistency level
	if !isValidConsistency(consistency) {
		return nil, fmt.Errorf("invalid consistency level: %s (must be one, quorum, or all)", consistency)
	}

	return &pb.ReadKeyValueRequest{
		TenantId:    tenantID,
		Key:         key,
		Consistency: consistency,
	}, nil
}

// CreateTenantRequest converts an HTTP request to a gRPC CreateTenantRequest.
func (c *HTTPToGRPC) CreateTenantRequest(r *http.Request) (*pb.CreateTenantRequest, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	defer r.Body.Close()

	var httpReq CreateTenantHTTPRequest
	if err := json.Unmarshal(body, &httpReq); err != nil {
		return nil, fmt.Errorf("failed to parse request body: %w", err)
	}

	if httpReq.TenantID == "" {
		return nil, fmt.Errorf("tenant_id is required")
	}

	// Default replication factor
	replicationFactor := httpReq.ReplicationFactor
	if replicationFactor == 0 {
		replicationFactor = 3
	}

	if replicationFactor < 1 {
		return nil, fmt.Errorf("replication_factor must be at least 1")
	}

	return &pb.CreateTenantRequest{
		TenantId:          httpReq.TenantID,
		ReplicationFactor: replicationFactor,
	}, nil
}

// UpdateReplicationFactorRequest converts an HTTP request to a gRPC UpdateReplicationFactorRequest.
func (c *HTTPToGRPC) UpdateReplicationFactorRequest(r *http.Request) (*pb.UpdateReplicationFactorRequest, error) {
	// Extract tenant_id from path
	vars := mux.Vars(r)
	tenantID := vars["tenant_id"]
	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id path parameter is required")
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	defer r.Body.Close()

	var httpReq UpdateReplicationFactorHTTPRequest
	if err := json.Unmarshal(body, &httpReq); err != nil {
		return nil, fmt.Errorf("failed to parse request body: %w", err)
	}

	if httpReq.ReplicationFactor < 1 {
		return nil, fmt.Errorf("replication_factor must be at least 1")
	}

	return &pb.UpdateReplicationFactorRequest{
		TenantId:             tenantID,
		NewReplicationFactor: httpReq.ReplicationFactor,
	}, nil
}

// GetTenantRequest converts an HTTP request to a gRPC GetTenantRequest.
func (c *HTTPToGRPC) GetTenantRequest(r *http.Request) (*pb.GetTenantRequest, error) {
	// Extract tenant_id from path
	vars := mux.Vars(r)
	tenantID := vars["tenant_id"]
	if tenantID == "" {
		return nil, fmt.Errorf("tenant_id path parameter is required")
	}

	return &pb.GetTenantRequest{
		TenantId: tenantID,
	}, nil
}

// AddStorageNodeRequest converts an HTTP request to a gRPC AddStorageNodeRequest.
func (c *HTTPToGRPC) AddStorageNodeRequest(r *http.Request) (*pb.AddStorageNodeRequest, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}
	defer r.Body.Close()

	var httpReq AddStorageNodeHTTPRequest
	if err := json.Unmarshal(body, &httpReq); err != nil {
		return nil, fmt.Errorf("failed to parse request body: %w", err)
	}

	if httpReq.NodeID == "" {
		return nil, fmt.Errorf("node_id is required")
	}

	if httpReq.Host == "" {
		return nil, fmt.Errorf("host is required")
	}

	if httpReq.Port <= 0 || httpReq.Port > 65535 {
		return nil, fmt.Errorf("invalid port: %d", httpReq.Port)
	}

	// Default virtual nodes
	virtualNodes := httpReq.VirtualNodes
	if virtualNodes == 0 {
		virtualNodes = 150
	}

	return &pb.AddStorageNodeRequest{
		NodeId:       httpReq.NodeID,
		Host:         httpReq.Host,
		Port:         httpReq.Port,
		VirtualNodes: virtualNodes,
	}, nil
}

// RemoveStorageNodeRequest converts an HTTP request to a gRPC RemoveStorageNodeRequest.
func (c *HTTPToGRPC) RemoveStorageNodeRequest(r *http.Request) (*pb.RemoveStorageNodeRequest, error) {
	// Extract node_id from path
	vars := mux.Vars(r)
	nodeID := vars["node_id"]
	if nodeID == "" {
		return nil, fmt.Errorf("node_id path parameter is required")
	}

	// Check for force parameter in query string
	force := false
	if forceStr := r.URL.Query().Get("force"); forceStr != "" {
		var err error
		force, err = strconv.ParseBool(forceStr)
		if err != nil {
			return nil, fmt.Errorf("invalid force parameter: %w", err)
		}
	}

	return &pb.RemoveStorageNodeRequest{
		NodeId: nodeID,
		Force:  force,
	}, nil
}

// GetMigrationStatusRequest converts an HTTP request to a gRPC GetMigrationStatusRequest.
func (c *HTTPToGRPC) GetMigrationStatusRequest(r *http.Request) (*pb.GetMigrationStatusRequest, error) {
	// Extract migration_id from path
	vars := mux.Vars(r)
	migrationID := vars["migration_id"]
	if migrationID == "" {
		return nil, fmt.Errorf("migration_id path parameter is required")
	}

	return &pb.GetMigrationStatusRequest{
		MigrationId: migrationID,
	}, nil
}

// ListStorageNodesRequest converts an HTTP request to a gRPC ListStorageNodesRequest.
func (c *HTTPToGRPC) ListStorageNodesRequest(r *http.Request) (*pb.ListStorageNodesRequest, error) {
	return &pb.ListStorageNodesRequest{}, nil
}

// isValidConsistency checks if the consistency level is valid.
func isValidConsistency(consistency string) bool {
	switch consistency {
	case "one", "quorum", "all":
		return true
	default:
		return false
	}
}

