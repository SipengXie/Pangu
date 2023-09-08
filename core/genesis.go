package core

import (
	"github.com/SipengXie/pangu/core/types"
	"math/big"
	"time"
)

func NewGenesisBlock() *types.Block {
	header := &types.Header{
		ParentHash: types.EmptyRootHash,
		Time:       uint64(time.Now().Unix()),
		Number:     big.NewInt(0),
		GasLimit:   12345678,
	}
	txs := make([]types.Transactions, 0)
	b := types.InitBlock(header, txs)
	return b
}
