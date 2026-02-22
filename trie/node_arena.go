// Copyright 2026 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package trie

// NodeArena is a typed arena allocator for trie nodes. It pre-allocates
// slabs of fullNode and shortNode structs and hands them out sequentially,
// avoiding per-node heap allocations.
//
// Unlike a []byte bump allocator, this is GC-safe: the backing arrays of
// []fullNode and []shortNode are normal Go objects, so the GC can trace
// pointers inside each node (Children interfaces, Key slices, etc.).
//
// The arena is intended to be created per-block and reset after the block
// is processed. All nodes allocated from the arena become invalid after Reset.
type NodeArena struct {
	fullSlabs  [][]fullNode
	shortSlabs [][]shortNode

	fullIdx  int // index within current full slab
	shortIdx int // index within current short slab
}

const (
	// fullSlabSize is the number of fullNodes per slab.
	// fullNode is ~304 bytes, so 512 nodes ≈ 152 KB per slab.
	fullSlabSize = 512

	// shortSlabSize is the number of shortNodes per slab.
	// shortNode is ~72 bytes, so 1024 nodes ≈ 72 KB per slab.
	shortSlabSize = 1024
)

// NewNodeArena creates a new node arena with one initial slab of each type.
func NewNodeArena() *NodeArena {
	return &NodeArena{
		fullSlabs:  [][]fullNode{make([]fullNode, fullSlabSize)},
		shortSlabs: [][]shortNode{make([]shortNode, shortSlabSize)},
	}
}

// NewFullNode returns a pointer to a zeroed fullNode from the arena.
func (a *NodeArena) NewFullNode() *fullNode {
	slab := a.fullSlabs[len(a.fullSlabs)-1]
	if a.fullIdx >= len(slab) {
		// Current slab is exhausted, allocate a new one.
		slab = make([]fullNode, fullSlabSize)
		a.fullSlabs = append(a.fullSlabs, slab)
		a.fullIdx = 0
	}
	n := &slab[a.fullIdx]
	a.fullIdx++
	return n
}

// NewShortNode returns a pointer to a zeroed shortNode from the arena.
func (a *NodeArena) NewShortNode() *shortNode {
	slab := a.shortSlabs[len(a.shortSlabs)-1]
	if a.shortIdx >= len(slab) {
		// Current slab is exhausted, allocate a new one.
		slab = make([]shortNode, shortSlabSize)
		a.shortSlabs = append(a.shortSlabs, slab)
		a.shortIdx = 0
	}
	n := &slab[a.shortIdx]
	a.shortIdx++
	return n
}

// Reset resets the arena for reuse. Existing slabs are retained to avoid
// re-allocation, but all nodes are considered invalid after this call.
func (a *NodeArena) Reset() {
	// Clear all slabs to zero out stale pointers (helps GC).
	for _, slab := range a.fullSlabs {
		clear(slab)
	}
	for _, slab := range a.shortSlabs {
		clear(slab)
	}
	// Keep only the first slab of each type, drop extras.
	if len(a.fullSlabs) > 1 {
		a.fullSlabs = a.fullSlabs[:1]
	}
	if len(a.shortSlabs) > 1 {
		a.shortSlabs = a.shortSlabs[:1]
	}
	a.fullIdx = 0
	a.shortIdx = 0
}

// Stats returns the number of allocated nodes of each type.
func (a *NodeArena) Stats() (fullNodes, shortNodes int) {
	fullNodes = (len(a.fullSlabs)-1)*fullSlabSize + a.fullIdx
	shortNodes = (len(a.shortSlabs)-1)*shortSlabSize + a.shortIdx
	return
}
