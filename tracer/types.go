package tracer

import (
	"encoding/json"
	"errors"
	"math/big"

	"github.com/SipengXie/pangu/accesslist"
	"github.com/SipengXie/pangu/common"
	"github.com/SipengXie/pangu/common/math"
	"github.com/SipengXie/pangu/core"
	"github.com/SipengXie/pangu/log"
	"github.com/holiman/uint256"
)

type TransactionArgs struct {
	From                 string `json:"from"`
	To                   string `json:"to"`
	Gas                  uint64 `json:"gas"`
	GasPrice             string `json:"gasPrice"`
	MaxFeePerGas         string `json:"maxFeePerGas"`
	MaxPriorityFeePerGas string `json:"maxPriorityFeePerGas"`
	Value                string `json:"value"`
	Nonce                uint64 `json:"nonce"`
	Salt                 string `json:"salt"`
	SigAlgo              byte   `json:"sigAlgo"`
	Signature            string `json:"signature"`
	Data                 string `json:"data"`
	Input                string `json:"input"`
	AccessList           string `json:"accessList,omitemty"`
	ChainID              string `json:"chainId,omitempty"`
}

func (txargs TransactionArgs) from() common.Address {
	return common.HexToAddress(txargs.From)
}

func (txargs TransactionArgs) to() common.Address {
	if txargs.To == "" {
		return common.Address{}
	}
	return common.HexToAddress(txargs.To)
}

func (txargs TransactionArgs) gas() uint64 {
	return txargs.Gas
}

func (txargs TransactionArgs) gasPrice() *big.Int {
	res, _ := new(big.Int).SetString(txargs.GasPrice, 10)
	return res
}

func (txargs TransactionArgs) salt() *uint256.Int {
	res := new(uint256.Int)
	res.SetFromHex(txargs.Salt)
	return res
}

func (txargs TransactionArgs) data() []byte {
	return common.FromHex(txargs.Data)
}

func (txargs TransactionArgs) RWAccessList() *accesslist.RW_AccessLists {
	if txargs.AccessList == "" {
		return nil
	}
	RWbytes := common.FromHex(txargs.AccessList)
	RWAL_Marshal := accesslist.NewRWAccessListsMarshal()
	json.Unmarshal(RWbytes, &RWAL_Marshal)
	return RWAL_Marshal.ToRWAL()
}

func (args *TransactionArgs) ToMessage(globalGasCap uint64, baseFee *big.Int) (*core.TxMessage, error) {
	// Reject invalid combinations of pre- and post-1559 fee styles
	if args.GasPrice != "" && (args.MaxFeePerGas != "" || args.MaxPriorityFeePerGas != "") {
		return nil, errors.New("both gasPrice and (maxFeePerGas or maxPriorityFeePerGas) specified")
	}
	// Set sender address or use zero address if none specified.
	addr := args.from()
	to := args.to()

	// Set default gas & gas price if none were set
	gas := globalGasCap
	if gas == 0 {
		gas = uint64(math.MaxUint64 / 2)
	}
	gas = args.Gas
	if globalGasCap != 0 && globalGasCap < gas {
		log.Warn("Caller gas above allowance, capping", "requested", gas, "cap", globalGasCap)
		gas = globalGasCap
	}
	var (
		gasPrice  *big.Int
		gasFeeCap *big.Int
		gasTipCap *big.Int
	)
	if baseFee == nil {
		// If there's no basefee, then it must be a non-1559 execution
		gasPrice = new(big.Int)
		if args.GasPrice != "" {
			gasPrice, _ = gasPrice.SetString(args.GasPrice, 10)
		}
		gasFeeCap, gasTipCap = gasPrice, gasPrice
	} else {
		// A basefee is provided, necessitating 1559-type execution
		if args.GasPrice != "" {
			// User specified the legacy gas field, convert to 1559 gas typing
			gasPrice = new(big.Int)
			gasPrice, _ = gasPrice.SetString(args.GasPrice, 10)
			gasFeeCap, gasTipCap = gasPrice, gasPrice
		} else {
			// User specified 1559 gas fields (or none), use those
			gasFeeCap = new(big.Int)
			if args.MaxFeePerGas != "" {
				gasFeeCap, _ = gasFeeCap.SetString(args.MaxFeePerGas, 10)
			}
			gasTipCap = new(big.Int)
			if args.MaxPriorityFeePerGas != "" {
				gasTipCap, _ = gasFeeCap.SetString(args.MaxPriorityFeePerGas, 10)
			}
			// Backfill the legacy gasPrice for EVM execution, unless we're all zeroes
			gasPrice = new(big.Int)
			if gasFeeCap.BitLen() > 0 || gasTipCap.BitLen() > 0 {
				gasPrice = math.BigMin(new(big.Int).Add(gasTipCap, baseFee), gasFeeCap)
			}
		}
	}
	value := new(big.Int)
	if args.Value != "" {
		value, _ = value.SetString(args.Value, 10)
	}
	data := args.data()
	var RWAccessList *accesslist.RW_AccessLists = nil
	if args.AccessList != "" {
		RWAccessList = args.RWAccessList()
	}
	msg := &core.TxMessage{
		From:              addr,
		To:                &to,
		Value:             value,
		GasLimit:          gas,
		GasPrice:          gasPrice,
		GasFeeCap:         gasFeeCap,
		GasTipCap:         gasTipCap,
		Data:              data,
		RWAccessList:      RWAccessList,
		SkipAccountChecks: true,
	}
	return msg, nil
}
