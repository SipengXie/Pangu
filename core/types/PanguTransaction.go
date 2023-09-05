package types

import (
	"math/big"

	"github.com/SipengXie/pangu/common"
)

type PanguTransaction struct {
	To       *common.Address `rlp:"nil"`
	Nonce    uint64
	Value    *big.Int
	GasLimit uint64
	FeeCap   *big.Int
	TipCap   *big.Int
	ChainID  *big.Int

	SigAlgo   byte
	Signature []byte

	// TODO: 暂时先不用
	EncAlgo    byte
	EncContent []byte // EncContent <--> {Data, AccessList}
	VmType     byte

	Data       []byte
	AccessList *AccessList
}

// copy creates a deep copy of the transaction data and initializes all fields.
func (tx *PanguTransaction) copy() TxData {
	cpy := &PanguTransaction{
		Nonce:     tx.Nonce,
		To:        copyAddressPtr(tx.To),
		Data:      common.CopyBytes(tx.Data),
		GasLimit:  tx.GasLimit,
		Signature: common.CopyBytes(tx.Signature),

		// These are copied below.
		AccessList: new(AccessList),
		Value:      new(big.Int),
		ChainID:    new(big.Int),
		TipCap:     new(big.Int),
		FeeCap:     new(big.Int),
	}
	*cpy.AccessList = *tx.AccessList
	if tx.Value != nil {
		cpy.Value.Set(tx.Value)
	}
	if tx.ChainID != nil {
		cpy.ChainID.Set(tx.ChainID)
	}
	if tx.TipCap != nil {
		cpy.TipCap.Set(tx.TipCap)
	}
	if tx.FeeCap != nil {
		cpy.FeeCap.Set(tx.FeeCap)
	}

	return cpy
}

func (tx *PanguTransaction) txType() byte            { return PanguTxType }
func (tx *PanguTransaction) chainID() *big.Int       { return tx.ChainID }
func (tx *PanguTransaction) encContent() []byte      { return tx.EncContent }
func (tx *PanguTransaction) accessList() *AccessList { return tx.AccessList }
func (tx *PanguTransaction) data() []byte            { return tx.Data }
func (tx *PanguTransaction) gasLimit() uint64        { return tx.GasLimit }
func (tx *PanguTransaction) gasFeeCap() *big.Int     { return tx.FeeCap }
func (tx *PanguTransaction) gasTipCap() *big.Int     { return tx.TipCap }
func (tx *PanguTransaction) gasPrice() *big.Int      { return tx.FeeCap }
func (tx *PanguTransaction) value() *big.Int         { return tx.Value }
func (tx *PanguTransaction) nonce() uint64           { return tx.Nonce }
func (tx *PanguTransaction) to() *common.Address     { return tx.To }
func (tx *PanguTransaction) sigAlgo() byte           { return tx.SigAlgo }

func (tx *PanguTransaction) rawSigValues() []byte {
	return tx.Signature
}

func (tx *PanguTransaction) setSigValues(chainID *big.Int, sig []byte, sigAlgo byte) {
	tx.ChainID, tx.SigAlgo, tx.Signature = chainID, sigAlgo, sig
}

func (tx *PanguTransaction) effectiveGasPrice(dst *big.Int, baseFee *big.Int) *big.Int {
	if baseFee == nil {
		return dst.Set(tx.FeeCap)
	}
	tip := dst.Sub(tx.FeeCap, baseFee)
	if tip.Cmp(tx.TipCap) > 0 {
		tip.Set(tx.TipCap)
	}
	return tip.Add(tip, baseFee)
}
