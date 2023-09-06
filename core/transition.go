// 该文件作为中间过渡程序，连接process执行器与evm内的执行函数

package core

import (
	"errors"
	"fmt"
	"github.com/SipengXie/pangu/common"
	"github.com/SipengXie/pangu/core/evm"
	"github.com/SipengXie/pangu/core/types"
	"math/big"
)

// ApplyTransaction 连接交易与evm
func ApplyTransaction(msg *TxMessage, evm *evm.EVM) (executionResult *ExecutionResult, err error) {
	// 保存交易快照
	SnapShot := evm.StateDB.Snapshot()

	// 交易预检查
	if err := PreCheck(msg, evm); err != nil {
		fmt.Printf("%sERROR MSG%s   交易执行出错 in PreCheck function\n", types.FRED, types.FRESET)
		return nil, err
	}

	GasRemain := msg.GasLimit // 买过的汽油费
	
	return nil, nil
}

// PreCheck 交易预检查函数，主要检查Nonce，账户是否合法，EIP-1559相关信息
func PreCheck(msg *TxMessage, evm *evm.EVM) error {
	// Nonce检查
	txNonce := evm.StateDB.GetNonce(msg.From)
	msgNonce := msg.Nonce
	if txNonce < msgNonce {
		return fmt.Errorf("%w: address %v, tx: %d state: %d", errors.New("nonce too high"), msg.From.Hex(), msgNonce, txNonce)
	} else if txNonce > msgNonce {
		return fmt.Errorf("%w: address %v, tx: %d state: %d", errors.New("nonce too low"), msg.From.Hex(), msgNonce, txNonce)
	} else if txNonce+1 < txNonce {
		return fmt.Errorf("%w: address %v, nonce: %d", errors.New("nonce has max value"), msg.From.Hex(), txNonce)
	}

	// 验证发送方是否是一个外部拥有账户，而不是一个合约账户
	codeHash := evm.StateDB.GetCodeHash(msg.From)
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
		if msg.GasFeeCap.Cmp(evm.Context.BaseFee) < 0 {
			return fmt.Errorf("%w: address %v, maxFeePerGas: %s baseFee: %s", errors.New("max fee per gas less than block base fee"), msg.From.Hex(), msg.GasFeeCap, evm.Context.BaseFee)
		}
	}

	return BuyGas(msg, evm)
}

// BuyGas 买汽油函数，买gas limit这么多汽油费
func BuyGas(msg *TxMessage, evm *evm.EVM) error {
	LimitGas := new(big.Int).SetUint64(msg.GasLimit)   // gas limit
	BalanceGas := LimitGas.Mul(LimitGas, msg.GasPrice) // gas limit * gas price
	BalanceGas.Add(BalanceGas, msg.Value)              // gas + value
	if have, want := evm.StateDB.GetBalance(msg.From), BalanceGas; have.Cmp(want) < 0 {
		return fmt.Errorf("%w: address %v have %v want %v", errors.New("insufficient funds for gas * price + value"), msg.From.Hex(), have, want)
	}

	// 扣除汽油费
	evm.StateDB.SubBalance(msg.From, BalanceGas)
	return nil
}
