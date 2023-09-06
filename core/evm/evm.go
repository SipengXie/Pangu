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
	"errors"
	"fmt"
	"github.com/SipengXie/pangu/core"
	"math/big"
	"sync/atomic"

	evmparams "github.com/SipengXie/pangu/core/evm/params"

	"github.com/SipengXie/pangu/common"
	"github.com/SipengXie/pangu/core/types"
	"github.com/SipengXie/pangu/crypto"
	"github.com/holiman/uint256"
)

type (
	CanTransferFunc func(StateDB, common.Address, *big.Int) bool
	TransferFunc    func(StateDB, common.Address, common.Address, *big.Int)
	GetHashFunc     func(uint64) common.Hash
)

func (evm *EVM) precompile(addr common.Address) (PrecompiledContract, bool) {
	var precompiles map[common.Address]PrecompiledContract
	switch {
	default:
		precompiles = PrecompiledContractsHomestead
	}
	p, ok := precompiles[addr]
	return p, ok
}

type BlockContext struct {
	CanTransfer CanTransferFunc
	Transfer    TransferFunc
	GetHash     GetHashFunc

	// Block information
	Coinbase    common.Address // Provides information for COINBASE
	GasLimit    uint64         // Provides information for GASLIMIT
	BlockNumber *big.Int       // Provides information for NUMBER
	Time        uint64         // Provides information for TIME
	BaseFee     *big.Int       // Provides information for BASEFEE
	Random      *common.Hash
}

// TxContext provides the EVM with information about a transaction.
// All fields can change between transactions.
type TxContext struct {
	// Message information
	Origin     common.Address // Provides information for ORIGIN
	GasPrice   *big.Int       // Provides information for GASPRICE
	BlobHashes []common.Hash  // Provides information for BLOBHASH
}

// EVM is the Ethereum Virtual Machine base object and provides
// the necessary tools to run a contract on the given state with
// the provided context. It should be noted that any error
// generated through any of the calls should be considered a
// revert-state-and-consume-all-gas operation, no checks on
// specific errors should ever be performed. The interpreter makes
// sure that any errors generated are to be considered faulty code.
//
// The EVM should never be reused and is not thread safe.
type EVM struct {
	// Context provides auxiliary blockchain related information
	Context BlockContext
	TxContext
	// StateDB gives access to the underlying state
	StateDB StateDB
	// Depth is the current call stack
	depth int

	// chainConfig contains information about the current chain
	chainConfig *evmparams.ChainConfig
	// chain rules contains the chain rules for the current epoch
	chainRules evmparams.Rules
	// virtual machine configuration options used to initialise the
	// evm.
	Config Config
	// global (to this context) ethereum virtual machine
	// used throughout the execution of the tx.
	interpreter *EVMInterpreter
	// abort is used to abort the EVM calling operations
	abort atomic.Bool
	// callGasTemp holds the gas available for the current call. This is needed because the
	// available gas is calculated in gasCall* according to the 63/64 rule and later
	// applied in opCall*.
	callGasTemp uint64
}

// NewEVM returns a new EVM. The returned EVM is not thread safe and should
// only ever be used *once*.
func NewEVM(blockCtx BlockContext, txCtx TxContext, statedb StateDB, chainConfig *evmparams.ChainConfig, config Config) *EVM {
	evm := &EVM{
		Context:     blockCtx,
		TxContext:   txCtx,
		StateDB:     statedb,
		Config:      config,
		chainConfig: chainConfig,
		chainRules:  chainConfig.Rules(blockCtx.BlockNumber, blockCtx.Time),
	}
	evm.interpreter = NewEVMInterpreter(evm)
	return evm
}

// Reset resets the EVM with a new transaction context.Reset
// This is not threadsafe and should only be done very cautiously.
func (evm *EVM) Reset(txCtx TxContext, statedb StateDB) {
	evm.TxContext = txCtx
	evm.StateDB = statedb
}

// Cancel cancels any running EVM operation. This may be called concurrently and
// it's safe to be called multiple times.
func (evm *EVM) Cancel() {
	evm.abort.Store(true)
}

// Cancelled returns true if Cancel has been called
func (evm *EVM) Cancelled() bool {
	return evm.abort.Load()
}

// Interpreter returns the current interpreter
func (evm *EVM) Interpreter() *EVMInterpreter {
	return evm.interpreter
}

// SetBlockContext updates the block context of the EVM.
func (evm *EVM) SetBlockContext(blockCtx BlockContext) {
	evm.Context = blockCtx
	num := blockCtx.BlockNumber
	timestamp := blockCtx.Time
	evm.chainRules = evm.chainConfig.Rules(num, timestamp)
}

