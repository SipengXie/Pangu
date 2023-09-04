// Copyright 2016 The go-ethereum Authors
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

package types

import (
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"

	"github.com/SipengXie/pangu/common"
	"github.com/SipengXie/pangu/crypto"
	"github.com/SipengXie/pangu/params"
)

const (
	SIG_ECDSA = 0x00
)

var (
	ErrInvalidChainId = errors.New("invalid chain id for signer")
	ErrInvalidSigAlgo = errors.New("invalid signature algorithm")
)

type ECDSA_Sig struct{}

func (E *ECDSA_Sig) Sign(hash []byte, prv *ecdsa.PrivateKey) ([]byte, error) {
	return crypto.Sign(hash, prv)
}

func (E *ECDSA_Sig) DecodeSignature(sig []byte) (r, s, v *big.Int) {
	if len(sig) != crypto.SignatureLength {
		panic(fmt.Sprintf("wrong size for signature: got %d, want %d", len(sig), crypto.SignatureLength))
	}
	r = new(big.Int).SetBytes(sig[:32])
	s = new(big.Int).SetBytes(sig[32:64])
	v = new(big.Int).SetBytes([]byte{sig[64] + 27})
	return r, s, v
}

func (E *ECDSA_Sig) RecoverPlain(sighash common.Hash, R, S, Vb *big.Int, homestead bool) (common.Address, error) {
	if Vb.BitLen() > 8 {
		return common.Address{}, ErrInvalidSig
	}
	V := byte(Vb.Uint64() - 27)
	if !crypto.ValidateSignatureValues(V, R, S, homestead) {
		return common.Address{}, ErrInvalidSig
	}
	// encode the signature in uncompressed format
	r, s := R.Bytes(), S.Bytes()
	sig := make([]byte, crypto.SignatureLength)
	copy(sig[32-len(r):32], r)
	copy(sig[64-len(s):64], s)
	sig[64] = V
	// recover the public key from the signature
	pub, err := crypto.Ecrecover(sighash[:], sig)
	if err != nil {
		return common.Address{}, err
	}
	if len(pub) == 0 || pub[0] != 4 {
		return common.Address{}, errors.New("invalid public key")
	}
	var addr common.Address
	copy(addr[:], crypto.Keccak256(pub[1:])[12:])
	return addr, nil
}

// deriveChainId derives the chain id from the given v parameter
func (E *ECDSA_Sig) DeriveChainId(v *big.Int) *big.Int {
	if v.BitLen() <= 64 {
		v := v.Uint64()
		if v == 27 || v == 28 {
			return new(big.Int)
		}
		return new(big.Int).SetUint64((v - 35) / 2)
	}
	v = new(big.Int).Sub(v, big.NewInt(35))
	return v.Div(v, big.NewInt(2))
}

// sigCache is used to cache the derived sender and contains
// the signer used to derive it.
type sigCache struct {
	signer Signer
	from   common.Address
}

// TODO:后面再说
// MakeSigner returns a Signer based on the given chain config and block number.
func MakeSigner(config *params.ChainConfig, blockNumber *big.Int, blockTime uint64) Signer {
	return NewPanguSigner(config.ChainID)
}

// LatestSignerForChainID returns the 'most permissive' Signer available. Specifically,
// this enables support for EIP-155 replay protection and all implemented EIP-2718
// transaction types if chainID is non-nil.
//
// Use this in transaction-handling code where the current block number and fork
// configuration are unknown. If you have a ChainConfig, use LatestSigner instead.
// If you have a ChainConfig and know the current block number, use MakeSigner instead.
func LatestSignerForChainID(chainID *big.Int) Signer {
	return NewPanguSigner(chainID)
}

// SignTx signs the transaction using the given signer and private key.
func SignTx(tx *Transaction, s Signer, prv []byte, algo byte) (*Transaction, error) {
	h := s.Hash(tx)
	switch algo {
	case SIG_ECDSA:
		{
			pri_k, err := crypto.ToECDSA(prv)
			if err != nil {
				return nil, err
			}
			sig, err := crypto.Sign(h[:], pri_k)
			if err != nil {
				return nil, err
			}
			return tx.WithSignature(s, sig, algo)
		}
	default:
		{
			return nil, ErrInvalidSigAlgo
		}
	}
}

