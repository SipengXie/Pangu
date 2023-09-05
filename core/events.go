package core

import "github.com/SipengXie/pangu/core/types"

// NewTxsEvent is posted when a batch of transactions enter the transaction pool.
type NewTxsEvent struct{ Txs []*types.Transaction }
