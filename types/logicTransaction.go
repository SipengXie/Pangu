package types

import (
	"math/big"

	"github.com/SipengXie/pangu/common"
)

type LogicTransaction struct {
	To             common.Address
	Nonce          uint64
	Value          *big.Int
	GasLimit       uint64
	EncryptionAlgo byte // 加密算法

	// 用户签名
	SigAlgo         byte
	SignatureSender []byte
	//// 担保人签名
	//SigGuarantee       byte
	//SignatureGuarantee []byte

	// 解密
	Data       []byte
	AccessList AccessList

	Content []byte // 加密 Content <-> Data, AccessList
}

type LogicTransactions []*LogicTransaction
