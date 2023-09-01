package types

import "math/big"

type CoverTransaction struct {
	SigAlgo   byte     // specifies the signature algorithm used
	Signature []byte   // signature of the transaction
	GasLimit  uint64   // gas limit of the transaction, which sums up all the logic txs
	GasPrice  *big.Int // gas price of the transaction, determined by the CoverTxCreator

	IsGuaranteed bool // whether the transaction is guaranteed，如果这个交易是被担保的，则任何出错回滚将触发惩罚机制
	LogicTxs     []*LogicTransaction
}
