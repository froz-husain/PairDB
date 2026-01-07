package unit

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/devrev/pairdb/api-gateway/internal/converter"
	pb "github.com/devrev/pairdb/api-gateway/pkg/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTTPToGRPC_WriteKeyValueRequest(t *testing.T) {
	conv := converter.NewHTTPToGRPC()

	tests := []struct {
		name           string
		body           string
		headers        map[string]string
		wantErr        bool
		errContains    string
		checkResult    func(*testing.T, *pb.WriteKeyValueRequest)
	}{
		{
			name: "valid request",
			body: `{"tenant_id": "tenant1", "key": "mykey", "value": "myvalue", "consistency": "quorum"}`,
			headers: map[string]string{
				"Idempotency-Key": "idem-123",
			},
			wantErr: false,
			checkResult: func(t *testing.T, req *pb.WriteKeyValueRequest) {
				assert.Equal(t, "tenant1", req.TenantId)
				assert.Equal(t, "mykey", req.Key)
				assert.Equal(t, []byte("myvalue"), req.Value)
				assert.Equal(t, "quorum", req.Consistency)
				assert.Equal(t, "idem-123", req.IdempotencyKey)
			},
		},
		{
			name: "default consistency",
			body: `{"tenant_id": "tenant1", "key": "mykey", "value": "myvalue"}`,
			wantErr: false,
			checkResult: func(t *testing.T, req *pb.WriteKeyValueRequest) {
				assert.Equal(t, "quorum", req.Consistency)
			},
		},
		{
			name:        "missing key",
			body:        `{"tenant_id": "tenant1", "value": "myvalue"}`,
			wantErr:     true,
			errContains: "key is required",
		},
		{
			name:        "missing tenant_id",
			body:        `{"key": "mykey", "value": "myvalue"}`,
			wantErr:     true,
			errContains: "tenant_id is required",
		},
		{
			name:        "invalid consistency",
			body:        `{"tenant_id": "tenant1", "key": "mykey", "value": "myvalue", "consistency": "invalid"}`,
			wantErr:     true,
			errContains: "invalid consistency level",
		},
		{
			name:        "invalid JSON",
			body:        `{invalid}`,
			wantErr:     true,
			errContains: "failed to parse request body",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/key-value", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			result, err := conv.WriteKeyValueRequest(req)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, result)
				}
			}
		})
	}
}

func TestHTTPToGRPC_ReadKeyValueRequest(t *testing.T) {
	conv := converter.NewHTTPToGRPC()

	tests := []struct {
		name        string
		query       string
		wantErr     bool
		errContains string
		checkResult func(*testing.T, *pb.ReadKeyValueRequest)
	}{
		{
			name:    "valid request",
			query:   "?tenant_id=tenant1&key=mykey&consistency=quorum",
			wantErr: false,
			checkResult: func(t *testing.T, req *pb.ReadKeyValueRequest) {
				assert.Equal(t, "tenant1", req.TenantId)
				assert.Equal(t, "mykey", req.Key)
				assert.Equal(t, "quorum", req.Consistency)
			},
		},
		{
			name:    "default consistency",
			query:   "?tenant_id=tenant1&key=mykey",
			wantErr: false,
			checkResult: func(t *testing.T, req *pb.ReadKeyValueRequest) {
				assert.Equal(t, "quorum", req.Consistency)
			},
		},
		{
			name:        "missing key",
			query:       "?tenant_id=tenant1",
			wantErr:     true,
			errContains: "key query parameter is required",
		},
		{
			name:        "missing tenant_id",
			query:       "?key=mykey",
			wantErr:     true,
			errContains: "tenant_id query parameter is required",
		},
		{
			name:        "invalid consistency",
			query:       "?tenant_id=tenant1&key=mykey&consistency=invalid",
			wantErr:     true,
			errContains: "invalid consistency level",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/v1/key-value"+tt.query, nil)

			result, err := conv.ReadKeyValueRequest(req)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, result)
				}
			}
		})
	}
}

func TestHTTPToGRPC_CreateTenantRequest(t *testing.T) {
	conv := converter.NewHTTPToGRPC()

	tests := []struct {
		name        string
		body        string
		wantErr     bool
		errContains string
		checkResult func(*testing.T, *pb.CreateTenantRequest)
	}{
		{
			name:    "valid request",
			body:    `{"tenant_id": "tenant1", "replication_factor": 3}`,
			wantErr: false,
			checkResult: func(t *testing.T, req *pb.CreateTenantRequest) {
				assert.Equal(t, "tenant1", req.TenantId)
				assert.Equal(t, int32(3), req.ReplicationFactor)
			},
		},
		{
			name:    "default replication factor",
			body:    `{"tenant_id": "tenant1"}`,
			wantErr: false,
			checkResult: func(t *testing.T, req *pb.CreateTenantRequest) {
				assert.Equal(t, int32(3), req.ReplicationFactor)
			},
		},
		{
			name:        "missing tenant_id",
			body:        `{"replication_factor": 3}`,
			wantErr:     true,
			errContains: "tenant_id is required",
		},
		{
			name:        "invalid replication factor",
			body:        `{"tenant_id": "tenant1", "replication_factor": -1}`,
			wantErr:     true,
			errContains: "replication_factor must be at least 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/tenants", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")

			result, err := conv.CreateTenantRequest(req)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, result)
				}
			}
		})
	}
}

