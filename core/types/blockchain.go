package types

import (
	"errors"
	"sync/atomic"

	"github.com/SipengXie/pangu/common"
	"github.com/SipengXie/pangu/core/state"
	"github.com/SipengXie/pangu/event"
	"github.com/SipengXie/pangu/params"
	"github.com/SipengXie/pangu/utils/syncx"
)

var (
	errChainStopped = errors.New("chain stopped")
)

type WriteStatus byte

const (
	NonStatTy WriteStatus = iota
	CanonStatTy
	SideStatTy
)

type Blockchain struct {
	blocks Blocks

	config        *params.ChainConfig
	gasLimit      atomic.Uint64
	statedb       *state.StateDB
	chainHeadFeed *event.Feed

	chainmu *syncx.ClosableMutex
}

func NewBlokchain(config *params.ChainConfig, statedb *state.StateDB) *Blockchain {
	return &Blockchain{
		blocks:        make(Blocks, 0),
		config:        config,
		statedb:       statedb,
		chainHeadFeed: new(event.Feed),
		chainmu:       syncx.NewClosableMutex(),
	}
}

func (bc *Blockchain) Config() *params.ChainConfig {
	return bc.config
}

func (bc *Blockchain) CurrentBlock() *Block {
	return bc.blocks[len(bc.blocks)-1]
}

func (bc *Blockchain) GetBlock(hash common.Hash, number uint64) *Block {
	return bc.blocks[number]
}

func (bc *Blockchain) StateAt(root common.Hash) (*state.StateDB, error) {
	return bc.statedb, nil
}

func (bc *Blockchain) writeHeadBlock(block *Block) {
	bc.blocks = append(bc.blocks, block)
	state.Commit(false) // TODO
}

func (bc *Blockchain) writeBlockAndSetHead(block *Block, receipts []*Receipt, logs []*Log, state *state.StateDB, emitHeadEvent bool) (status WriteStatus, err error) {
	bc.writeHeadBlock(block)
	if emitHeadEvent {
		bc.chainHeadFeed.Send(ChainHeadEvent{Block: block})
	}
	return CanonStatTy, nil
}

func (bc *Blockchain) WriteBlockAndSetHead(block *Block, receipts []*Receipt, logs []*Log, state *state.StateDB, emitHeadEvent bool) (status WriteStatus, err error) {
	if !bc.chainmu.TryLock() {
		// TODO: 并发安全
		return NonStatTy, errChainStopped
	}
	defer bc.chainmu.Unlock()

	return bc.writeBlockAndSetHead(block, receipts, logs, state, emitHeadEvent)
}
