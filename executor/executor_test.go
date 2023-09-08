package executor

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/SipengXie/pangu/accesslist"
	"github.com/SipengXie/pangu/common"
	"github.com/SipengXie/pangu/core"
	"github.com/SipengXie/pangu/core/evm"
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
	AKeyHex   = "c3914129fade8d775d22202702690a8a0dcb178040bcb232a950c65b84308828"
	AKeyBytes = common.Hex2Bytes(AKeyHex)

	BAddress = "0x055504FE4d542fE266C7215a9cc2aa22E6a78445"

	CKeyHex   = "c3914129fade8d775d22202702690a8a0dcb178040bcb232a950c65b84308830"
	CKeyBytes = common.Hex2Bytes(CKeyHex)
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
		data, _ := tx.MarshalBinary()
		newTx := new(types.Transaction)
		err := newTx.UnmarshalBinary(data)
		if err != nil {
			fmt.Printf("err = %v", err)
		}
		return tx
	} else {
		al := &accesslist.AccessList{
			Addresses: make(map[common.Address]int),
			Slots:     make([]map[common.Hash]struct{}, 0),
		}
		al.Addresses[from] = -1
		al.Addresses[to] = -1
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
		data, _ := tx.MarshalBinary()
		newTx := new(types.Transaction)
		err := newTx.UnmarshalBinary(data)
		if err != nil {
			fmt.Printf("err = %v", err)
		}
		return tx
	}
}

// 构造一笔交易，A -> B
func NewSingleTX() []types.Transactions {
	// 构造交易
	toAddr := common.BytesToAddress(common.FromHex(BAddress))
	fromKey, _ := crypto.ToECDSA(AKeyBytes)
	fromAddr := crypto.PubkeyToAddress(fromKey.PublicKey)
	tx := panguTx(0, toAddr, big.NewInt(100), testTxGas, nil, big.NewInt(100), big.NewInt(1), AKeyBytes, fromAddr)
	txs := make([]types.Transactions, 1)
	txs[0] = append(txs[0], tx)
	// 变成二维数组
	return txs
}

// 构造三笔交易，A -> B，A -> C，B -> C
func NewTripleTX() []types.Transactions {
	// A -> B
	BAddr := common.BytesToAddress(common.FromHex(BAddress))
	AKey, _ := crypto.ToECDSA(AKeyBytes)
	AAddr := crypto.PubkeyToAddress(AKey.PublicKey)
	tx1 := panguTx(0, BAddr, big.NewInt(100), testTxGas, nil, big.NewInt(100), big.NewInt(1), AKeyBytes, AAddr)

	// A -> C
	CKey, _ := crypto.ToECDSA(CKeyBytes)
	CAddr := crypto.PubkeyToAddress(CKey.PublicKey)
	tx2 := panguTx(1, CAddr, big.NewInt(100), testTxGas, nil, big.NewInt(100), big.NewInt(2), AKeyBytes, AAddr)

	// C -> B
	tx3 := panguTx(0, BAddr, big.NewInt(1), testTxGas, nil, big.NewInt(50), big.NewInt(1), CKeyBytes, CAddr)

	// 组装交易
	txs := make([]types.Transactions, 1)
	txs[0] = append(txs[0], tx1, tx2, tx3)
	// 变成二维数组
	return txs
}

