// 该文件作为中间过渡程序，连接process执行器与evm内的执行函数

package core

import (
	"errors"
	"fmt"
	"github.com/SipengXie/pangu/common"
	"github.com/SipengXie/pangu/common/math"
	"github.com/SipengXie/pangu/core/evm"
	"github.com/SipengXie/pangu/core/types"
	"github.com/SipengXie/pangu/params"
	"math/big"
)

// ApplyTransaction 连接交易与evm
func ApplyTransaction(msg *TxMessage, evmEvent *evm.EVM) (executionResult *ExecutionResult) {
	// 保存交易快照
	SnapShot := evmEvent.StateDB.Snapshot()

	// 交易预检查
	if err := PreCheck(msg, evmEvent); err != nil {
		fmt.Printf("%sERROR MSG%s   交易执行出错 in PreCheck function\n", types.FRED, types.FRESET)
		return NewExecutionResult(0, err, nil, false, nil)
	}

	var (
		GasRemainBefore  = msg.GasLimit             // 买过的汽油费，执行前
		GasRemainAfter   = uint64(0)                // 买过的汽油费，执行后
		Sender           = evm.AccountRef(msg.From) // 发送人
		ContractCreation = msg.To == nil            // 是否是合约创建交易
	)

	// 计算基础汽油费
	ExpenseGasBase, err := IntrinsicGas(msg.Data, msg.AccessList, ContractCreation)
	if err != nil {
		fmt.Printf("%sERROR MSG%s   汽油费错误 in IntrinsicGas function\n", types.FRED, types.FRESET)
		return NewExecutionResult(0, err, nil, false, nil)
	}
	if GasRemainBefore < ExpenseGasBase {
		fmt.Printf("%sERROR MSG%s   汽油费错误 in GasRemain < ExpenseGas_Base\n", types.FRED, types.FRESET)
		return NewExecutionResult(0, errors.New("gas is not enough in ExpenseGas_Base"), nil, false, nil)
	}
	GasRemainBefore -= ExpenseGasBase

	// 检查是否有足够的钱来转账
	if msg.Value.Sign() > 0 && !evmEvent.Context.CanTransfer(evmEvent.StateDB, msg.From, msg.Value) {
		fmt.Printf("%sERROR MSG%s   汽油费错误 in 没有足够的钱转账\n", types.FRED, types.FRESET)
		return NewExecutionResult(0, errors.New("insufficient funds for transfer"), nil, false, nil)
	}

	// 检查初始代码是否超出大小（合约创建）
	if ContractCreation && len(msg.Data) > params.MaxInitCodeSize {
		fmt.Printf("%sERROR MSG%s   数据错误 in 初始代码超出大小\n", types.FRED, types.FRESET)
		return NewExecutionResult(0, errors.New("max init-code size exceeded"), nil, false, nil)
	}

	// * 这里不再执行prepare函数，用户自己填写AccessList，并承担出错的风险，对接后续重构的担保人交易类型

	var (
		ReturnData     []byte                             // 交易执行后返回的数据
		EvmError       error                              // 交易执行错误
		IsParallel     bool              = msg.IsParallel // 并行组或串行组
		CanParallel    bool                               // 交易能否并行
		TrueAccessList *types.AccessList                  // 真正的AccessList 我们依然获取到真实的AccessList并返回给用户，但是不再修改
	)

	if ContractCreation {
		// 合约创建交易
		ReturnData, _, GasRemainAfter, CanParallel, EvmError = evmEvent.Create(Sender, msg.Data, GasRemainBefore, msg.Value, TrueAccessList, IsParallel)
	} else {
		// 调用合约交易
		evmEvent.StateDB.SetNonce(msg.From, evmEvent.StateDB.GetNonce(Sender.Address())+1)
		ReturnData, GasRemainAfter, CanParallel, EvmError = evmEvent.Call(Sender, *msg.To, msg.Data, GasRemainBefore, msg.Value, TrueAccessList, IsParallel)
	}
	msg.CanParallel = CanParallel

	// 并行组交易无法并行执行
	if EvmError == nil && (IsParallel && !CanParallel) {
		fmt.Printf("%sPROMPT MSG%s   并行组中的交易无法并行执行\n", types.FGREEN, types.FRESET)
		evmEvent.StateDB.RevertToSnapshot(SnapShot) // 快照回滚
		return NewExecutionResult(0, nil, nil, true, nil)
	}

	// 交易没有出错，归还剩余的汽油费
	if EvmError == nil {
		// 多余的汽油费还给用户
		fmt.Printf("%sPROMPT MSG%s   交易没有出错，归还汽油费\n", types.FGREEN, types.FRESET)
		RefundGas(ExpenseGasBase+GasRemainBefore-GasRemainAfter, GasRemainAfter, evmEvent, msg)
		// Coinbase交易
		CoinbaseFee := new(big.Int).SetUint64(ExpenseGasBase + GasRemainBefore - GasRemainAfter)
		CoinbaseFee.Mul(CoinbaseFee, msg.GasTipCap)
		evmEvent.StateDB.AddBalance(evmEvent.Context.Coinbase, CoinbaseFee)

		return NewExecutionResult(ExpenseGasBase+GasRemainBefore-GasRemainAfter, nil, ReturnData, false, TrueAccessList)
	} else {
		// 交易出错，不归还汽油费，将剩余汽油费交给被选举人
		fmt.Printf("%sPROMPT MSG%s   交易出错，不归还用户汽油费\n", types.FGREEN, types.FRESET)
		// Coinbase交易
		CoinbaseFee := new(big.Int).SetUint64(ExpenseGasBase + GasRemainBefore)
		CoinbaseFee.Mul(CoinbaseFee, msg.GasTipCap)
		evmEvent.StateDB.AddBalance(evmEvent.Context.Coinbase, CoinbaseFee)

		return NewExecutionResult(ExpenseGasBase+GasRemainBefore, EvmError, nil, false, nil)
	}
}

