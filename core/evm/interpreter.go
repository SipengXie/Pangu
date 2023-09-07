// Copyright 2014 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package evm

import (
	"fmt"

	"github.com/SipengXie/pangu/accesslist"
	"github.com/SipengXie/pangu/common"
	"github.com/SipengXie/pangu/common/math"
	"github.com/SipengXie/pangu/core/types"
	"github.com/SipengXie/pangu/crypto"
	"github.com/SipengXie/pangu/log"
)

// Config are the configuration options for the Interpreter
type Config struct {
	Tracer                  EVMLogger // Opcode logger
	NoBaseFee               bool      // Forces the EIP-1559 baseFee to 0 (needed for 0 price calls)
	EnablePreimageRecording bool      // Enables recording of SHA3/keccak preimages
	ExtraEips               []int     // Additional EIPS that are to be enabled
}

// ScopeContext contains the things that are per-call, such as stack and memory,
// but not transients like pc and gas
type ScopeContext struct {
	Memory   *Memory
	Stack    *Stack
	Contract *Contract
}

// EVMInterpreter represents an EVM interpreter
type EVMInterpreter struct {
	evm   *EVM
	table *JumpTable

	hasher    crypto.KeccakState // Keccak256 hasher instance shared across opcodes
	hasherBuf common.Hash        // Keccak256 hasher result array shared aross opcodes

	readOnly   bool   // Whether to throw on stateful modifications
	returnData []byte // Last CALL's return data for subsequent reuse
}

// NewEVMInterpreter returns a new instance of the Interpreter.
func NewEVMInterpreter(evm *EVM) *EVMInterpreter {
	// If jump table was not initialised we set the default one.
	var table *JumpTable
	switch {
	//case evm.chainRules.IsCancun:	// TODO: 暂时替换
	//	table = &cancunInstructionSet
	//case evm.chainRules.IsShanghai:
	//	table = &shanghaiInstructionSet
	//case evm.chainRules.IsMerge:
	//	table = &mergeInstructionSet
	//case evm.chainRules.IsLondon:
	//	table = &londonInstructionSet
	//case evm.chainRules.IsBerlin:
	//	table = &berlinInstructionSet
	//case evm.chainRules.IsIstanbul:
	//	table = &istanbulInstructionSet
	//case evm.chainRules.IsConstantinople:
	//	table = &constantinopleInstructionSet
	//case evm.chainRules.IsByzantium:
	//	table = &byzantiumInstructionSet
	//case evm.chainRules.IsEIP158:
	//	table = &spuriousDragonInstructionSet
	//case evm.chainRules.IsEIP150:
	//	table = &tangerineWhistleInstructionSet
	//case evm.chainRules.IsHomestead:
	//	table = &homesteadInstructionSet
	default:
		table = &frontierInstructionSet
	}
	var extraEips []int
	if len(evm.Config.ExtraEips) > 0 {
		// Deep-copy jumptable to prevent modification of opcodes in other tables
		table = copyJumpTable(table)
	}
	for _, eip := range evm.Config.ExtraEips {
		if err := EnableEIP(eip, table); err != nil {
			// Disable it, so caller can check if it's activated or not
			log.Error("EIP activation failed", "eip", eip, "error", err)
		} else {
			extraEips = append(extraEips, eip)
		}
	}
	evm.Config.ExtraEips = extraEips
	return &EVMInterpreter{evm: evm, table: table}
}