// 测试单笔转账交易 串行
func TestSingleTransferTX(t *testing.T) {
	// 创建一笔交易
	txs := NewSingleTX()
	fmt.Printf("%sPROMPT MSG%s   创建了一笔新交易，该交易内容是 %v", types.FGREEN, types.FRESET, txs[0])

	// ToAddr := common.BytesToAddress(common.FromHex(BAddress)) // 目的地址
	AKey, _ := crypto.ToECDSA(AKeyBytes)
	AAddr := crypto.PubkeyToAddress(AKey.PublicKey) // 发送地址
	CKey, _ := crypto.ToECDSA(CKeyBytes)
	CAddr := crypto.PubkeyToAddress(CKey.PublicKey)

	statedb, _ := state.New(types.EmptyRootHash, state.NewDatabase(db), nil)
	statedb.SetBalance(AAddr, big.NewInt(99999999999999999)) // 发送地址余额增加
	statedb.SetBalance(CAddr, big.NewInt(99999999999999999)) // 发送地址余额增加

	// 创建一条完整链作为父链
	// 模拟一个区块链
	chainCfg := &params.ChainConfig{
		ChainID: big.NewInt(1337),
	}
	// 起链
	blockchain := core.NewBlokchain(chainCfg, statedb, evm.Config{})
	// 获取最新区块的区块头
	curblock := blockchain.CurrentBlock()
	curblock.BaseFee = big.NewInt(0)
	// 获取最新区块
	NewBlock := blockchain.GetBlock(curblock.Hash(), curblock.Number.Uint64())
	// 交易赋值
	NewBlock.SetTransactions(txs)

	// 新建执行器
	processer := core.NewStateProcessor(chainCfg, blockchain)
	// 新建EVM执行环境
	// 执行交易
	returnmsg, err := processer.Process(NewBlock, statedb, evm.Config{
		Tracer:                  nil,
		NoBaseFee:               false,
		EnablePreimageRecording: false,
		ExtraEips:               nil,
	})
	if err != nil {
		fmt.Printf("%sERROR MSG%s   测试函数交易执行失败 err = %v\n", types.FRED, types.FRESET, err)
		t.Fatalf("failed to import forked block chain: %v", err)
		return
	}
	fmt.Printf("交易结果：%v\n", returnmsg)
	if returnmsg.ErrTx != nil {
		fmt.Printf("%sERROR MSG%s   交易中出现了错误 err = %v\n", types.FRED, types.FRESET, err)
		for _, value := range returnmsg.ErrTx {
			fmt.Printf("错误 %v", value.ErrorMsg)
		}
	}
}

// 测试多笔转账交易 单组串行
func TestTripleTransferTX(t *testing.T) {
	// 创建一笔交易
	txs := NewTripleTX()
	fmt.Printf("%sPROMPT MSG%s   创建了三笔交易，该交易内容是 %v", types.FGREEN, types.FRESET, txs[0])

	// ToAddr := common.BytesToAddress(common.FromHex(BAddress)) // 目的地址
	FromKey, _ := crypto.ToECDSA(AKeyBytes)
	FromAddr := crypto.PubkeyToAddress(FromKey.PublicKey) // 发送地址
	CKey, _ := crypto.ToECDSA(CKeyBytes)
	CAddr := crypto.PubkeyToAddress(CKey.PublicKey)

	statedb, _ := state.New(types.EmptyRootHash, state.NewDatabase(db), nil)
	statedb.SetBalance(FromAddr, big.NewInt(99999999999999999)) // A发送地址余额增加
	statedb.SetBalance(CAddr, big.NewInt(99999999999999999))    // C发送地址余额增加

	fmt.Println(common.Bytes2Hex(txs[0][0].RawSigValues()))
	// statedb, _ := state.New(types.EmptyRootHash, state.NewDatabase(db), nil)
	// statedb.SetBalance(FromAddr, big.NewInt(99999999999999999)) // 发送地址余额增加

	// 创建一条完整链作为父链
	// 模拟一个区块链
	chainCfg := &params.ChainConfig{
		ChainID: big.NewInt(1337),
	}
	// 起链
	blockchain := core.NewBlokchain(chainCfg, statedb, evm.Config{})
	// 获取最新区块的区块头
	curblock := blockchain.CurrentBlock()
	curblock.BaseFee = big.NewInt(0)
	// 获取最新区块
	NewBlock := blockchain.GetBlock(curblock.Hash(), curblock.Number.Uint64())
	// 交易赋值
	NewBlock.SetTransactions(txs)

	// 新建执行器
	processer := core.NewStateProcessor(chainCfg, blockchain)
	// 新建EVM执行环境
	// 执行交易
	returnmsg, err := processer.Process(NewBlock, statedb, evm.Config{
		Tracer:                  nil,
		NoBaseFee:               false,
		EnablePreimageRecording: false,
		ExtraEips:               nil,
	})
	if err != nil {
		fmt.Printf("%sERROR MSG%s   测试函数交易执行失败 err = %v\n", types.FRED, types.FRESET, err)
		t.Fatalf("failed to import forked block chain: %v", err)
		return
	}
	fmt.Printf("交易结果：%v\n", returnmsg)
	if returnmsg.ErrTx != nil {
		fmt.Printf("%sERROR MSG%s   交易中出现了错误 err = %v\n", types.FRED, types.FRESET, err)
		for _, value := range returnmsg.ErrTx {
			fmt.Printf("错误 %v", value.ErrorMsg)
		}
	}
}
