package executor

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/SipengXie/pangu/accesslist"
	"github.com/SipengXie/pangu/common"
	"github.com/SipengXie/pangu/core/rawdb"
	"github.com/SipengXie/pangu/core/state"
	"github.com/SipengXie/pangu/core/txpool/legacypool"
	"github.com/SipengXie/pangu/core/types"
	"github.com/SipengXie/pangu/crypto"
	"github.com/SipengXie/pangu/event"
	"github.com/SipengXie/pangu/params"
	"github.com/SipengXie/pangu/trie"

	"math/big"
	"sync/atomic"
)

const (
	// testGas is the gas required for contract deployment.
	testGas   = 1441090
	testTxGas = 210000
)

var (
	Address          = "127.0.0.1:9876"
	testTxPoolConfig legacypool.Config
	db               = rawdb.NewMemoryDatabase()
	eip1559Config    *params.ChainConfig

	// Test accounts
	bankKeyHex  = "c3914129fade8d775d22202702690a8a0dcb178040bcb232a950c65b84308828"
	bankKeyByes = common.Hex2Bytes(bankKeyHex)
	address111  = "0x055504FE4d542fE266C7215a9cc2aa22E6a78445"
)

func init() {
	testTxPoolConfig = legacypool.DefaultConfig
	testTxPoolConfig.Journal = ""
	cpy := *params.TestChainConfig
	eip1559Config = &cpy
	eip1559Config.ChainID = big.NewInt(1337)
}

type testBlockChain struct {
	config        *params.ChainConfig
	gasLimit      atomic.Uint64
	statedb       *state.StateDB
	chainHeadFeed *event.Feed
}

func newTestBlockChain(config *params.ChainConfig, gasLimit uint64, statedb *state.StateDB, chainHeadFeed *event.Feed) *testBlockChain {
	bc := testBlockChain{config: config, statedb: statedb, chainHeadFeed: new(event.Feed)}
	bc.gasLimit.Store(gasLimit)
	return &bc
}

func (bc *testBlockChain) Config() *params.ChainConfig {
	return bc.config
}

func (bc *testBlockChain) CurrentBlock() *types.Header {
	return &types.Header{
		Number:   new(big.Int),
		GasLimit: bc.gasLimit.Load(),
	}
}

func (bc *testBlockChain) GetBlock(hash common.Hash, number uint64) *types.Block {
	return types.NewBlock(bc.CurrentBlock(), nil, nil, types.EmptyRootHash, trie.NewStackTrie(nil))
}

func (bc *testBlockChain) StateAt(common.Hash) (*state.StateDB, error) {
	return bc.statedb, nil
}

func (bc *testBlockChain) SubscribeChainHeadEvent(ch chan<- types.ChainHeadEvent) event.Subscription {
	return bc.chainHeadFeed.Subscribe(ch)
}

func panguTx(nonce uint64, to common.Address, amount *big.Int, gasLimit uint64, data []byte, gasFee *big.Int, tip *big.Int, key []byte, from common.Address) *types.Transaction {
	res := bytes.Compare(to.Bytes(), types.EmptyAddress)
	if res == 0 { // 合约创建交易
		al := &accesslist.AccessList{
			Addresses: make(map[common.Address]int),
			Slots:     make([]map[common.Hash]struct{}, 0),
		}
		al.Addresses[from] = -1
		tx, _ := types.SignNewTx(&types.PanguTransaction{
			To:         nil,
			Nonce:      nonce,
			Value:      amount,
			GasLimit:   gasLimit,
			TipCap:     tip,
			FeeCap:     gasFee,
			ChainID:    big.NewInt(1337),
			SigAlgo:    byte(0x00),
			Data:       data,
			AccessList: al,
		}, types.LatestSignerForChainID(big.NewInt(1337)), key, byte(0x00))
		return tx
	} else {
		al := &accesslist.AccessList{
			Addresses: make(map[common.Address]int),
			Slots:     make([]map[common.Hash]struct{}, 0),
		}
		al.Addresses[from] = -1
		al.Addresses[to] = -1
		// var result = "{\n"
		// result += "\tFrom: " + from.Hex() + ",\n"
		// nonce_str := strconv.Itoa(int(nonce))
		// result += "\tNonce: " + nonce_str + ",\n"
		// result += "\tTo: " + to.Hex() + ",\n"
		// result += "}\n"
		// fmt.Println(result)
		tx, _ := types.SignNewTx(&types.PanguTransaction{
			To:         &to,
			Nonce:      nonce,
			Value:      amount,
			GasLimit:   gasLimit,
			TipCap:     tip,
			FeeCap:     gasFee,
			ChainID:    big.NewInt(1337),
			SigAlgo:    byte(0x00),
			Data:       data,
			AccessList: al,
		}, types.LatestSignerForChainID(big.NewInt(1337)), key, byte(0x00))
		return tx
	}
}

// 构造3比交易，A -> B, B->C , D->E
func newPankutxs() []*types.Transaction {
	// 构造交易
	toAddr := common.BytesToAddress(common.FromHex(address111))
	fromKey, _ := crypto.ToECDSA(bankKeyByes)
	fromAddr := crypto.PubkeyToAddress(fromKey.PublicKey)
	tx := panguTx(0, toAddr, big.NewInt(100), testTxGas, nil, big.NewInt(100), big.NewInt(1), bankKeyByes, fromAddr)
	var txs types.Transactions
	txs = append(txs, tx)
	fmt.Println(toAddr)
	fmt.Println(fromAddr)
	fmt.Println(common.Bytes2Hex(tx.RawSigValues()))
	alByte, _ := tx.AccessList().Serialize()
	fmt.Println(common.Bytes2Hex(alByte))
	return txs
}

func TestCreateTx(t *testing.T) {
	txs := newPankutxs()
	fmt.Println(txs[0])
}
