package tracer

import (
	"crypto/sha256"
	"math/big"

	"github.com/SipengXie/pangu/accesslist"
	"github.com/SipengXie/pangu/common"
	"github.com/SipengXie/pangu/core/evm"
	"github.com/SipengXie/pangu/crypto"
	"github.com/ethereum/go-ethereum/core/vm"
)

var (
	CODE     = sha256.Sum256([]byte("code"))
	CODEHASH = sha256.Sum256([]byte("codeHash"))
	BALANCE  = sha256.Sum256([]byte("balance"))
	NONCE    = sha256.Sum256([]byte("nonce"))
	ALIVE    = sha256.Sum256([]byte("alive"))
)

// Tracer mainly records the accesslist of each transaction during evm execution (interpreter.run)
type RW_AccessListsTracer struct {
	excl map[common.Address]struct{} // only excludes those stateless precompile contracts
	list accesslist.RW_AccessLists
}

func NewRWAccessListTracer(rwAL accesslist.RW_AccessLists, precompiles []common.Address) *RW_AccessListsTracer {
	excl := make(map[common.Address]struct{})
	for _, addr := range precompiles {
		excl[addr] = struct{}{}
	}
	rwList := accesslist.NewRWAccessLists()
	for key := range rwAL.ReadAL {
		addr := common.BytesToAddress(key[:20])
		if _, ok := excl[addr]; !ok {
			rwList.ReadAL.Add(addr, common.BytesToHash(key[20:]))
		}
	}
	for key := range rwAL.WriteAL {
		addr := common.BytesToAddress(key[:20])
		if _, ok := excl[addr]; !ok {
			rwList.WriteAL.Add(addr, common.BytesToHash(key[20:]))
		}
	}
	return &RW_AccessListsTracer{
		excl: excl,
		list: rwList,
	}
}

func (a *RW_AccessListsTracer) CaptureStart(env *evm.EVM, from common.Address, to common.Address, create bool, input []byte, gas uint64, value *big.Int) {
}

