package unit

import (
	"testing"

	"github.com/devrev/pairdb/api-gateway/internal/converter"
	pb "github.com/devrev/pairdb/api-gateway/pkg/proto"
	"github.com/stretchr/testify/assert"
)

func TestGRPCToHTTP_WriteKeyValueResponse(t *testing.T) {
	conv := converter.NewGRPCToHTTP()

	t.Run("successful response", func(t *testing.T) {
		grpcResp := &pb.WriteKeyValueResponse{
			Success:        true,
			Key:            "mykey",
			IdempotencyKey: "idem-123",
			VectorClock: &pb.VectorClock{
				Entries: []*pb.VectorClockEntry{
					{CoordinatorNodeId: "node1", LogicalTimestamp: 1},
					{CoordinatorNodeId: "node2", LogicalTimestamp: 2},
				},
			},
			ReplicaCount: 3,
			Consistency:  "quorum",
			IsDuplicate:  false,
		}

		httpResp := conv.WriteKeyValueResponse(grpcResp)

		assert.Equal(t, "success", httpResp.Status)
		assert.Equal(t, "mykey", httpResp.Key)
		assert.Equal(t, "idem-123", httpResp.IdempotencyKey)
		assert.Equal(t, int32(3), httpResp.ReplicaCount)
		assert.Equal(t, "quorum", httpResp.Consistency)
		assert.False(t, httpResp.IsDuplicate)
		assert.Equal(t, int64(1), httpResp.VectorClock["node1"])
		assert.Equal(t, int64(2), httpResp.VectorClock["node2"])
	})

	t.Run("duplicate response", func(t *testing.T) {
		grpcResp := &pb.WriteKeyValueResponse{
			Success:     true,
			Key:         "mykey",
			IsDuplicate: true,
		}

		httpResp := conv.WriteKeyValueResponse(grpcResp)

		assert.True(t, httpResp.IsDuplicate)
	})

	t.Run("error response", func(t *testing.T) {
		grpcResp := &pb.WriteKeyValueResponse{
			Success: false,
		}

		httpResp := conv.WriteKeyValueResponse(grpcResp)

		assert.Equal(t, "error", httpResp.Status)
	})

	t.Run("nil vector clock", func(t *testing.T) {
		grpcResp := &pb.WriteKeyValueResponse{
			Success:     true,
			VectorClock: nil,
		}

		httpResp := conv.WriteKeyValueResponse(grpcResp)

		assert.Nil(t, httpResp.VectorClock)
	})
}

func TestGRPCToHTTP_ReadKeyValueResponse(t *testing.T) {
	conv := converter.NewGRPCToHTTP()

	t.Run("successful response", func(t *testing.T) {
		grpcResp := &pb.ReadKeyValueResponse{
			Success: true,
			Key:     "mykey",
			Value:   []byte("myvalue"),
			VectorClock: &pb.VectorClock{
				Entries: []*pb.VectorClockEntry{
					{CoordinatorNodeId: "node1", LogicalTimestamp: 5},
				},
			},
		}

		httpResp := conv.ReadKeyValueResponse(grpcResp)

		assert.Equal(t, "success", httpResp.Status)
		assert.Equal(t, "mykey", httpResp.Key)
		assert.Equal(t, "myvalue", httpResp.Value)
		assert.Equal(t, int64(5), httpResp.VectorClock["node1"])
	})
}

func TestGRPCToHTTP_CreateTenantResponse(t *testing.T) {
	conv := converter.NewGRPCToHTTP()

	t.Run("successful response", func(t *testing.T) {
		grpcResp := &pb.CreateTenantResponse{
			Success:           true,
			TenantId:          "tenant1",
			ReplicationFactor: 3,
			CreatedAt:         1609459200,
		}

		httpResp := conv.CreateTenantResponse(grpcResp)

		assert.Equal(t, "success", httpResp.Status)
		assert.Equal(t, "tenant1", httpResp.TenantID)
		assert.Equal(t, int32(3), httpResp.ReplicationFactor)
		assert.Equal(t, int64(1609459200), httpResp.CreatedAt)
	})
}

func TestGRPCToHTTP_UpdateReplicationFactorResponse(t *testing.T) {
	conv := converter.NewGRPCToHTTP()

	t.Run("successful response with migration", func(t *testing.T) {
		grpcResp := &pb.UpdateReplicationFactorResponse{
			Success:              true,
			TenantId:             "tenant1",
			OldReplicationFactor: 3,
			NewReplicationFactor: 5,
			MigrationId:          "mig-123",
			UpdatedAt:            1609459200,
		}

		httpResp := conv.UpdateReplicationFactorResponse(grpcResp)

		assert.Equal(t, "success", httpResp.Status)
		assert.Equal(t, "tenant1", httpResp.TenantID)
		assert.Equal(t, int32(3), httpResp.OldReplicationFactor)
		assert.Equal(t, int32(5), httpResp.NewReplicationFactor)
		assert.Equal(t, "mig-123", httpResp.MigrationID)
		assert.Equal(t, int64(1609459200), httpResp.UpdatedAt)
	})
}

func TestGRPCToHTTP_GetTenantResponse(t *testing.T) {
	conv := converter.NewGRPCToHTTP()

	t.Run("successful response", func(t *testing.T) {
		grpcResp := &pb.GetTenantResponse{
			Success:           true,
			TenantId:          "tenant1",
			ReplicationFactor: 3,
			CreatedAt:         1609459200,
			UpdatedAt:         1609459300,
		}

		httpResp := conv.GetTenantResponse(grpcResp)

		assert.Equal(t, "success", httpResp.Status)
		assert.Equal(t, "tenant1", httpResp.TenantID)
		assert.Equal(t, int32(3), httpResp.ReplicationFactor)
		assert.Equal(t, int64(1609459200), httpResp.CreatedAt)
		assert.Equal(t, int64(1609459300), httpResp.UpdatedAt)
	})
}

