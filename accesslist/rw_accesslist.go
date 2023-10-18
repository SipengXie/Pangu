package accesslist

import (
	"crypto/sha256"

	"github.com/SipengXie/pangu/common"
)

var (
	CODE     = sha256.Sum256([]byte("code"))
	CODEHASH = sha256.Sum256([]byte("codeHash"))
	BALANCE  = sha256.Sum256([]byte("balance"))
	NONCE    = sha256.Sum256([]byte("nonce"))
	ALIVE    = sha256.Sum256([]byte("alive"))
)

type byte52 [52]byte

type ALTuple map[byte52]struct{}

func Combine(addr common.Address, hash common.Hash) byte52 {
	var key byte52
	copy(key[:], addr[:])
	copy(key[20:], hash[:])
	return key
}

func (tuple ALTuple) Add(addr common.Address, hash common.Hash) {
	key := Combine(addr, hash)
	if _, ok := tuple[key]; ok {
		return
	}
	tuple[key] = struct{}{}
}

func (tuple ALTuple) Contains(key byte52) bool {
	_, ok := tuple[key]
	return ok
}

type RW_AccessLists struct {
	ReadAL  ALTuple
	WriteAL ALTuple
}

func NewRWAccessLists() RW_AccessLists {
	return RW_AccessLists{
		ReadAL:  make(ALTuple),
		WriteAL: make(ALTuple),
	}
}

func (RWAL RW_AccessLists) AddReadAL(addr common.Address, hash common.Hash) {
	RWAL.ReadAL.Add(addr, hash)
}

func (RWAL RW_AccessLists) AddWriteAL(addr common.Address, hash common.Hash) {
	RWAL.WriteAL.Add(addr, hash)
}

func (RWAL RW_AccessLists) HasConflict(other RW_AccessLists) bool {
	for key := range RWAL.ReadAL {
		if other.WriteAL.Contains(key) {
			return true
		}
	}
	for key := range RWAL.WriteAL {
		if other.WriteAL.Contains(key) {
			return true
		}
		if other.ReadAL.Contains(key) {
			return true
		}
	}
	return false
}

func (RWAL RW_AccessLists) ToMarshal() RW_AccessLists_Marshal {
	readSet := make(map[common.Address][]string)
	writeSet := make(map[common.Address][]string)
	for key := range RWAL.ReadAL {
		addr := common.BytesToAddress(key[:20])
		hash := common.BytesToHash(key[20:])
		readSet[addr] = append(readSet[addr], decodeHash(hash))
	}
	for key := range RWAL.WriteAL {
		addr := common.BytesToAddress(key[:20])
		hash := common.BytesToHash(key[20:])
		writeSet[addr] = append(writeSet[addr], decodeHash(hash))
	}
	return RW_AccessLists_Marshal{
		ReadSet:  readSet,
		WriteSet: writeSet,
	}
}

func decodeHash(hash common.Hash) string {
	switch hash {
	case CODE:
		return "code"
	case BALANCE:
		return "balance"
	case ALIVE:
		return "alive"
	case CODEHASH:
		return "codeHash"
	case NONCE:
		return "nonce"
	default:
		return hash.Hex()
	}
}

type RW_AccessLists_Marshal struct {
	ReadSet  map[common.Address][]string `json:"readSet"`
	WriteSet map[common.Address][]string `json:"writeSet"`
}
