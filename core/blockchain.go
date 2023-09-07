package core

import (
	"errors"
	"sync/atomic"

	"github.com/SipengXie/pangu/common"
	"github.com/SipengXie/pangu/core/evm"
	"github.com/SipengXie/pangu/core/state"
	"github.com/SipengXie/pangu/core/types"
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
	blocks types.Blocks

	config        *params.ChainConfig
	gasLimit      atomic.Uint64
	statedb       *state.StateDB
	chainHeadFeed *event.Feed
	vmConfig      evm.Config

	chainmu *syncx.ClosableMutex

	scope event.SubscriptionScope
}

func NewBlokchain(config *params.ChainConfig, statedb *state.StateDB, vmConfig evm.Config) *Blockchain {
	return &Blockchain{
		blocks:        make(types.Blocks, 0),
		config:        config,
		statedb:       statedb,
		chainHeadFeed: new(event.Feed),
		chainmu:       syncx.NewClosableMutex(),
		vmConfig:      vmConfig,
	}
}

func (bc *Blockchain) VmConfig() evm.Config {
	return bc.vmConfig
}

func (bc *Blockchain) Config() *params.ChainConfig {
	return bc.config
}

func (bc *Blockchain) CurrentBlock() *types.Header {
	return bc.blocks[len(bc.blocks)-1].Header()
}

func (bc *Blockchain) GetBlock(hash common.Hash, number uint64) *types.Block {
	return bc.blocks[number]
}

func (bc *Blockchain) StateAt(root common.Hash) (*state.StateDB, error) {
	return bc.statedb, nil
}

func (bc *Blockchain) writeHeadBlock(block *types.Block, state *state.StateDB) {
	bc.blocks = append(bc.blocks, block)
	// state.Commit(false) // TODO
}

func (bc *Blockchain) writeBlockAndSetHead(block *types.Block, receipts []*types.Receipt, logs []*types.Log, state *state.StateDB, emitHeadEvent bool) (status WriteStatus, err error) {
	bc.writeHeadBlock(block, state)
	if emitHeadEvent {
		bc.chainHeadFeed.Send(types.ChainHeadEvent{Block: block})
	}
	return CanonStatTy, nil
}

func (bc *Blockchain) WriteBlockAndSetHead(block *types.Block, receipts []*types.Receipt, logs []*types.Log, state *state.StateDB, emitHeadEvent bool) (status WriteStatus, err error) {
	if !bc.chainmu.TryLock() {
		// TODO: 并发安全
		return NonStatTy, errChainStopped
	}
	defer bc.chainmu.Unlock()

	return bc.writeBlockAndSetHead(block, receipts, logs, state, emitHeadEvent)
}

// SubscribeChainHeadEvent registers a subscription of ChainHeadEvent.
func (bc *Blockchain) SubscribeChainHeadEvent(ch chan<- types.ChainHeadEvent) event.Subscription {
	return bc.scope.Track(bc.chainHeadFeed.Subscribe(ch))
}

// GetHeader 临时定义一个，在process中需要实现这个方法获取哈希值来创建evm环境
func (bc *Blockchain) GetHeader(h common.Hash, i uint64) *types.Header {
	return bc.blocks[i].Header()
}
