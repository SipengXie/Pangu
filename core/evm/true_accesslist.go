package evm

import (
	"github.com/SipengXie/pangu/accesslist"
	"github.com/SipengXie/pangu/common"
)

// GetTrueAccessList 得到当前操作实际访问到的AccessList，类型归类为*AccessList，表明可以对调用数据进行修改
func GetTrueAccessList(op OpCode, scope *ScopeContext, NewAL *accesslist.AccessList) {
	stack := scope.Stack // scope ScopeContext包含每个调用的东西，比如堆栈和内存
	stackData := stack.Data()
	stackLen := len(stackData)
	if (op == SLOAD || op == SSTORE) && stackLen >= 1 {
		slot := common.Hash(stackData[stackLen-1].Bytes32())
		//a.list.addSlot(scope.Contract.Address(), slot)
		NewAL.AccessListAddSlot(scope.Contract.Address(), slot)
	}
	if (op == EXTCODECOPY || op == EXTCODEHASH || op == EXTCODESIZE || op == BALANCE || op == SELFDESTRUCT) && stackLen >= 1 {
		addr := common.Address(stackData[stackLen-1].Bytes20())
		if ok := NewAL.AccessListIsAddressExce(addr); !ok {
			NewAL.AccessListAddAddress(addr)
		}
	}
	if (op == DELEGATECALL || op == CALL || op == STATICCALL || op == CALLCODE) && stackLen >= 5 {
		addr := common.Address(stackData[stackLen-2].Bytes20())
		if ok := NewAL.AccessListIsAddressExce(addr); !ok {
			NewAL.AccessListAddAddress(addr)
		}
	}
	if op == CREATE || op == CREATE2 {
		// TODO: 是否也会访问和修改地址
	}
}
