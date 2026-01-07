package storage_test

import (
	"testing"

	"github.com/devrev/pairdb/storage-node/internal/storage/memtable"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSkipList_Insert(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		value  string
		verify func(*testing.T, *memtable.SkipList)
	}{
		{
			name:  "insert single element",
			key:   "key1",
			value: "value1",
			verify: func(t *testing.T, sl *memtable.SkipList) {
				val, found := sl.Search("key1")
				assert.True(t, found)
				assert.Equal(t, "value1", val)
			},
		},
		{
			name:  "insert multiple elements",
			key:   "key2",
			value: "value2",
			verify: func(t *testing.T, sl *memtable.SkipList) {
				sl.Insert("key3", "value3")
				sl.Insert("key1", "value1")

				assert.Equal(t, 3, sl.Len())
				val, found := sl.Search("key1")
				assert.True(t, found)
				assert.Equal(t, "value1", val)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sl := memtable.NewSkipList()
			sl.Insert(tt.key, tt.value)
			tt.verify(t, sl)
		})
	}
}

func TestSkipList_Update(t *testing.T) {
	sl := memtable.NewSkipList()

	// Insert initial value
	sl.Insert("key1", "value1")
	val, found := sl.Search("key1")
	require.True(t, found)
	assert.Equal(t, "value1", val)

	// Update value
	sl.Insert("key1", "value2")
	val, found = sl.Search("key1")
	require.True(t, found)
	assert.Equal(t, "value2", val)

	// Size should remain 1
	assert.Equal(t, 1, sl.Len())
}

func TestSkipList_Search(t *testing.T) {
	sl := memtable.NewSkipList()

	// Insert test data
	sl.Insert("apple", "fruit1")
	sl.Insert("banana", "fruit2")
	sl.Insert("cherry", "fruit3")

	tests := []struct {
		name       string
		key        string
		wantValue  interface{}
		wantFound  bool
	}{
		{
			name:      "search existing key",
			key:       "banana",
			wantValue: "fruit2",
			wantFound: true,
		},
		{
			name:      "search non-existing key",
			key:       "mango",
			wantValue: nil,
			wantFound: false,
		},
		{
			name:      "search first key",
			key:       "apple",
			wantValue: "fruit1",
			wantFound: true,
		},
		{
			name:      "search last key",
			key:       "cherry",
			wantValue: "fruit3",
			wantFound: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, found := sl.Search(tt.key)
			assert.Equal(t, tt.wantFound, found)
			assert.Equal(t, tt.wantValue, val)
		})
	}
}

func TestSkipList_Delete(t *testing.T) {
	sl := memtable.NewSkipList()

	// Insert test data
	sl.Insert("key1", "value1")
	sl.Insert("key2", "value2")
	sl.Insert("key3", "value3")

	tests := []struct {
		name    string
		key     string
		wantOk  bool
		wantLen int
	}{
		{
			name:    "delete existing key",
			key:     "key2",
			wantOk:  true,
			wantLen: 2,
		},
		{
			name:    "delete non-existing key",
			key:     "key4",
			wantOk:  false,
			wantLen: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ok := sl.Delete(tt.key)
			assert.Equal(t, tt.wantOk, ok)
			assert.Equal(t, tt.wantLen, sl.Len())

			if ok {
				_, found := sl.Search(tt.key)
				assert.False(t, found)
			}
		})
	}
}

func TestSkipList_Iterator(t *testing.T) {
	sl := memtable.NewSkipList()

	// Insert test data in unsorted order
	sl.Insert("cherry", "fruit3")
	sl.Insert("apple", "fruit1")
	sl.Insert("banana", "fruit2")

	// Iterate and verify sorted order
	iter := sl.Iterator()
	keys := []string{}
	for iter.Next() {
		keys = append(keys, iter.Key())
	}

	expected := []string{"apple", "banana", "cherry"}
	assert.Equal(t, expected, keys)
}

func TestSkipList_Concurrent(t *testing.T) {
	t.Skip("Skipping concurrent test - requires mutex in SkipList")
	// This test would verify concurrent access safety
	// In production, the SkipList should be protected by MemTable's mutex
}

func TestSkipList_Empty(t *testing.T) {
	sl := memtable.NewSkipList()

	// Search in empty list
	_, found := sl.Search("key1")
	assert.False(t, found)

	// Delete from empty list
	ok := sl.Delete("key1")
	assert.False(t, ok)

	// Iterate empty list
	iter := sl.Iterator()
	assert.False(t, iter.Next())

	// Check size
	assert.Equal(t, 0, sl.Len())
}

func BenchmarkSkipList_Insert(b *testing.B) {
	sl := memtable.NewSkipList()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "key" + string(rune(i))
		sl.Insert(key, "value")
	}
}

func BenchmarkSkipList_Search(b *testing.B) {
	sl := memtable.NewSkipList()

	// Populate skip list
	for i := 0; i < 10000; i++ {
		key := "key" + string(rune(i))
		sl.Insert(key, "value")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := "key" + string(rune(i%10000))
		sl.Search(key)
	}
}
