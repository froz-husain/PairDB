package sstable

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/devrev/pairdb/storage-node/internal/model"
	"github.com/devrev/pairdb/storage-node/internal/util"
)

// SSTableReader reads data from an SSTable
type SSTableReader struct {
	dataFile  *os.File
	indexFile *os.File
	index     map[string]IndexEntry
}

// NewSSTableReader creates a new SSTable reader
func NewSSTableReader(dataPath, indexPath string) (*SSTableReader, error) {
	dataFile, err := os.Open(dataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open data file: %w", err)
	}

	indexFile, err := os.Open(indexPath)
	if err != nil {
		dataFile.Close()
		return nil, fmt.Errorf("failed to open index file: %w", err)
	}

	reader := &SSTableReader{
		dataFile:  dataFile,
		indexFile: indexFile,
		index:     make(map[string]IndexEntry),
	}

	// Load index into memory
	if err := reader.loadIndex(); err != nil {
		reader.Close()
		return nil, fmt.Errorf("failed to load index: %w", err)
	}

	return reader, nil
}

// loadIndex loads the index file into memory with checksum support
func (r *SSTableReader) loadIndex() error {
	for {
		// Read key length
		var keyLen int32
		if err := binary.Read(r.indexFile, binary.LittleEndian, &keyLen); err != nil {
			if err == io.EOF {
				break
			}
			return err
		}

		// Read key
		keyBytes := make([]byte, keyLen)
		if _, err := io.ReadFull(r.indexFile, keyBytes); err != nil {
			return err
		}
		key := string(keyBytes)

		// Read offset
		var offset int64
		if err := binary.Read(r.indexFile, binary.LittleEndian, &offset); err != nil {
			return err
		}

		// Read size
		var size int32
		if err := binary.Read(r.indexFile, binary.LittleEndian, &size); err != nil {
			return err
		}

		// Read checksum (if present, backward compatible)
		var checksum uint32
		if err := binary.Read(r.indexFile, binary.LittleEndian, &checksum); err != nil {
			// Backward compatibility: older files may not have checksums
			if err != io.EOF {
				return err
			}
			checksum = 0
		}

		r.index[key] = IndexEntry{
			Key:      key,
			Offset:   offset,
			Size:     size,
			Checksum: checksum,
		}
	}

	return nil
}

// Get retrieves a value by key with checksum validation
func (r *SSTableReader) Get(key string) (*model.KeyValueEntry, error) {
	// Check index
	indexEntry, found := r.index[key]
	if !found {
		return nil, nil
	}

	// Seek to offset
	if _, err := r.dataFile.Seek(indexEntry.Offset, 0); err != nil {
		return nil, fmt.Errorf("failed to seek to offset: %w", err)
	}

	// Read entry size
	var entrySize int32
	if err := binary.Read(r.dataFile, binary.LittleEndian, &entrySize); err != nil {
		return nil, fmt.Errorf("failed to read entry size: %w", err)
	}

	// Read checksum if present (backward compatible)
	var checksum uint32
	if indexEntry.Checksum != 0 {
		if err := binary.Read(r.dataFile, binary.LittleEndian, &checksum); err != nil {
			return nil, fmt.Errorf("failed to read checksum: %w", err)
		}
	}

	// Read entry data
	data := make([]byte, entrySize)
	if _, err := io.ReadFull(r.dataFile, data); err != nil {
		return nil, fmt.Errorf("failed to read entry data: %w", err)
	}

	// Validate checksum if present
	if indexEntry.Checksum != 0 {
		if !util.ValidateChecksum(data, checksum) {
			return nil, fmt.Errorf("checksum validation failed for key %s: expected %d, got %d",
				key, checksum, util.ComputeChecksum(data))
		}
	}

	// Deserialize entry
	var memEntry model.MemTableEntry
	if err := json.Unmarshal(data, &memEntry); err != nil {
		return nil, fmt.Errorf("failed to unmarshal entry: %w", err)
	}

	// Convert to KeyValueEntry
	// Extract tenant_id from composite key (format: "tenant_id:key")
	tenantID, actualKey := splitCompositeKey(memEntry.Key)

	return &model.KeyValueEntry{
		TenantID:    tenantID,
		Key:         actualKey,
		Value:       memEntry.Value,
		VectorClock: memEntry.VectorClock,
		Timestamp:   memEntry.Timestamp,
		IsTombstone: memEntry.IsTombstone,
	}, nil
}

// HasKey checks if a key exists in the SSTable
func (r *SSTableReader) HasKey(key string) bool {
	_, found := r.index[key]
	return found
}

// GetAllKeys returns all keys in the SSTable
func (r *SSTableReader) GetAllKeys() []string {
	keys := make([]string, 0, len(r.index))
	for key := range r.index {
		keys = append(keys, key)
	}
	return keys
}

// Close closes the reader
func (r *SSTableReader) Close() error {
	var err error
	if e := r.dataFile.Close(); e != nil {
		err = e
	}
	if e := r.indexFile.Close(); e != nil {
		err = e
	}
	return err
}

// splitCompositeKey splits a composite key into tenant_id and key
func splitCompositeKey(compositeKey string) (string, string) {
	for i := 0; i < len(compositeKey); i++ {
		if compositeKey[i] == ':' {
			return compositeKey[:i], compositeKey[i+1:]
		}
	}
	return "", compositeKey
}
