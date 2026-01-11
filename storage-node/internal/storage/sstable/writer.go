package sstable

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"os"

	"github.com/devrev/pairdb/storage-node/internal/model"
	"github.com/devrev/pairdb/storage-node/internal/util"
)

// IndexEntry represents an entry in the SSTable index
type IndexEntry struct {
	Key      string
	Offset   int64
	Size     int32
	Checksum uint32 // CRC32 checksum of the data block
}

// SSTableWriter writes data to an SSTable file
type SSTableWriter struct {
	dataFile    *os.File
	indexFile   *os.File
	bloomFile   *os.File
	offset      int64
	index       []IndexEntry
	bloomFilter *BloomFilter
	config      *SSTableConfig
}

// SSTableConfig holds SSTable configuration
type SSTableConfig struct {
	BloomFilterFP float64
	BlockSize     int
	IndexInterval int
}

// NewSSTableWriter creates a new SSTable writer
func NewSSTableWriter(filePath string, config *SSTableConfig) (*SSTableWriter, error) {
	dataFile, err := os.Create(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to create data file: %w", err)
	}

	indexFile, err := os.Create(filePath + ".idx")
	if err != nil {
		dataFile.Close()
		return nil, fmt.Errorf("failed to create index file: %w", err)
	}

	bloomFile, err := os.Create(filePath + ".bloom")
	if err != nil {
		dataFile.Close()
		indexFile.Close()
		return nil, fmt.Errorf("failed to create bloom file: %w", err)
	}

	return &SSTableWriter{
		dataFile:    dataFile,
		indexFile:   indexFile,
		bloomFile:   bloomFile,
		index:       make([]IndexEntry, 0),
		bloomFilter: NewBloomFilter(10000, config.BloomFilterFP),
		config:      config,
	}, nil
}

// Write writes a memtable entry to the SSTable with checksum
func (w *SSTableWriter) Write(entry *model.MemTableEntry) error {
	// Serialize entry to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("failed to marshal entry: %w", err)
	}

	// Compute checksum of the data
	checksum := util.ComputeChecksum(data)

	// Write entry size
	entrySize := int32(len(data))
	if err := binary.Write(w.dataFile, binary.LittleEndian, entrySize); err != nil {
		return fmt.Errorf("failed to write entry size: %w", err)
	}

	// Write checksum
	if err := binary.Write(w.dataFile, binary.LittleEndian, checksum); err != nil {
		return fmt.Errorf("failed to write checksum: %w", err)
	}

	// Write entry data
	n, err := w.dataFile.Write(data)
	if err != nil {
		return fmt.Errorf("failed to write entry data: %w", err)
	}

	// Add to index
	w.index = append(w.index, IndexEntry{
		Key:      entry.Key,
		Offset:   w.offset,
		Size:     entrySize,
		Checksum: checksum,
	})

	// Add to bloom filter
	w.bloomFilter.Add(entry.Key)

	// Update offset (size field + checksum + data)
	w.offset += int64(4 + 4 + n)

	return nil
}

// Finalize completes the SSTable writing process
func (w *SSTableWriter) Finalize() error {
	// Write index to index file
	for _, entry := range w.index {
		if err := w.writeIndexEntry(entry); err != nil {
			return fmt.Errorf("failed to write index entry: %w", err)
		}
	}

	// Write bloom filter to bloom file
	if err := w.bloomFilter.WriteTo(w.bloomFile); err != nil {
		return fmt.Errorf("failed to write bloom filter: %w", err)
	}

	// Sync all files
	if err := w.dataFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync data file: %w", err)
	}
	if err := w.indexFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync index file: %w", err)
	}
	if err := w.bloomFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync bloom file: %w", err)
	}

	return nil
}

// writeIndexEntry writes a single index entry with checksum
func (w *SSTableWriter) writeIndexEntry(entry IndexEntry) error {
	// Write key length
	keyLen := int32(len(entry.Key))
	if err := binary.Write(w.indexFile, binary.LittleEndian, keyLen); err != nil {
		return err
	}

	// Write key
	if _, err := w.indexFile.Write([]byte(entry.Key)); err != nil {
		return err
	}

	// Write offset
	if err := binary.Write(w.indexFile, binary.LittleEndian, entry.Offset); err != nil {
		return err
	}

	// Write size
	if err := binary.Write(w.indexFile, binary.LittleEndian, entry.Size); err != nil {
		return err
	}

	// Write checksum
	if err := binary.Write(w.indexFile, binary.LittleEndian, entry.Checksum); err != nil {
		return err
	}

	return nil
}

// Size returns the current size of the SSTable
func (w *SSTableWriter) Size() int64 {
	return w.offset
}

// Close closes all files
func (w *SSTableWriter) Close() error {
	var err error
	if e := w.dataFile.Close(); e != nil {
		err = e
	}
	if e := w.indexFile.Close(); e != nil {
		err = e
	}
	if e := w.bloomFile.Close(); e != nil {
		err = e
	}
	return err
}
