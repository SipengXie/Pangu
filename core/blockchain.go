package core

import (
	"encoding/binary"
	"errors"
	"os"
	"sync/atomic"

	"github.com/boltdb/bolt"

	"github.com/SipengXie/pangu/common"
	"github.com/SipengXie/pangu/core/evm"
	"github.com/SipengXie/pangu/core/state"
	"github.com/SipengXie/pangu/core/types"
	"github.com/SipengXie/pangu/event"
	"github.com/SipengXie/pangu/params"
	"github.com/SipengXie/pangu/rlp"
	"github.com/SipengXie/pangu/utils/syncx"
)

var (
	//dbFile          = "blockchain.db"
	blocksBucket    = "blocks"
	headerBucket    = "headers"
	errChainStopped = errors.New("chain stopped")
)

type WriteStatus byte

const (
	NonStatTy WriteStatus = iota
	CanonStatTy
	SideStatTy
)

type Blockchain struct {
	head *types.Header
	db   *bolt.DB
	// blocks    types.Blocks

	config        *params.ChainConfig
	gasLimit      atomic.Uint64
	statedb       *state.StateDB
	chainHeadFeed *event.Feed
	vmConfig      evm.Config

	chainmu *syncx.ClosableMutex

	scope event.SubscriptionScope
}

func dbExists(dbFile string) bool {
	if _, err := os.Stat(dbFile); os.IsNotExist(err) {
		return false
	}

	return true
}

func NewBlokchain(config *params.ChainConfig, db *bolt.DB, statedb *state.StateDB, vmConfig evm.Config) *Blockchain {
	//// 打开DB
	//db, err := bolt.Open(dbFile, 0600, nil)
	//if err != nil {
	//	panic(err)
	//}

	bc := &Blockchain{
		db:            db,
		config:        config,
		statedb:       statedb,
		chainHeadFeed: new(event.Feed),
		chainmu:       syncx.NewClosableMutex(),
		vmConfig:      vmConfig,
	}
	b := NewGenesisBlock()
	blkData, err := rlp.EncodeToBytes(b)
	if err != nil {
		panic(err)
	}

	err = db.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucket([]byte(blocksBucket))
		if err != nil {
			panic(err)
		}

		err = bucket.Put(b.Hash().Bytes(), blkData)
		if err != nil {
			panic(err)
		}

		err = bucket.Put([]byte("head"), b.Hash().Bytes())
		if err != nil {
			panic(err)
		}

		hbucket, err := tx.CreateBucket([]byte(headerBucket))
		if err != nil {
			panic(err)
		}

		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, b.NumberU64())

		err = hbucket.Put(buf, b.Header().Hash().Bytes())
		if err != nil {
			panic(err)
		}

		bc.head = b.Header()
		return nil
	})

	return bc
}

func (bc *Blockchain) VmConfig() evm.Config {
	return bc.vmConfig
}

func (bc *Blockchain) Config() *params.ChainConfig {
	return bc.config
}

func (bc *Blockchain) CurrentBlock() *types.Header {
	return bc.head
}

func (bc *Blockchain) GetBlock(hash common.Hash, number uint64) *types.Block {
	var blk types.Block
	if (hash != common.Hash{}) {
		err := bc.db.View(func(tx *bolt.Tx) error {
			b := tx.Bucket([]byte(blocksBucket))
			blkData := b.Get(hash.Bytes())
			return rlp.DecodeBytes(blkData, &blk)
		})
		if err != nil {
			panic(err)
		}
		return &blk
	}

	if (hash == common.Hash{}) {
		err := bc.db.View(func(tx *bolt.Tx) error {
			hb := tx.Bucket([]byte(headerBucket))
			buf := make([]byte, 8)
			binary.BigEndian.PutUint64(buf, number)

			hash := hb.Get(buf)
			b := tx.Bucket([]byte(blocksBucket))
			blkData := b.Get(hash)
			return rlp.DecodeBytes(blkData, &blk)
		})
		if err != nil {
			panic(err)
		}
		return &blk
	}

	return nil
}

func (bc *Blockchain) StateAt(root common.Hash) (*state.StateDB, error) {
	return bc.statedb, nil
}

func (bc *Blockchain) WriteBlockAndSetHead(block *types.Block, receipts []*types.Receipt, logs []*types.Log, state *state.StateDB, emitHeadEvent bool) (status WriteStatus, err error) {
	if !bc.chainmu.TryLock() {
		// TODO: 并发安全
		return NonStatTy, errChainStopped
	}
	defer bc.chainmu.Unlock()

	blkData, err := rlp.EncodeToBytes(block)

	// without any check
	err = bc.db.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte(blocksBucket))
		if err != nil {
			panic(err)
		}

		err = bucket.Put(block.Hash().Bytes(), blkData)
		if err != nil {
			panic(err)
		}

		err = bucket.Put([]byte("head"), block.Hash().Bytes())
		if err != nil {
			panic(err)
		}

		hbucket := tx.Bucket([]byte(headerBucket))
		if err != nil {
			panic(err)
		}

		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, block.NumberU64())

		err = hbucket.Put(buf, block.Header().Hash().Bytes())
		if err != nil {
			panic(err)
		}

		bc.head = block.Header()
		return nil
	})

	if emitHeadEvent {
		bc.chainHeadFeed.Send(types.ChainHeadEvent{Block: block})
	}

	return CanonStatTy, nil
}

// SubscribeChainHeadEvent registers a subscription of ChainHeadEvent.
func (bc *Blockchain) SubscribeChainHeadEvent(ch chan<- types.ChainHeadEvent) event.Subscription {
	return bc.scope.Track(bc.chainHeadFeed.Subscribe(ch))
}

// GetHeader 临时定义一个，在process中需要实现这个方法获取哈希值来创建evm环境
func (bc *Blockchain) GetHeader(h common.Hash, i uint64) *types.Header {
	return bc.GetBlock(h, i).Header()
}