// Call executes the contract associated with the addr with the given input as
// parameters. It also handles any necessary value transfer required and takes
// the necessary steps to create accounts and reverses the state in case of an
// execution error or failed value transfer.
// input -> msg.Data value -> msg.Value addr -> msg.To
func (evm *EVM) Call(caller ContractRef, msg *core.TxMessage, gas uint64, TrueAccessList *types.AccessList, IsParallel bool) (ret []byte, GasRemain uint64, CanParallel bool, err error) {
	// 调用深度检查
	if evm.depth > int(evmparams.CallCreateDepth) {
		fmt.Printf("%sERROR MSG%s   调用深度超出限制\n", types.FRED, types.FRESET)
		return nil, gas, true, ErrDepth
	}

	// 余额是否可以转账
	if msg.Value.Sign() != 0 && !evm.Context.CanTransfer(evm.StateDB, caller.Address(), msg.Value) {
		fmt.Printf("%sERROR MSG%s   余额不足，不能转账\n", types.FRED, types.FRESET)
		return nil, gas, true, ErrInsufficientBalance
	}
	snapshot := evm.StateDB.Snapshot()
	p, isPrecompile := evm.precompile(*msg.To)

	if !evm.StateDB.Exist(*msg.To) {
		if !isPrecompile && msg.Value.Sign() == 0 { // TODO: remove "evm.chainRules.IsEIP158"
			fmt.Printf("%sERROR MSG%s   调用合约地址不存在\n", types.FRED, types.FRESET)
			return nil, gas, true, errors.New("contract address is not exist")
		}
		evm.StateDB.CreateAccount(*msg.To)
	}

	// 转账交易
	evm.Context.Transfer(evm.StateDB, caller.Address(), *msg.To, msg.Value)

	if isPrecompile {
		ret, gas, err = RunPrecompiledContract(p, msg.Data, gas)
	} else {
		// 不是预编译合约，初始化一个新的合约，并设置EVM要使用的代码
		code := evm.StateDB.GetCode(*msg.To)
		if len(code) == 0 {
			fmt.Printf("%sPROMPT MSG%s   调用合约中没有代码\n", types.FGREEN, types.FRESET)
			ret, err = nil, nil // gas is unchanged
			return nil, gas, true, nil
		} else {
			// 调用合约中有代码
			addrCopy := *msg.To
			contract := NewContract(caller, AccountRef(addrCopy), msg.Value, gas)
			contract.SetCallCode(&addrCopy, evm.StateDB.GetCodeHash(addrCopy), code)
			ret, err, CanParallel = evm.interpreter.Run(contract, msg.Data, false, TrueAccessList, IsParallel)
			gas = contract.Gas
		}
	}

	if err != nil {
		fmt.Printf("%sERROR MSG%s   交易执行出错\n", types.FRED, types.FRESET)
		evm.StateDB.RevertToSnapshot(snapshot)
	}
	return ret, gas, CanParallel, err
}

// CallCode executes the contract associated with the addr with the given input
// as parameters. It also handles any necessary value transfer required and takes
// the necessary steps to create accounts and reverses the state in case of an
// execution error or failed value transfer.
//
// CallCode differs from Call in the sense that it executes the given address'
// code with the caller as context.
func (evm *EVM) CallCode(caller ContractRef, addr common.Address, input []byte, gas uint64, value *big.Int, TrueAccessList *types.AccessList, IsParallel bool) (ret []byte, GasRemain uint64, CanParallel bool, err error) {
	CanParallel = true
	// Fail if we're trying to execute above the call depth limit
	if evm.depth > int(evmparams.CallCreateDepth) {
		return nil, gas, true, ErrDepth
	}

	if !evm.Context.CanTransfer(evm.StateDB, caller.Address(), value) {
		return nil, gas, true, ErrInsufficientBalance
	}
	var snapshot = evm.StateDB.Snapshot()

	// Invoke tracer hooks that signal entering/exiting a call frame
	if evm.Config.Tracer != nil {
		evm.Config.Tracer.CaptureEnter(CALLCODE, caller.Address(), addr, input, gas, value)
		defer func(startGas uint64) {
			evm.Config.Tracer.CaptureExit(ret, startGas-gas, err)
		}(gas)
	}

	// It is allowed to call precompiles, even via delegatecall
	if p, isPrecompile := evm.precompile(addr); isPrecompile {
		ret, gas, err = RunPrecompiledContract(p, input, gas)
	} else {
		addrCopy := addr
		// Initialise a new contract and set the code that is to be used by the EVM.
		// The contract is a scoped environment for this execution context only.
		contract := NewContract(caller, AccountRef(caller.Address()), value, gas)
		contract.SetCallCode(&addrCopy, evm.StateDB.GetCodeHash(addrCopy), evm.StateDB.GetCode(addrCopy))
		ret, err, CanParallel = evm.interpreter.Run(contract, input, false, TrueAccessList, IsParallel)
		gas = contract.Gas
	}
	if err != nil {
		evm.StateDB.RevertToSnapshot(snapshot)
	}
	return ret, gas, CanParallel, err
}