// Run loops and evaluates the contract's code with the given input data and returns
// the return byte-slice and an error if one occurred.
func (in *EVMInterpreter) Run(contract *Contract, input []byte, readOnly bool, TrueAccessList *accesslist.AccessList, IsParallel bool) (ret []byte, err error, CanParallel bool) {
	// Increment the call depth which is restricted to 1024
	in.evm.depth++
	defer func() { in.evm.depth-- }()

	CanParallel = true

	if readOnly && !in.readOnly {
		in.readOnly = true
		defer func() { in.readOnly = false }()
	}

	// Don't bother with the execution if there's no code.
	if len(contract.Code) == 0 {
		fmt.Printf("%sPROMPT MSG%s   调用合约中没有代码\n", types.FGREEN, types.FRESET)
		return nil, nil, true
	}

	var (
		op          OpCode        // current opcode
		mem         = NewMemory() // bound memory
		stack       = newstack()  // local stack
		callContext = &ScopeContext{
			Memory:   mem,
			Stack:    stack,
			Contract: contract,
		}
		pc   = uint64(0) // program counter
		cost uint64
		res  []byte // result of the opcode execution function
	)
	// Don't move this deferred function
	defer func() {
		returnStack(stack)
	}()
	contract.Input = input

	for {
		TrueAccessListPart := accesslist.NewAccessList() // 每次操作访问到的AccessList

		op = contract.GetOp(pc)
		operation := in.table[op]
		cost = operation.constantGas // For tracing
		// Validate stack
		if sLen := stack.len(); sLen < operation.minStack {
			fmt.Printf("%sERROR MSG%s   栈长度过低\n", types.FRED, types.FRESET)
			return nil, &ErrStackUnderflow{stackLen: sLen, required: operation.minStack}, true
		} else if sLen > operation.maxStack {
			fmt.Printf("%sERROR MSG%s   栈长度过大\n", types.FRED, types.FRESET)
			return nil, &ErrStackOverflow{stackLen: sLen, limit: operation.maxStack}, true
		}
		if !contract.UseGas(cost) {
			fmt.Printf("%sERROR MSG%s   无法支付合约所需的汽油费\n", types.FRED, types.FRESET)
			return nil, ErrOutOfGas, true
		}
		if operation.dynamicGas != nil {
			var memorySize uint64
			if operation.memorySize != nil {
				memSize, overflow := operation.memorySize(stack)
				if overflow {
					fmt.Printf("%sERROR MSG%s   gas变量溢出\n", types.FRED, types.FRESET)
					return nil, ErrGasUintOverflow, true
				}
				if memorySize, overflow = math.SafeMul(toWordSize(memSize), 32); overflow {
					fmt.Printf("%sERROR MSG%s   gas变量溢出\n", types.FRED, types.FRESET)
					return nil, ErrGasUintOverflow, true
				}
			}
			var dynamicCost uint64
			dynamicCost, err = operation.dynamicGas(in.evm, contract, stack, mem, memorySize)
			cost += dynamicCost // for tracing
			if err != nil || !contract.UseGas(dynamicCost) {
				fmt.Printf("%sERROR MSG%s   无法支付合约所需的汽油费\n", types.FRED, types.FRESET)
				return nil, ErrOutOfGas, true
			}
			if memorySize > 0 {
				mem.Resize(memorySize)
			}
		}

		// 并行队列判断AccessList是否冲突
		if IsParallel {
			// 获取到本次操作实际访问的AccessList
			GetTrueAccessList(op, callContext, TrueAccessListPart)
			// 暂时不需要合并AccessList，因为不需要更改AccessList

			result, _, _, _ := TrueAccessListPart.ConflictDetection(in.evm.StateDB.GetAccessList())
			if !result {
				fmt.Printf("%sPROMPT MSG%s   Run函数执行获取到的AccessList与用户定义的AccessList不同，并行程序无法串行执行\n", types.FGREEN, types.FRESET)
				CanParallel = false
			}
		} else {
			// 串行组需要返回真实的AccessList
			GetTrueAccessList(op, callContext, TrueAccessListPart)
			TrueAccessList.CombineTrueAccessList(TrueAccessListPart)
		}

		if op == CREATE || op == CREATE2 {
			CanParallel = false
			if !IsParallel {
				// create操作只在串行队列中完成
				res, err, _ = operation.executeAL(&pc, in, callContext, TrueAccessList, IsParallel)
			}
		}
		// 不执行并行队列无法并行执行的交易
		if IsParallel || CanParallel {
			if op == DELEGATECALL || op == CALL || op == STATICCALL || op == CALLCODE {
				res, err, CanParallel = operation.executeAL(&pc, in, callContext, TrueAccessList, IsParallel)
			} else {
				res, err = operation.execute(&pc, in, callContext)
			}
		}

		if err != nil {
			fmt.Printf("%sERROR MSG%s   执行出错\n", types.FRED, types.FRESET)
			break
		}

		// 并行队列交易无法并行执行，交易推出Run函数
		if IsParallel && !CanParallel {
			break
		}

		pc++
	}

	if err == errStopToken {
		err = nil // clear stop token error
	}

	return res, err, CanParallel
}
