package tracer

import (
	"math/big"

	"github.com/SipengXie/pangu/accesslist"
	"github.com/SipengXie/pangu/common"
	"github.com/SipengXie/pangu/core"
	"github.com/SipengXie/pangu/core/evm"
	"github.com/SipengXie/pangu/core/evm/params"
	"github.com/SipengXie/pangu/core/state"
	"github.com/SipengXie/pangu/core/types"
	"github.com/SipengXie/pangu/crypto"
)

func createRWAL(db *state.StateDB, args TransactionArgs, header *types.Header, bc *core.Blockchain) *accesslist.RW_AccessLists {
	from := args.from()
	to := args.to()
	isCreate := false
	if to == (common.Address{}) {
		hash := crypto.Keccak256Hash(args.data()).Bytes()
		to = crypto.CreateAddress2(from, args.salt().Bytes32(), hash)
		isCreate = true
	}
	precompiles := evm.ActivePrecompiles(params.TestChainConfig.Rules(big.NewInt(0), 0)) // 非常不严谨的chainconfig
	prevTracer := NewRWAccessListTracer(nil, precompiles)
	if isCreate {
		prevTracer.list.AddReadAL(from, BALANCE)
		prevTracer.list.AddWriteAL(from, BALANCE)
		prevTracer.list.AddReadAL(from, NONCE)
		prevTracer.list.AddWriteAL(from, NONCE)

		prevTracer.list.AddWriteAL(to, BALANCE)
		prevTracer.list.AddWriteAL(to, CODEHASH)
		prevTracer.list.AddWriteAL(to, CODE)
		prevTracer.list.AddWriteAL(to, NONCE)
		prevTracer.list.AddWriteAL(to, ALIVE)
		// Read to check if the contract to is already occupied
		prevTracer.list.AddReadAL(to, NONCE)
		prevTracer.list.AddReadAL(to, CODEHASH)
	} else {
		prevTracer.list.AddReadAL(from, BALANCE)
		prevTracer.list.AddWriteAL(from, BALANCE)
		prevTracer.list.AddReadAL(from, NONCE)
		prevTracer.list.AddWriteAL(from, NONCE)

		prevTracer.list.AddReadAL(to, CODE)
		prevTracer.list.AddReadAL(to, CODEHASH)
		prevTracer.list.AddReadAL(to, BALANCE)
		prevTracer.list.AddWriteAL(to, BALANCE)
	}
	prevTracer.list.Merge(*args.RWAccessList())

	for {
		RWAL := prevTracer.RWAccessList()
		statedb := db.Copy()

		args.AccessList = RWAL.ToJSON()
		msg, err := args.ToMessage(1000000000000000, header.BaseFee) // 没有设置globalGasCap
		if err != nil {
			panic(err) // TODO: handle error
		}

		tracer := NewRWAccessListTracer(RWAL, precompiles)
		config := evm.Config{Tracer: tracer, NoBaseFee: true}
		txCtx := core.NewEVMTxContext(msg)
		blkCtx := core.NewEVMBlockContext(header, bc, nil)
		vm := evm.NewEVM(blkCtx, txCtx, statedb, new(params.ChainConfig).FromGlobal(bc.Config()), config)
		res := core.ApplyTransaction(msg, vm)
		if res.Err != nil {
			panic(err) // TODO: handle error
		}
		if tracer.list.Equal(*prevTracer.list) {
			return tracer.list
		}
	}
}
