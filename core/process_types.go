package core

import (
	"math/big"
	"sort"

	"github.com/SipengXie/pangu/accesslist"
	"github.com/SipengXie/pangu/common"
	"github.com/SipengXie/pangu/core/evm"
	"github.com/SipengXie/pangu/core/types"
	"github.com/SipengXie/pangu/params"
	cmath "github.com/ethereum/go-ethereum/common/math"
)

// MessageReturn MesReturn 作为返回结构体，通过管道传输线程中每组交易的结果，暂时去掉了IsScusses，因为这是执行函数，不是验证函数
type MessageReturn struct {
	// 成功交易收据树
	NewReceipt []*types.Receipt
	// logs
	NewLogs []*types.Log
	// 串行队列
	TxSerial []*types.Transaction
	// 出错交易
	TxError []*TxErrorMessage
	// AccessList有问题的交易
	TxAccessList []*TxAccessListMessage
	// 当前线程返回的CoinbaseFeePart
	CoinbaseFeeThread *big.Int
}

// TxErrorMessage 首次执行时记录每组中执行错误的交易错误信息
type TxErrorMessage struct {
	Tx       *types.Transaction // 出错交易
	Result   string             // 出错原因
	ErrorMsg error              // 具体错误
}

// TxAccessListMessage 首次执行时记录每组中需要更改AccessList的交易
type TxAccessListMessage struct {
	Tx             *types.Transaction // 出错交易
	TrueAccessList *accesslist.AccessList
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
	AccessList  *accesslist.AccessList
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
	TrueAccessList  *accesslist.AccessList
	Coinbase        big.Int // 给矿工多少钱
}

// ProcessReturnMsg Process函数返回结构体
type ProcessReturnMsg struct {
	Receipt  types.Receipts
	Logs     []*types.Log
	ErrTx    []*TxErrorMessage
	AlTx     []*TxAccessListMessage
	UsedGas  *uint64
	RootHash common.Hash
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
		GasPrice:          baseFee.Add(baseFee, tipfee), // ! error
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

// SortSerialTX 串行队列排序方法
func SortSerialTX(SingleTxList []*types.Transaction) {
	// 使用 sort.Slice 排序函数对第二项进行排序
	sort.Slice(SingleTxList, func(i, j int) bool {
		// todo：1 Nonce从小到大
		if SingleTxList[i].Nonce() < SingleTxList[j].Nonce() {
			return true
		} else if SingleTxList[i].Nonce() == SingleTxList[j].Nonce() {
			// todo：2 GasPrice从大到小
			if SingleTxList[i].GasPrice().Cmp(SingleTxList[j].GasPrice()) == 1 {
				return true
			} else if SingleTxList[i].GasPrice().Cmp(SingleTxList[j].GasPrice()) == 0 {
				// todo：3 哈希值从小到大
				if SingleTxList[i].Hash().Less(SingleTxList[j].Hash()) {
					return true
				}
			}
		}
		return false
	})
}

func NewProcessReturnMsg(Receipts types.Receipts, AllLogs []*types.Log, ErrorTxList []*TxErrorMessage, AccessListTx []*TxAccessListMessage, UsedGas *uint64, RootHash common.Hash) *ProcessReturnMsg {
	return &ProcessReturnMsg{
		Receipt:  Receipts,
		Logs:     AllLogs,
		ErrTx:    ErrorTxList,
		AlTx:     AccessListTx,
		UsedGas:  UsedGas,
		RootHash: RootHash,
	}
}
