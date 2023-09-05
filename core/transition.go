// 该文件作为中间过渡程序，连接process执行器与evm内的执行函数

package core

import "github.com/SipengXie/pangu/core/evm"

func ApplyTransaction(msg *TxMessage, evm *evm.EVM) (executionResult *ExecutionResult, err error) {
	return nil, nil
}