// PreCheck 交易预检查函数，主要检查Nonce，账户是否合法，EIP-1559相关信息
func PreCheck(msg *TxMessage, evmEvent *evm.EVM) error {
	// Nonce检查
	txNonce := evmEvent.StateDB.GetNonce(msg.From)
	msgNonce := msg.Nonce
	if txNonce < msgNonce {
		return fmt.Errorf("%w: address %v, tx: %d state: %d", errors.New("nonce too high"), msg.From.Hex(), msgNonce, txNonce)
	} else if txNonce > msgNonce {
		return fmt.Errorf("%w: address %v, tx: %d state: %d", errors.New("nonce too low"), msg.From.Hex(), msgNonce, txNonce)
	} else if txNonce+1 < txNonce {
		return fmt.Errorf("%w: address %v, nonce: %d", errors.New("nonce has max value"), msg.From.Hex(), txNonce)
	}

	// 验证发送方是否是一个外部拥有账户，而不是一个合约账户
	codeHash := evmEvent.StateDB.GetCodeHash(msg.From)
	if codeHash != (common.Hash{}) && codeHash != types.EmptyCodeHash {
		return fmt.Errorf("%w: address %v, codehash: %s", errors.New("sender not an eoa"), msg.From.Hex(), codeHash)
	}

	// EIP-1559相关检查
	if msg.GasFeeCap.BitLen() > 0 || msg.GasTipCap.BitLen() > 0 {
		if l := msg.GasFeeCap.BitLen(); l > 256 {
			return fmt.Errorf("%w: address %v, maxFeePerGas bit length: %d", errors.New("max fee per gas higher than 2^256-1"), msg.From.Hex(), l)
		}
		if l := msg.GasTipCap.BitLen(); l > 256 {
			return fmt.Errorf("%w: address %v, maxPriorityFeePerGas bit length: %d", errors.New("max priority fee per gas higher than 2^256-1"), msg.From.Hex(), l)
		}
		if msg.GasFeeCap.Cmp(msg.GasTipCap) < 0 {
			return fmt.Errorf("%w: address %v, maxPriorityFeePerGas: %s, maxFeePerGas: %s", errors.New("max priority fee per gas higher than max fee per gas"), msg.From.Hex(), msg.GasTipCap, msg.GasFeeCap)
		}
		if msg.GasFeeCap.Cmp(evmEvent.Context.BaseFee) < 0 {
			return fmt.Errorf("%w: address %v, maxFeePerGas: %s baseFee: %s", errors.New("max fee per gas less than block base fee"), msg.From.Hex(), msg.GasFeeCap, evmEvent.Context.BaseFee)
		}
	}

	return BuyGas(msg, evmEvent)
}

