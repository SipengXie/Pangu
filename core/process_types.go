package core

import (
	"github.com/SipengXie/pangu/common"
	"github.com/SipengXie/pangu/core/evm"
	"github.com/SipengXie/pangu/core/types"
	"github.com/SipengXie/pangu/params"
	cmath "github.com/ethereum/go-ethereum/common/math"
	"math/big"
)

// MessageReturn MesReturn 作为返回结构体，通过管道传输线程中每组交易的结果，暂时去掉了IsScusses，因为这是执行函数，不是验证函数
type MessageReturn struct {
	// 成功交易收据树
	NewReceipt []*types.Receipt
	// logs
	NewLogs []*types.Log
	// 串行队列
	SingleTx []*types.Transaction
	// 出错交易
	TxError []*TxErrorMessage
}

// TxErrorMessage 首次执行时记录每组中执行错误的交易错误信息
type TxErrorMessage struct {
	Tx       *types.Transaction // 出错交易
	Result   string             // 出错原因
	ErrorMsg error              // 具体错误
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
	// BaseFee     *big.Int // 获取区块头中的BaseFee
}

// TxMessage 实际交易执行传递的信息
type TxMessage struct {
	To          *common.Address
	From        common.Address
	Nonce       uint64
	Value       *big.Int
	GasLimit    uint64
	GasPrice    *big.Int // 最终的价格 = 基础费 + 小费
	GasFeeCap   *big.Int // Max Fee
	GasTipCap   *big.Int // 小费
	Data        []byte
	AccessList  *types.AccessList
	BlobHashes  []common.Hash
	IsParallel  bool // 是否是并行队列
	CanParallel bool // 交易能否并行执行

	// 当SkipAccountChecks为true时，消息的nonce不会与状态中的账户nonce进行检查
	SkipAccountChecks bool
}

// ExecutionResult ApplyTransaction函数执行后返回值
type ExecutionResult struct {
	UsedGas         uint64
	Err             error
	ReturnData      []byte // Returned data from evm(function result or data supplied with revert opcode)
	IsParallelError bool   // 标识当前错误是否时因为交易无法并行导致的错误
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

// TransactionToMessage converts a transaction into a Message. TODO: 新增参数 IsParallel bool
func TransactionToMessage(tx *types.Transaction, s types.Signer, baseFee *big.Int, IsParallel bool) (*TxMessage, error) {
	tipfee := new(big.Int).Set(tx.GasTipCap())
	msg := &TxMessage{
		Nonce:             tx.Nonce(),
		GasLimit:          tx.GasLimit(),
		GasPrice:          baseFee.Add(baseFee, tipfee),
		GasFeeCap:         new(big.Int).Set(tx.GasFeeCap()),
		GasTipCap:         new(big.Int).Set(tx.GasTipCap()),
		To:                tx.To(),
		Value:             tx.Value(),
		Data:              tx.Data(),
		AccessList:        tx.AccessList(),
		SkipAccountChecks: false,
		BlobHashes:        make([]common.Hash, 0),
		IsParallel:        IsParallel,
		CanParallel:       true,
	}

	// 当前交易在串行队列
	if !msg.IsParallel {
		msg.CanParallel = false
	}
	// If baseFee provided, set gasPrice to effectiveGasPrice.
	if baseFee != nil {
		msg.GasPrice = cmath.BigMin(msg.GasPrice.Add(msg.GasTipCap, baseFee), msg.GasFeeCap)
	}
	var err error
	msg.From, err = types.Sender(s, tx)
	return msg, err
}

// NewTxErrorMessage 创建一个交易错误原因
func NewTxErrorMessage(tx *types.Transaction, result string, err error) *TxErrorMessage {
	return &TxErrorMessage{
		Tx:       tx,
		Result:   result,
		ErrorMsg: err,
	}
}
