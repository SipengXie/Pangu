package types

import (
	"math/big"

	"github.com/SipengXie/pangu/common"
)

type PanguTransaction struct {
	To       common.Address
	Nonce    uint64
	Value    *big.Int
	GasLimit uint64
	FeeCap   *big.Int
	TipCap   *big.Int

	SigAlgo   byte
	Signature []byte

	EncContent []byte // EncContent <--> {Data, AccessList}

	Data       []byte
	AccessList AccessList
}