// BuyGas 买汽油函数，买gas limit这么多汽油费
func BuyGas(msg *TxMessage, evmEvent *evm.EVM) error {
	LimitGas := new(big.Int).SetUint64(msg.GasLimit)   // gas limit
	BalanceGas := LimitGas.Mul(LimitGas, msg.GasPrice) // gas limit * gas price
	BalanceGas.Add(BalanceGas, msg.Value)              // gas + value
	if have, want := evmEvent.StateDB.GetBalance(msg.From), BalanceGas; have.Cmp(want) < 0 {
		return fmt.Errorf("%w: address %v have %v want %v", errors.New("insufficient funds for gas * price + value"), msg.From.Hex(), have, want)
	}

	// 扣除汽油费
	evmEvent.StateDB.SubBalance(msg.From, BalanceGas)
	return nil
}

// IntrinsicGas 计算具有给定数据的消息的内在燃料
func IntrinsicGas(data []byte, accessList *types.AccessList, isContractCreation bool) (uint64, error) {
	var gas uint64
	if isContractCreation {
		gas = params.TxGasContractCreation
	} else {
		gas = params.TxGas
	}
	dataLen := uint64(len(data))
	if dataLen > 0 {
		var nz uint64
		for _, byt := range data {
			if byt != 0 {
				nz++
			}
		}
		nonZeroGas := params.TxDataNonZeroGasFrontier
		if (math.MaxUint64-gas)/nonZeroGas < nz {
			return 0, errors.New("gas uint64 overflow")
		}
		gas += nz * nonZeroGas
		z := dataLen - nz
		if (math.MaxUint64-gas)/params.TxDataZeroGas < z {
			return 0, errors.New("gas uint64 overflow")
		}
		gas += z * params.TxDataZeroGas

		if isContractCreation {
			lenWords := toWordSize(dataLen)
			if (math.MaxUint64-gas)/params.InitCodeWordGas < lenWords {
				return 0, errors.New("gas uint64 overflow")
			}
			gas += lenWords * params.InitCodeWordGas
		}
	}
	if accessList.Addresses != nil {
		gas += uint64(accessList.Len()) * params.TxAccessListAddressGas
		gas += uint64(accessList.StorageKeys()) * params.TxAccessListStorageKeyGas
	}
	return gas, nil
}

// toWordSize 返回用于初始化代码支付计算的取上整的字长大小
func toWordSize(size uint64) uint64 {
	if size > math.MaxUint64-31 {
		return math.MaxUint64/32 + 1
	}

	return (size + 31) / 32
}

// NewExecutionResult 新建ExecutionResult类型结构体
func NewExecutionResult(UsedGas uint64, Err error, ReturnData []byte, IsParallelError bool, TrueAccessList *types.AccessList) *ExecutionResult {
	return &ExecutionResult{
		UsedGas:         UsedGas,
		Err:             Err,
		ReturnData:      ReturnData,
		IsParallelError: IsParallelError,
		TrueAccessList:  TrueAccessList,
	}
}

// RefundGas 还钱
func RefundGas(GasUsed uint64, GasRemain uint64, evmEvent *evm.EVM, msg *TxMessage) {
	// Apply refund counter, capped to a refund quotient
	refund := GasUsed / 2
	if refund > evmEvent.StateDB.GetRefund() {
		refund = evmEvent.StateDB.GetRefund()
	}
	GasRemain += refund

	// Return ETH for remaining gas, exchanged at the original rate.
	remaining := new(big.Int).Mul(new(big.Int).SetUint64(GasRemain), msg.GasPrice)
	evmEvent.StateDB.AddBalance(msg.From, remaining)

	//// Also return remaining gas to the block gas counter so it is
	//// available for the next transaction.
	//st.gp.AddGas(st.gasRemaining)	TODO:汽油池去掉了
}