// CaptureState captures all opcodes that touch storage or addresses and adds them to the accesslist.
func (a *RW_AccessListsTracer) CaptureState(pc uint64, op evm.OpCode, gas, cost uint64, scope *evm.ScopeContext, rData []byte, depth int, err error) {
	stack := scope.Stack
	stackData := stack.Data()
	stackLen := len(stackData)
	switch op {
	case evm.SLOAD:
		{
			if stackLen >= 1 {
				slot := common.Hash(stackData[stackLen-1].Bytes32())
				a.list.AddReadAL(scope.Contract.Address(), slot)
			}
		}
	case evm.SSTORE:
		{
			if stackLen >= 1 {
				slot := common.Hash(stackData[stackLen-1].Bytes32())
				a.list.AddWriteAL(scope.Contract.Address(), slot)
			}
		}
	case evm.EXTCODECOPY: // read code
		{
			if stackLen >= 1 {
				addr := common.Address(stackData[stackLen-1].Bytes20())
				if _, ok := a.excl[addr]; !ok {
					a.list.AddReadAL(addr, CODE)
				}
			}
		}
	case evm.EXTCODEHASH:
		{
			if stackLen >= 1 {
				addr := common.Address(stackData[stackLen-1].Bytes20())
				if _, ok := a.excl[addr]; !ok {
					a.list.AddReadAL(addr, CODEHASH)
				}
			}
		}
	case evm.EXTCODESIZE:
		{
			if stackLen >= 1 {
				addr := common.Address(stackData[stackLen-1].Bytes20())
				if _, ok := a.excl[addr]; !ok {
					a.list.AddReadAL(addr, CODE)
				}
			}
		}
	case evm.BALANCE:
		{
			if stackLen >= 1 {
				addr := common.Address(stackData[stackLen-1].Bytes20())
				if _, ok := a.excl[addr]; !ok {
					a.list.AddReadAL(addr, BALANCE)
				}
			}
		}
	case evm.SELFDESTRUCT:
		{
			if stackLen >= 1 {
				beneficiary := common.Address(stackData[stackLen-1].Bytes20())
				if _, ok := a.excl[beneficiary]; !ok {
					a.list.AddReadAL(beneficiary, BALANCE)
					a.list.AddWriteAL(beneficiary, BALANCE)
				}
				addr := scope.Contract.Address()
				if _, ok := a.excl[addr]; !ok {
					a.list.AddWriteAL(addr, BALANCE)
					a.list.AddWriteAL(addr, ALIVE)
				}
			}
		}
	case evm.CALL:
		{
			if stackLen >= 5 {
				from := scope.Contract.Address()
				to := common.Address(stackData[stackLen-2].Bytes20())
				if _, ok := a.excl[from]; !ok {
					a.list.AddReadAL(from, BALANCE)
					a.list.AddWriteAL(from, BALANCE)
					a.list.AddReadAL(from, NONCE)
					a.list.AddWriteAL(from, NONCE)
				}
				if _, ok := a.excl[to]; !ok {
					a.list.AddReadAL(to, CODE)
					a.list.AddReadAL(to, CODEHASH)
					a.list.AddReadAL(to, BALANCE)
					a.list.AddWriteAL(to, BALANCE)
				}
			}
		}
	case evm.STATICCALL, evm.DELEGATECALL, evm.CALLCODE:
		{
			if stackLen >= 5 {
				to := common.Address(stackData[stackLen-2].Bytes20())
				if _, ok := a.excl[to]; !ok {
					a.list.AddReadAL(to, CODE)
					a.list.AddReadAL(to, CODEHASH)
				}
			}
		}
	case evm.CREATE2: // cannot apply to CREATE, because the addr is dependent on the nonce
		{
			if stackLen >= 4 {
				from := scope.Contract.Address()
				if _, ok := a.excl[from]; !ok {
					a.list.AddReadAL(from, BALANCE)
					a.list.AddWriteAL(from, BALANCE)
					a.list.AddReadAL(from, NONCE)
					a.list.AddWriteAL(from, NONCE)
				}

				offset, size := stackData[stackLen-2].Uint64(), stackData[stackLen-3].Uint64()
				salt := stackData[stackLen-4].Bytes32()
				input := scope.Memory.GetCopy(int64(offset), int64(size))
				codeHash := crypto.Keccak256Hash(input)
				addr := crypto.CreateAddress2(scope.Contract.Address(), salt, codeHash.Bytes())
				if _, ok := a.excl[addr]; !ok {
					a.list.AddWriteAL(addr, BALANCE)
					a.list.AddWriteAL(addr, CODEHASH)
					a.list.AddWriteAL(addr, CODE)
					a.list.AddWriteAL(addr, NONCE)
					a.list.AddWriteAL(addr, ALIVE)
					// Read to check if the contract addr is already occupied
					a.list.AddReadAL(addr, NONCE)
					a.list.AddReadAL(addr, CODEHASH)
				}
			}
		}
	}
}

func (*RW_AccessListsTracer) CaptureFault(pc uint64, op vm.OpCode, gas, cost uint64, scope *vm.ScopeContext, depth int, err error) {
}

func (*RW_AccessListsTracer) CaptureEnd(output []byte, gasUsed uint64, err error) {}

func (*RW_AccessListsTracer) CaptureEnter(typ vm.OpCode, from common.Address, to common.Address, input []byte, gas uint64, value *big.Int) {
}

func (*RW_AccessListsTracer) CaptureExit(output []byte, gasUsed uint64, err error) {}

func (*RW_AccessListsTracer) CaptureTxStart(gasLimit uint64) {}

func (*RW_AccessListsTracer) CaptureTxEnd(restGas uint64) {}

// AccessList returns the current accesslist maintained by the tracer.
func (a *RW_AccessListsTracer) RWAccessList() accesslist.RW_AccessLists {
	return a.list
}