// DelegateCall executes the contract associated with the addr with the given input
// as parameters. It reverses the state in case of an execution error.
//
// DelegateCall differs from CallCode in the sense that it executes the given address'
// code with the caller as context and the caller is set to the caller of the caller.
func (evm *EVM) DelegateCall(caller ContractRef, addr common.Address, input []byte, gas uint64, TrueAccessList *types.AccessList, IsParallel bool) (ret []byte, GasRemain uint64, CanParallel bool, err error) {
	CanParallel = true
	// Fail if we're trying to execute above the call depth limit
	if evm.depth > int(evmparams.CallCreateDepth) {
		return nil, gas, true, ErrDepth
	}
	var snapshot = evm.StateDB.Snapshot()

	// Invoke tracer hooks that signal entering/exiting a call frame
	if evm.Config.Tracer != nil {
		// NOTE: caller must, at all times be a contract. It should never happen
		// that caller is something other than a Contract.
		parent := caller.(*Contract)
		// DELEGATECALL inherits value from parent call
		evm.Config.Tracer.CaptureEnter(DELEGATECALL, caller.Address(), addr, input, gas, parent.value)
		defer func(startGas uint64) {
			evm.Config.Tracer.CaptureExit(ret, startGas-gas, err)
		}(gas)
	}

	// It is allowed to call precompiles, even via delegatecall
	if p, isPrecompile := evm.precompile(addr); isPrecompile {
		ret, gas, err = RunPrecompiledContract(p, input, gas)
	} else {
		addrCopy := addr
		// Initialise a new contract and make initialise the delegate values
		contract := NewContract(caller, AccountRef(caller.Address()), nil, gas).AsDelegate()
		contract.SetCallCode(&addrCopy, evm.StateDB.GetCodeHash(addrCopy), evm.StateDB.GetCode(addrCopy))
		ret, err, CanParallel = evm.interpreter.Run(contract, input, false, TrueAccessList, IsParallel)
		gas = contract.Gas
	}
	if err != nil {
		evm.StateDB.RevertToSnapshot(snapshot)
	}
	return ret, gas, CanParallel, err
}

// StaticCall executes the contract associated with the addr with the given input
// as parameters while disallowing any modifications to the state during the call.
// Opcodes that attempt to perform such modifications will result in exceptions
// instead of performing the modifications.
func (evm *EVM) StaticCall(caller ContractRef, addr common.Address, input []byte, gas uint64, TrueAccessList *types.AccessList, IsParallel bool) (ret []byte, GasRemain uint64, CanParallel bool, err error) {
	CanParallel = true
	// Fail if we're trying to execute above the call depth limit
	if evm.depth > int(evmparams.CallCreateDepth) {
		return nil, gas, true, ErrDepth
	}

	var snapshot = evm.StateDB.Snapshot()

	evm.StateDB.AddBalance(addr, big0)

	// Invoke tracer hooks that signal entering/exiting a call frame
	if evm.Config.Tracer != nil {
		evm.Config.Tracer.CaptureEnter(STATICCALL, caller.Address(), addr, input, gas, nil)
		defer func(startGas uint64) {
			evm.Config.Tracer.CaptureExit(ret, startGas-gas, err)
		}(gas)
	}

	if p, isPrecompile := evm.precompile(addr); isPrecompile {
		ret, gas, err = RunPrecompiledContract(p, input, gas)
	} else {
		// At this point, we use a copy of address. If we don't, the go compiler will
		// leak the 'contract' to the outer scope, and make allocation for 'contract'
		// even if the actual execution ends on RunPrecompiled above.
		addrCopy := addr
		// Initialise a new contract and set the code that is to be used by the EVM.
		// The contract is a scoped environment for this execution context only.
		contract := NewContract(caller, AccountRef(addrCopy), new(big.Int), gas)
		contract.SetCallCode(&addrCopy, evm.StateDB.GetCodeHash(addrCopy), evm.StateDB.GetCode(addrCopy))
		// When an error was returned by the EVM or when setting the creation code
		// above we revert to the snapshot and consume any gas remaining. Additionally
		// when we're in Homestead this also counts for code storage gas errors.
		ret, err, CanParallel = evm.interpreter.Run(contract, input, true, TrueAccessList, IsParallel)
		gas = contract.Gas
	}
	if err != nil {
		evm.StateDB.RevertToSnapshot(snapshot)
	}
	return ret, gas, CanParallel, err
}

type codeAndHash struct {
	code []byte
	hash common.Hash
}

func (c *codeAndHash) Hash() common.Hash {
	if c.hash == (common.Hash{}) {
		c.hash = crypto.Keccak256Hash(c.code)
	}
	return c.hash
}

