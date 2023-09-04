package types

import (
	"math/big"

	"github.com/SipengXie/pangu/common"
)

type PanguTransaction struct {
	// >> 交易封皮 <<
	// 基础数据
	To    common.Address
	Nonce uint64
	Value *big.Int
	// EIP-1559
	GasLimit uint64   // 单笔交易汽油费数量上限
	FeeCap   *big.Int // 用户自定义的小费价格
	TipCap   *big.Int // 用户愿意支付的最大汽油费价格 = 2 * Base Fee + TipCap
	// 交易签名
	SigAlgo   byte   // 用户选择的签名算法
	Signature []byte // 用户签名
	// 加密过后的内容
	EncContent []byte // EncContent <--> {Data, AccessList}

	// >> 交易内核 <<
	Data       []byte
	AccessList AccessList
}