func TestGRPCToHTTP_AddStorageNodeResponse(t *testing.T) {
	conv := converter.NewGRPCToHTTP()

	t.Run("successful response", func(t *testing.T) {
		grpcResp := &pb.AddStorageNodeResponse{
			Success:             true,
			NodeId:              "node1",
			MigrationId:         "mig-123",
			Message:             "Node added successfully",
			EstimatedCompletion: 1609459200,
		}

		httpResp := conv.AddStorageNodeResponse(grpcResp)

		assert.Equal(t, "success", httpResp.Status)
		assert.Equal(t, "node1", httpResp.NodeID)
		assert.Equal(t, "mig-123", httpResp.MigrationID)
		assert.Equal(t, "Node added successfully", httpResp.Message)
		assert.Equal(t, int64(1609459200), httpResp.EstimatedCompletion)
	})
}

func TestGRPCToHTTP_RemoveStorageNodeResponse(t *testing.T) {
	conv := converter.NewGRPCToHTTP()

	t.Run("successful response", func(t *testing.T) {
		grpcResp := &pb.RemoveStorageNodeResponse{
			Success:             true,
			NodeId:              "node1",
			MigrationId:         "mig-456",
			Message:             "Node removal initiated",
			EstimatedCompletion: 1609459200,
		}

		httpResp := conv.RemoveStorageNodeResponse(grpcResp)

		assert.Equal(t, "success", httpResp.Status)
		assert.Equal(t, "node1", httpResp.NodeID)
		assert.Equal(t, "mig-456", httpResp.MigrationID)
		assert.Equal(t, "Node removal initiated", httpResp.Message)
	})
}

func TestGRPCToHTTP_GetMigrationStatusResponse(t *testing.T) {
	conv := converter.NewGRPCToHTTP()

	t.Run("successful response with progress", func(t *testing.T) {
		grpcResp := &pb.GetMigrationStatusResponse{
			Success:     true,
			MigrationId: "mig-123",
			Type:        "node_addition",
			NodeId:      "node1",
			Status:      "in_progress",
			Progress: &pb.MigrationProgress{
				KeysMigrated: 5000,
				TotalKeys:    10000,
				Percentage:   50.0,
			},
			StartedAt:           1609459200,
			EstimatedCompletion: 1609459500,
		}

		httpResp := conv.GetMigrationStatusResponse(grpcResp)

		assert.Equal(t, "success", httpResp.Status)
		assert.Equal(t, "mig-123", httpResp.MigrationID)
		assert.Equal(t, "node_addition", httpResp.Type)
		assert.Equal(t, "node1", httpResp.NodeID)
		assert.Equal(t, "in_progress", httpResp.MigrationStatus)
		assert.NotNil(t, httpResp.Progress)
		assert.Equal(t, int64(5000), httpResp.Progress.KeysMigrated)
		assert.Equal(t, int64(10000), httpResp.Progress.TotalKeys)
		assert.Equal(t, float32(50.0), httpResp.Progress.Percentage)
	})

	t.Run("response without progress", func(t *testing.T) {
		grpcResp := &pb.GetMigrationStatusResponse{
			Success:  true,
			Status:   "completed",
			Progress: nil,
		}

		httpResp := conv.GetMigrationStatusResponse(grpcResp)

		assert.Nil(t, httpResp.Progress)
	})
}

func TestGRPCToHTTP_ListStorageNodesResponse(t *testing.T) {
	conv := converter.NewGRPCToHTTP()

	t.Run("successful response with nodes", func(t *testing.T) {
		grpcResp := &pb.ListStorageNodesResponse{
			Success: true,
			Nodes: []*pb.StorageNodeInfo{
				{
					NodeId:           "node1",
					Host:             "192.168.1.1",
					Port:             50051,
					Status:           "active",
					VirtualNodes:     150,
					KeysCount:        10000,
					DiskUsagePercent: 45.5,
				},
				{
					NodeId:           "node2",
					Host:             "192.168.1.2",
					Port:             50051,
					Status:           "draining",
					VirtualNodes:     150,
					KeysCount:        8000,
					DiskUsagePercent: 38.2,
				},
			},
		}

		httpResp := conv.ListStorageNodesResponse(grpcResp)

		assert.Equal(t, "success", httpResp.Status)
		assert.Len(t, httpResp.Nodes, 2)
		assert.Equal(t, "node1", httpResp.Nodes[0].NodeID)
		assert.Equal(t, "192.168.1.1", httpResp.Nodes[0].Host)
		assert.Equal(t, int32(50051), httpResp.Nodes[0].Port)
		assert.Equal(t, "active", httpResp.Nodes[0].Status)
		assert.Equal(t, "node2", httpResp.Nodes[1].NodeID)
		assert.Equal(t, "draining", httpResp.Nodes[1].Status)
	})

	t.Run("empty nodes list", func(t *testing.T) {
		grpcResp := &pb.ListStorageNodesResponse{
			Success: true,
			Nodes:   []*pb.StorageNodeInfo{},
		}

		httpResp := conv.ListStorageNodesResponse(grpcResp)

		assert.Equal(t, "success", httpResp.Status)
		assert.Empty(t, httpResp.Nodes)
	})
}