// create creates a new contract using code as deployment code.
func (evm *EVM) create(caller ContractRef, codeAndHash *codeAndHash, gas uint64, value *big.Int, address common.Address, TrueAccessList *types.AccessList, IsParallel bool) ([]byte, uint64, bool, error) {
	// 检查调用深度
	if evm.depth > int(evmparams.CallCreateDepth) {
		fmt.Printf("%sERROR MSG%s   调用深度出错\n", types.FRED, types.FRESET)
		return nil, gas, true, errors.New("max call depth exceeded") // CanParallel必须为true，为了不进入上层串行队列交易判断
	}
	// 是否有足够的钱转账
	if !evm.Context.CanTransfer(evm.StateDB, caller.Address(), value) {
		fmt.Printf("%sERROR MSG%s   没有足够的钱转账\n", types.FRED, types.FRESET)
		return nil, gas, true, errors.New("insufficient balance for transfer")
	}

	// 检查nonce是否正确
	nonce := evm.StateDB.GetNonce(caller.Address())
	if nonce+1 < nonce {
		fmt.Printf("%sERROR MSG%s   nonce值错误\n", types.FRED, types.FRESET)
		return nil, gas, true, errors.New("nonce uint64 overflow")
	}
	evm.StateDB.SetNonce(caller.Address(), nonce+1)

	// 确保在指定的地址没有现有的合约
	contractHash := evm.StateDB.GetCodeHash(address)
	if evm.StateDB.GetNonce(address) != 0 || (contractHash != (common.Hash{}) && contractHash != types.EmptyCodeHash) {
		fmt.Printf("%sERROR MSG%s   合约地址错误\n", types.FRED, types.FRESET)
		return nil, gas, true, errors.New("contract address collision")
	}

	snapshot := evm.StateDB.Snapshot()

	// 新建账户
	evm.StateDB.CreateAccount(address)

	// 转账交易
	evm.Context.Transfer(evm.StateDB, caller.Address(), address, value)

	// 新建合约
	contract := NewContract(caller, AccountRef(address), value, gas)
	contract.SetCodeOptionalHash(&address, codeAndHash)

	// 执行合约代码
	ret, err, CanParallel := evm.interpreter.Run(contract, nil, false, TrueAccessList, IsParallel)

	// 检查代码大小是否超出最大值
	if err == nil && len(ret) > evmparams.MaxCodeSize { // TODO: remove "evm.chainRules.IsEIP158"
		fmt.Printf("%sERROR MSG%s   合约代码大小错误\n", types.FRED, types.FRESET)
		return nil, 0, true, errors.New("max code size exceeded")
	}

	// 如果合约创建成功并且没有返回错误，计算存储代码所需的gas
	if err == nil {
		createDataGas := uint64(len(ret)) * evmparams.CreateDataGas
		if contract.UseGas(createDataGas) {
			evm.StateDB.SetCode(address, ret)
		} else {
			fmt.Printf("%sERROR MSG%s   汽油费无法支付存放合约代码的费用\n", types.FRED, types.FRESET)
			return nil, 0, true, errors.New("contract creation code storage out of gas")
		}
	}

	// 交易出错，快照回滚
	if err != nil { // TODO: remove "evm.chainRules.IsHomestead"
		evm.StateDB.RevertToSnapshot(snapshot)

	}

	return ret, contract.Gas, CanParallel, err
}

// Create creates a new contract using code as deployment code.
func (evm *EVM) Create(caller ContractRef, msg *core.TxMessage, gas uint64, TrueAccessList *types.AccessList,
	IsParallel bool) (ret []byte, GasRemain uint64, CanParallel bool, err error) {
	contractAddr := crypto.CreateAddress(caller.Address(), evm.StateDB.GetNonce(caller.Address()))
	return evm.create(caller, &codeAndHash{code: msg.Data}, gas, msg.Value, contractAddr, TrueAccessList, IsParallel)
}

// Create2 creates a new contract using code as deployment code.
//
// The different between Create2 with Create is Create2 uses keccak256(0xff ++ msg.sender ++ salt ++ keccak256(init_code))[12:]
// instead of the usual sender-and-nonce-hash as the address where the contract is initialized at.
func (evm *EVM) Create2(caller ContractRef, code []byte, gas uint64, endowment *big.Int, salt *uint256.Int, TrueAccessList *types.AccessList,
	IsParallel bool) (ret []byte, GasRemain uint64, CanParallel bool, err error) {
	codeAndHash := &codeAndHash{code: code}
	contractAddr := crypto.CreateAddress2(caller.Address(), salt.Bytes32(), codeAndHash.Hash().Bytes())
	return evm.create(caller, codeAndHash, gas, endowment, contractAddr, TrueAccessList, IsParallel)
}

// ChainConfig returns the environment's chain configuration
func (evm *EVM) ChainConfig() *evmparams.ChainConfig { return evm.chainConfig }