// SignNewTx creates a transaction and signs it.
func SignNewTx(txdata TxData, s Signer, prv []byte, algo byte) (*Transaction, error) {
	tx := NewTx(txdata)
	h := s.Hash(tx)

	switch algo {
	case SIG_ECDSA:
		{
			pri_k, err := crypto.ToECDSA(prv)
			if err != nil {
				return nil, err
			}
			sig, err := crypto.Sign(h[:], pri_k)
			if err != nil {
				return nil, err
			}
			return tx.WithSignature(s, sig, algo)
		}
	default:
		{
			return nil, ErrInvalidSigAlgo
		}
	}
}

// MustSignNewTx creates a transaction and signs it.
// This panics if the transaction cannot be signed.
func MustSignNewTx(txdata TxData, s Signer, prv []byte, algo byte) *Transaction {
	tx, err := SignNewTx(txdata, s, prv, algo)
	if err != nil {
		panic(err)
	}
	return tx
}

// Sender returns the address derived from the signature (V, R, S) using secp256k1
// elliptic curve and an error if it failed deriving or upon an incorrect
// signature.
//
// Sender may cache the address, allowing it to be used regardless of
// signing method. The cache is invalidated if the cached signer does
// not match the signer used in the current call.
func Sender(signer Signer, tx *Transaction) (common.Address, error) {
	if sc := tx.from.Load(); sc != nil {
		sigCache := sc.(sigCache)
		// If the signer used to derive from in a previous
		// call is not the same as used current, invalidate
		// the cache.
		if sigCache.signer.Equal(signer) {
			return sigCache.from, nil
		}
	}

	addr, err := signer.Sender(tx)
	if err != nil {
		return common.Address{}, err
	}
	tx.from.Store(sigCache{signer: signer, from: addr})
	return addr, nil
}

// Signer encapsulates transaction signature handling. The name of this type is slightly
// misleading because Signers don't actually sign, they're just for validating and
// processing of signatures.
//
// Note that this interface is not a stable API and may change at any time to accommodate
// new protocol rules.
type Signer interface {
	// Sender returns the sender address of the transaction.
	Sender(tx *Transaction) (common.Address, error)

	// // SignatureValues returns the raw R, S, V values corresponding to the
	// // given signature.
	// SignatureValues(tx *Transaction, sig []byte) (r, s, v *big.Int, err error)
	ChainID() *big.Int

	// Hash returns 'signature hash', i.e. the transaction hash that is signed by the
	// private key. This hash does not uniquely identify the transaction.
	Hash(tx *Transaction) common.Hash

	// Equal returns true if the given signer is the same as the receiver.
	Equal(Signer) bool
}

type panguSigner struct {
	chainId, chainIdMul *big.Int
	ECDSA_Algo          ECDSA_Sig
}

func NewPanguSigner(chainId *big.Int) Signer {
	if chainId == nil {
		chainId = new(big.Int)
	}
	return panguSigner{
		chainId:    chainId,
		chainIdMul: new(big.Int).Mul(chainId, big.NewInt(2)),
		ECDSA_Algo: ECDSA_Sig{},
	}
}

func (s panguSigner) ChainID() *big.Int {
	return s.chainId
}

func (s panguSigner) Sender(tx *Transaction) (common.Address, error) {
	if tx.Type() != PanguTxType {
		return common.Address{}, ErrTxTypeNotSupported
	}
	sigAlgo := tx.SigAlgo()
	switch sigAlgo {
	case SIG_ECDSA:
		{
			V, R, S := s.ECDSA_Algo.DecodeSignature(tx.RawSigValues())
			if tx.ChainId().Cmp(s.chainId) != 0 {
				return common.Address{}, fmt.Errorf("%w: have %d want %d", ErrInvalidChainId, tx.ChainId(), s.chainId)
			}
			return s.ECDSA_Algo.RecoverPlain(s.Hash(tx), R, S, V, true)
		}
	default:
		return common.Address{}, ErrInvalidSigAlgo
	}

}

func (s panguSigner) Equal(o Signer) bool {
	x, ok := o.(panguSigner)
	return ok && x.chainId.Cmp(s.chainId) == 0
}

// Hash returns the hash to be signed by the sender.
// It does not uniquely identify the transaction.
func (s panguSigner) Hash(tx *Transaction) common.Hash {
	return prefixedRlpHash(
		tx.Type(),
		[]interface{}{
			s.chainId,
			tx.Nonce(),
			tx.GasTipCap(),
			tx.GasFeeCap(),
			tx.GasLimit(),
			tx.To(),
			tx.Value(),
			tx.EncContent(),
		})
}