func TestHTTPToGRPC_GetTenantRequest(t *testing.T) {
	conv := converter.NewHTTPToGRPC()

	t.Run("valid request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/tenants/tenant1", nil)
		req = mux.SetURLVars(req, map[string]string{"tenant_id": "tenant1"})

		result, err := conv.GetTenantRequest(req)

		require.NoError(t, err)
		assert.Equal(t, "tenant1", result.TenantId)
	})

	t.Run("missing tenant_id", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/tenants/", nil)
		req = mux.SetURLVars(req, map[string]string{})

		_, err := conv.GetTenantRequest(req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "tenant_id path parameter is required")
	})
}

func TestHTTPToGRPC_UpdateReplicationFactorRequest(t *testing.T) {
	conv := converter.NewHTTPToGRPC()

	t.Run("valid request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/v1/tenants/tenant1/replication-factor", bytes.NewBufferString(`{"replication_factor": 5}`))
		req = mux.SetURLVars(req, map[string]string{"tenant_id": "tenant1"})
		req.Header.Set("Content-Type", "application/json")

		result, err := conv.UpdateReplicationFactorRequest(req)

		require.NoError(t, err)
		assert.Equal(t, "tenant1", result.TenantId)
		assert.Equal(t, int32(5), result.NewReplicationFactor)
	})

	t.Run("invalid replication factor", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/v1/tenants/tenant1/replication-factor", bytes.NewBufferString(`{"replication_factor": 0}`))
		req = mux.SetURLVars(req, map[string]string{"tenant_id": "tenant1"})
		req.Header.Set("Content-Type", "application/json")

		_, err := conv.UpdateReplicationFactorRequest(req)

		assert.Error(t, err)
		assert.Contains(t, err.Error(), "replication_factor must be at least 1")
	})
}

func TestHTTPToGRPC_AddStorageNodeRequest(t *testing.T) {
	conv := converter.NewHTTPToGRPC()

	tests := []struct {
		name        string
		body        string
		wantErr     bool
		errContains string
		checkResult func(*testing.T, *pb.AddStorageNodeRequest)
	}{
		{
			name:    "valid request",
			body:    `{"node_id": "node1", "host": "192.168.1.1", "port": 50051, "virtual_nodes": 100}`,
			wantErr: false,
			checkResult: func(t *testing.T, req *pb.AddStorageNodeRequest) {
				assert.Equal(t, "node1", req.NodeId)
				assert.Equal(t, "192.168.1.1", req.Host)
				assert.Equal(t, int32(50051), req.Port)
				assert.Equal(t, int32(100), req.VirtualNodes)
			},
		},
		{
			name:    "default virtual nodes",
			body:    `{"node_id": "node1", "host": "192.168.1.1", "port": 50051}`,
			wantErr: false,
			checkResult: func(t *testing.T, req *pb.AddStorageNodeRequest) {
				assert.Equal(t, int32(150), req.VirtualNodes)
			},
		},
		{
			name:        "missing node_id",
			body:        `{"host": "192.168.1.1", "port": 50051}`,
			wantErr:     true,
			errContains: "node_id is required",
		},
		{
			name:        "missing host",
			body:        `{"node_id": "node1", "port": 50051}`,
			wantErr:     true,
			errContains: "host is required",
		},
		{
			name:        "invalid port",
			body:        `{"node_id": "node1", "host": "192.168.1.1", "port": 70000}`,
			wantErr:     true,
			errContains: "invalid port",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, "/v1/admin/storage-nodes", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")

			result, err := conv.AddStorageNodeRequest(req)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				assert.NoError(t, err)
				if tt.checkResult != nil {
					tt.checkResult(t, result)
				}
			}
		})
	}
}

func TestHTTPToGRPC_RemoveStorageNodeRequest(t *testing.T) {
	conv := converter.NewHTTPToGRPC()

	t.Run("valid request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/v1/admin/storage-nodes/node1?force=true", nil)
		req = mux.SetURLVars(req, map[string]string{"node_id": "node1"})

		result, err := conv.RemoveStorageNodeRequest(req)

		require.NoError(t, err)
		assert.Equal(t, "node1", result.NodeId)
		assert.True(t, result.Force)
	})

	t.Run("default force", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/v1/admin/storage-nodes/node1", nil)
		req = mux.SetURLVars(req, map[string]string{"node_id": "node1"})

		result, err := conv.RemoveStorageNodeRequest(req)

		require.NoError(t, err)
		assert.False(t, result.Force)
	})
}

func TestHTTPToGRPC_GetMigrationStatusRequest(t *testing.T) {
	conv := converter.NewHTTPToGRPC()

	t.Run("valid request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/admin/migrations/mig-123", nil)
		req = mux.SetURLVars(req, map[string]string{"migration_id": "mig-123"})

		result, err := conv.GetMigrationStatusRequest(req)

		require.NoError(t, err)
		assert.Equal(t, "mig-123", result.MigrationId)
	})
}

func TestHTTPToGRPC_ListStorageNodesRequest(t *testing.T) {
	conv := converter.NewHTTPToGRPC()

	t.Run("valid request", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/admin/storage-nodes", nil)

		result, err := conv.ListStorageNodesRequest(req)

		require.NoError(t, err)
		assert.NotNil(t, result)
	})
}

