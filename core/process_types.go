package core

import (
	"github.com/SipengXie/pangu/common"
	"github.com/SipengXie/pangu/core/evm"
	"github.com/SipengXie/pangu/core/types"
	"github.com/SipengXie/pangu/params"
	"math/big"
)

// MessageReturn MesReturn 作为返回结构体，通过管道传输线程中每组交易的结果，暂时去掉了IsScusses，因为这是执行函数，不是验证函数
type MessageReturn struct {
	// 交易出错原因，字符串
	ErrorMessage string
	// err
	Error error
	// 成功交易收据树
	NewReceipt []*types.Receipt
	// logs
	NewLogs []*types.Log
	// 串行队列
	SingleTx []*types.Transaction
}

// ThreadMessage 作为多线程函数的传入参数，以一个线程为单位
type ThreadMessage struct {
	Config      *params.ChainConfig
	BlockNumber *big.Int
	BlockHash   common.Hash
	UsedGas     *uint64
	EVMenv      *evm.EVM
	Signer      types.Signer
	Header      *types.Header
}

// NewThreadMessage 新建，复制AllMessage结构体
func NewThreadMessage(Config *params.ChainConfig, BlockNumber *big.Int,
	BlockHash common.Hash, UsedGas *uint64, EVMenv *evm.EVM, Signer types.Signer, Header *types.Header) *ThreadMessage {
	ThreadMessage := &ThreadMessage{
		Config:      Config,
		BlockNumber: BlockNumber,
		BlockHash:   BlockHash,
		UsedGas:     UsedGas,
		EVMenv:      EVMenv,
		Signer:      Signer,
		Header:      Header,
	}
	return ThreadMessage
}
