// Copyright 2024 The go-ethereum Authors
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

package core

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/lru"
	"github.com/ethereum/go-ethereum/consensus/beacon"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/stateless"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/trie"
	"github.com/ethereum/go-ethereum/triedb"
)

// ExecuteStateless runs a stateless execution based on a witness, fully
// self-validating the block (header, body, state root and receipt root).
//
// This method is a bit of a sore thumb here, but:
//   - It cannot be placed in core/stateless, because state.New produces a circular dep
//   - It cannot be placed outside of core, because it needs to construct a dud headerchain
//
// TODO(karalabe): Would be nice to resolve both issues above somehow and move it.
func ExecuteStateless(ctx context.Context, config *params.ChainConfig, vmconfig vm.Config, block *types.Block, witness *stateless.Witness) (common.Hash, common.Hash, error) {
	// Create and populate the state database to serve as the stateless backend
	memdb := witness.MakeHashDB()
	db, err := state.New(witness.Root(), state.NewDatabase(triedb.NewDatabase(memdb, triedb.HashDefaults), state.NewCodeDB(memdb)))
	if err != nil {
		return common.Hash{}, common.Hash{}, err
	}
	// Create a blockchain that is idle, but can be used to access headers through
	engine := beacon.New(ethash.NewFaker())
	chain := &HeaderChain{
		config:      config,
		chainDb:     memdb,
		headerCache: lru.NewCache[common.Hash, *types.Header](256),
		engine:      engine,
	}
	// Pre-execution validation: verify the block header and body
	if err := engine.VerifyHeader(chain, block.Header()); err != nil {
		return common.Hash{}, common.Hash{}, fmt.Errorf("header verification failed: %w", err)
	}
	validator := NewBlockValidator(config, nil) // No chain needed for body validation
	if err := validator.ValidateBody(block); err != nil {
		return common.Hash{}, common.Hash{}, fmt.Errorf("body validation failed: %w", err)
	}
	// Execute the block
	processor := NewStateProcessor(chain)
	res, err := processor.Process(ctx, block, db, vmconfig)
	if err != nil {
		return common.Hash{}, common.Hash{}, err
	}
	// Post-execution validation: gas used, bloom filter
	if err = validator.ValidateState(block, db, res, true); err != nil {
		return common.Hash{}, common.Hash{}, err
	}
	// Compute and verify the state root and receipt root
	receiptRoot := types.DeriveSha(res.Receipts, trie.NewStackTrie(nil))
	stateRoot := db.IntermediateRoot(config.IsEIP158(block.Number()))

	if stateRoot != block.Root() {
		return stateRoot, receiptRoot, fmt.Errorf("state root mismatch: computed %x, expected %x", stateRoot, block.Root())
	}
	if receiptRoot != block.ReceiptHash() {
		return stateRoot, receiptRoot, fmt.Errorf("receipt root mismatch: computed %x, expected %x", receiptRoot, block.ReceiptHash())
	}
	return stateRoot, receiptRoot, nil
}
