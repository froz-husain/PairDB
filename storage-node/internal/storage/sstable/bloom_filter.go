package sstable

import (
	"encoding/binary"
	"hash/fnv"
	"io"
	"math"
	"os"
)

// BloomFilter is a probabilistic data structure for set membership
type BloomFilter struct {
	bits      []bool
	size      uint64
	hashCount uint64
}

// NewBloomFilter creates a new bloom filter with expected elements and false positive rate
func NewBloomFilter(expectedElements int, falsePositiveRate float64) *BloomFilter {
	// Calculate optimal size: m = -(n * ln(p)) / (ln(2)^2)
	size := uint64(-float64(expectedElements) * math.Log(falsePositiveRate) / (math.Ln2 * math.Ln2))

	// Calculate optimal hash count: k = (m/n) * ln(2)
	hashCount := uint64(float64(size) / float64(expectedElements) * math.Ln2)

	// Ensure at least 1 hash function
	if hashCount == 0 {
		hashCount = 1
	}

	return &BloomFilter{
		bits:      make([]bool, size),
		size:      size,
		hashCount: hashCount,
	}
}

// Add inserts a key into the bloom filter
func (bf *BloomFilter) Add(key string) {
	hashes := bf.getHashes(key)
	for _, hash := range hashes {
		bf.bits[hash%bf.size] = true
	}
}

// MayContain checks if a key might be in the set
func (bf *BloomFilter) MayContain(key string) bool {
	hashes := bf.getHashes(key)
	for _, hash := range hashes {
		if !bf.bits[hash%bf.size] {
			return false
		}
	}
	return true
}

// getHashes generates k hash values for a key
func (bf *BloomFilter) getHashes(key string) []uint64 {
	hashes := make([]uint64, bf.hashCount)

	// Use double hashing: h(i) = h1(x) + i*h2(x)
	h := fnv.New64()
	h.Write([]byte(key))
	hash1 := h.Sum64()

	h.Reset()
	h.Write([]byte(key + "salt"))
	hash2 := h.Sum64()

	for i := uint64(0); i < bf.hashCount; i++ {
		hashes[i] = hash1 + i*hash2
	}

	return hashes
}

// WriteTo serializes and writes the bloom filter to a file
func (bf *BloomFilter) WriteTo(file *os.File) error {
	// Write size
	if err := binary.Write(file, binary.LittleEndian, bf.size); err != nil {
		return err
	}

	// Write hash count
	if err := binary.Write(file, binary.LittleEndian, bf.hashCount); err != nil {
		return err
	}

	// Write bit array (pack into bytes)
	byteCount := (bf.size + 7) / 8
	bytes := make([]byte, byteCount)

	for i := uint64(0); i < bf.size; i++ {
		if bf.bits[i] {
			byteIndex := i / 8
			bitIndex := i % 8
			bytes[byteIndex] |= 1 << bitIndex
		}
	}

	_, err := file.Write(bytes)
	return err
}

// LoadBloomFilter loads a bloom filter from a file
func LoadBloomFilter(filePath string) (*BloomFilter, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	bf := &BloomFilter{}

	// Read size
	if err := binary.Read(file, binary.LittleEndian, &bf.size); err != nil {
		return nil, err
	}

	// Read hash count
	if err := binary.Read(file, binary.LittleEndian, &bf.hashCount); err != nil {
		return nil, err
	}

	// Read bit array
	byteCount := (bf.size + 7) / 8
	bytes := make([]byte, byteCount)

	if _, err := io.ReadFull(file, bytes); err != nil {
		return nil, err
	}

	// Unpack bytes into bits
	bf.bits = make([]bool, bf.size)
	for i := uint64(0); i < bf.size; i++ {
		byteIndex := i / 8
		bitIndex := i % 8
		bf.bits[i] = (bytes[byteIndex] & (1 << bitIndex)) != 0
	}

	return bf, nil
}
