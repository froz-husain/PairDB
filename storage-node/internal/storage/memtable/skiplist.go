package memtable

import (
	"math/rand"
)

const (
	MaxLevel    = 16
	Probability = 0.5
)

// SkipListNode represents a node in the skip list
type SkipListNode struct {
	Key     string
	Value   interface{}
	Forward []*SkipListNode
}

// SkipList is a probabilistic data structure for fast key-value operations
type SkipList struct {
	Head  *SkipListNode
	Level int
	Size  int
}

// NewSkipList creates a new skip list
func NewSkipList() *SkipList {
	head := &SkipListNode{
		Forward: make([]*SkipListNode, MaxLevel),
	}
	return &SkipList{
		Head:  head,
		Level: 0,
	}
}

// randomLevel generates a random level for a new node
func (sl *SkipList) randomLevel() int {
	level := 0
	for rand.Float64() < Probability && level < MaxLevel-1 {
		level++
	}
	return level
}

// Insert adds or updates a key-value pair
func (sl *SkipList) Insert(key string, value interface{}) {
	update := make([]*SkipListNode, MaxLevel)
	current := sl.Head

	// Find position to insert
	for i := sl.Level; i >= 0; i-- {
		for current.Forward[i] != nil && current.Forward[i].Key < key {
			current = current.Forward[i]
		}
		update[i] = current
	}

	// Check if key already exists
	current = current.Forward[0]
	if current != nil && current.Key == key {
		current.Value = value
		return
	}

	// Insert new node
	newLevel := sl.randomLevel()
	if newLevel > sl.Level {
		for i := sl.Level + 1; i <= newLevel; i++ {
			update[i] = sl.Head
		}
		sl.Level = newLevel
	}

	newNode := &SkipListNode{
		Key:     key,
		Value:   value,
		Forward: make([]*SkipListNode, newLevel+1),
	}

	for i := 0; i <= newLevel; i++ {
		newNode.Forward[i] = update[i].Forward[i]
		update[i].Forward[i] = newNode
	}

	sl.Size++
}

// Search finds a value by key
func (sl *SkipList) Search(key string) (interface{}, bool) {
	current := sl.Head

	for i := sl.Level; i >= 0; i-- {
		for current.Forward[i] != nil && current.Forward[i].Key < key {
			current = current.Forward[i]
		}
	}

	current = current.Forward[0]
	if current != nil && current.Key == key {
		return current.Value, true
	}

	return nil, false
}

// Delete removes a key from the skip list
func (sl *SkipList) Delete(key string) bool {
	update := make([]*SkipListNode, MaxLevel)
	current := sl.Head

	// Find node to delete
	for i := sl.Level; i >= 0; i-- {
		for current.Forward[i] != nil && current.Forward[i].Key < key {
			current = current.Forward[i]
		}
		update[i] = current
	}

	current = current.Forward[0]
	if current == nil || current.Key != key {
		return false
	}

	// Remove node
	for i := 0; i <= sl.Level; i++ {
		if update[i].Forward[i] != current {
			break
		}
		update[i].Forward[i] = current.Forward[i]
	}

	// Update level
	for sl.Level > 0 && sl.Head.Forward[sl.Level] == nil {
		sl.Level--
	}

	sl.Size--
	return true
}

// Len returns the number of elements in the skip list
func (sl *SkipList) Len() int {
	return sl.Size
}

// Iterator returns a new skip list iterator
func (sl *SkipList) Iterator() *SkipListIterator {
	return &SkipListIterator{
		current: sl.Head,
	}
}

// SkipListIterator iterates over skip list entries
type SkipListIterator struct {
	current *SkipListNode
}

// Next moves to the next element
func (it *SkipListIterator) Next() bool {
	if it.current == nil {
		return false
	}
	it.current = it.current.Forward[0]
	return it.current != nil
}

// Key returns the current key
func (it *SkipListIterator) Key() string {
	if it.current == nil {
		return ""
	}
	return it.current.Key
}

// Value returns the current value
func (it *SkipListIterator) Value() interface{} {
	if it.current == nil {
		return nil
	}
	return it.current.Value
}
