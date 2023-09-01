package types

import (
	"math/big"

	"github.com/SipengXie/pangu/common"
)

type LogicTransaction struct {
	To       common.Address
	Nonce    uint64
	Value    *big.Int
	GasLimit uint64

	SigAlgo   byte
	Signature []byte

	Data       []byte
	AccessList AccessList
}

type LogicTransactions []*LogicTransaction
