// 并行执行交易测试文件

package tests

import (
	"bytes"
	"fmt"
	"github.com/SipengXie/pangu/common"
	"github.com/SipengXie/pangu/core/rawdb"
	"github.com/SipengXie/pangu/core/state"
	"github.com/SipengXie/pangu/core/txpool"
	"github.com/SipengXie/pangu/core/txpool/legacypool"
	"github.com/SipengXie/pangu/core/types"
	"github.com/SipengXie/pangu/crypto"
	"github.com/SipengXie/pangu/event"
	"github.com/SipengXie/pangu/executor"
	"github.com/SipengXie/pangu/params"
	"github.com/SipengXie/pangu/pb"
	"github.com/SipengXie/pangu/trie"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/vm"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/grpclog"
	"math/big"
	"net"
	"sync"
	"sync/atomic"
	"testing"
)

const (
	testCode  = "0x608060405234801561001057600080fd5b5060016000819055506101cb806100286000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c80630464287d1461003b57806361bc221a14610057575b600080fd5b610055600480360381019061005091906100d1565b610075565b005b61005f610090565b60405161006c9190610117565b60405180910390f35b806000808282546100869190610161565b9250508190555050565b60005481565b600080fd5b6000819050919050565b6100ae8161009b565b81146100b957600080fd5b50565b6000813590506100cb816100a5565b92915050565b6000602082840312156100e7576100e6610096565b5b60006100f5848285016100bc565b91505092915050565b6000819050919050565b610111816100fe565b82525050565b600060208201905061012c6000830184610108565b92915050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b600061016c826100fe565b9150610177836100fe565b925082820190508082111561018f5761018e610132565b5b9291505056fea2646970667358221220cf4fd818e53a6d4bd9dce4d01f4ef5c38903de07c072a3e16aec183c4cc41c8364736f6c63430008120033"
	testCode1 = "0x608060405234801561001057600080fd5b50600080819055506101f1806100276000396000f3fe608060405234801561001057600080fd5b50600436106100365760003560e01c8063771602f71461003b578063853255cc14610057575b600080fd5b610055600480360381019061005091906100cc565b610075565b005b61005f61008b565b60405161006c919061011b565b60405180910390f35b80826100819190610165565b6000819055505050565b60005481565b600080fd5b6000819050919050565b6100a981610096565b81146100b457600080fd5b50565b6000813590506100c6816100a0565b92915050565b600080604083850312156100e3576100e2610091565b5b60006100f1858286016100b7565b9250506020610102858286016100b7565b9150509250929050565b61011581610096565b82525050565b6000602082019050610130600083018461010c565b92915050565b7f4e487b7100000000000000000000000000000000000000000000000000000000600052601160045260246000fd5b600061017082610096565b915061017b83610096565b9250827fffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff038211156101b0576101af610136565b5b82820190509291505056fea2646970667358221220d9b55c3436462222f4b927fc94b64fdc2585461e87ba9a31407ec35371d1826364736f6c634300080c0033"
	testGas   = 1441090
	testTxGas = 210000 // 转账交易的基础汽油费
)

var (
	Address          = "127.0.0.1:9876"
	testTxPoolConfig legacypool.Config
	db               = rawdb.NewMemoryDatabase()
	eip1559Config    *params.ChainConfig

	// Test accounts
	testBankKey, _  = crypto.GenerateKey()
	testBankAddress = crypto.PubkeyToAddress(testBankKey.PublicKey)
	testBankFunds   = big.NewInt(9000000000000000000)

	testUserKey, _  = crypto.GenerateKey()
	testUserAddress = crypto.PubkeyToAddress(testUserKey.PublicKey)

	testUserA_Key, _  = crypto.GenerateKey()
	testUserA_Address = crypto.PubkeyToAddress(testUserA_Key.PublicKey)

	testUserB_Key, _  = crypto.GenerateKey()
	testUserB_Address = crypto.PubkeyToAddress(testUserB_Key.PublicKey)

	testUserC_Key, _  = crypto.GenerateKey()
	testUserC_Address = crypto.PubkeyToAddress(testUserC_Key.PublicKey)

	testUserD_Key, _  = crypto.GenerateKey()
	testUserD_Address = crypto.PubkeyToAddress(testUserD_Key.PublicKey)

	testUserE_Key, _  = crypto.GenerateKey()
	testUserE_Address = crypto.PubkeyToAddress(testUserE_Key.PublicKey)

	testContractA1 = crypto.CreateAddress(testBankAddress, 0)
	testContractA2 = crypto.CreateAddress(testBankAddress, 2)
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

func (bc *testBlockChain) SubscribeChainHeadEvent(ch chan<- core.ChainHeadEvent) event.Subscription {
	return bc.chainHeadFeed.Subscribe(ch)
}

func newRandomTx(creation bool, nonce uint64) *types.Transaction {
	var tx *types.Transaction
	gasPrice := big.NewInt(10 * params.InitialBaseFee)
	if creation {
		tx, _ = types.SignTx(types.NewContractCreation(nonce, big.NewInt(0), testGas, gasPrice, common.FromHex(testCode)), types.HomesteadSigner{}, testBankKey)
	} else {
		tx, _ = types.SignTx(types.NewTransaction(nonce, testUserAddress, big.NewInt(1000), params.TxGas, gasPrice, nil), types.HomesteadSigner{}, testBankKey)
	}
	// tx, _ = types.SignTx(types.NewPankuTransaction(nonce, testUserAddress, big.NewInt(1000), params.TxGas, gasPrice, nil, big.NewInt(1), big.NewInt(2), nil), types.HomesteadSigner{}, testBankKey)
	return tx
}

func pankuTx(nonce uint64, to common.Address, amount *big.Int, gasLimit uint64, gasPrice *big.Int, data []byte, gasFee *big.Int, tip *big.Int, al types.AccessList, key *ecdsa.PrivateKey, from common.Address) *types.Transaction {
	res := bytes.Compare(to.Bytes(), types.EmptyAddress)
	if res == 0 { // 合约创建交易
		al := types.AccessList{
			types.AccessTuple{
				Address:     from,
				StorageKeys: nil,
			},
		}
		tx, _ := types.SignNewTx(key, types.LatestSignerForChainID(params.TestChainConfig.ChainID), &types.PankuTx{
			ChainID:    params.TestChainConfig.ChainID,
			Nonce:      nonce,
			GasTipCap:  tip,
			GasFeeCap:  gasFee,
			Gas:        gasLimit,
			To:         nil,
			Value:      amount,
			Data:       data,
			AccessList: al,
		})
		return tx
	} else {
		al := types.AccessList{
			types.AccessTuple{
				Address:     to,
				StorageKeys: nil,
			},
			types.AccessTuple{
				Address:     from,
				StorageKeys: nil,
			},
		}

		// var result = "{\n"
		// result += "\tFrom: " + from.Hex() + ",\n"
		// nonce_str := strconv.Itoa(int(nonce))
		// result += "\tNonce: " + nonce_str + ",\n"
		// result += "\tTo: " + to.Hex() + ",\n"
		// result += "}\n"
		// fmt.Println(result)

		tx, _ := types.SignNewTx(key, types.LatestSignerForChainID(params.TestChainConfig.ChainID), &types.PankuTx{
			ChainID:    params.TestChainConfig.ChainID,
			Nonce:      nonce,
			GasTipCap:  tip,
			GasFeeCap:  gasFee,
			Gas:        gasLimit,
			To:         &to,
			Value:      amount,
			Data:       data,
			AccessList: al,
		})
		return tx
	}
}

// 构造3比交易，A -> B, B->C , D->E
func newPankutxs() []*types.Transaction {
	// gasPrice := big.NewInt(10 * params.InitialBaseFee)
	gasPrice := big.NewInt(0)
	// 合约交易
	tx0 := pankuTx(0, common.BytesToAddress(types.EmptyAddress), big.NewInt(0), testGas, gasPrice, common.FromHex(testCode), big.NewInt(10000000199), big.NewInt(1), nil, testBankKey, testBankAddress)
	// A -> B 100
	tx1 := pankuTx(0, testUserB_Address, big.NewInt(100), testTxGas, gasPrice, nil, big.NewInt(10000000199), big.NewInt(1), nil, testUserA_Key, testUserA_Address)
	// B -> C 200
	tx2 := pankuTx(0, testUserC_Address, big.NewInt(200), testTxGas, gasPrice, nil, big.NewInt(10000000299), big.NewInt(1), nil, testUserB_Key, testUserB_Address)
	// D -> E 100
	tx3 := pankuTx(0, testUserE_Address, big.NewInt(100), testTxGas, gasPrice, nil, big.NewInt(10000000299), big.NewInt(1), nil, testUserD_Key, testUserD_Address)
	var txs types.Transactions
	txs = append(txs, tx0)
	txs = append(txs, tx1)
	txs = append(txs, tx2)
	txs = append(txs, tx3)
	return txs
}

func newContractTx() []*types.Transaction {
	gasPrice := big.NewInt(10 * params.InitialBaseFee)
	inputdataA := "0x0464287d0000000000000000000000000000000000000000000000000000000000000002"
	inputdataB := "0x771602f700000000000000000000000000000000000000000000000000000000000000010000000000000000000000000000000000000000000000000000000000000002"
	// 用户A 调用 合约1
	tx0 := pankuTx(0, testContractA1, big.NewInt(0), testTxGas, gasPrice, common.FromHex(inputdataA), big.NewInt(10000000199), big.NewInt(1), nil, testUserA_Key, testUserA_Address)
	// 用户A 转账给 用户C
	tx1 := pankuTx(1, testUserC_Address, big.NewInt(100), testTxGas, gasPrice, nil, big.NewInt(10000000199), big.NewInt(1), nil, testUserA_Key, testUserA_Address)
	// ! 用户C 转账给 用户D (note : 这笔交易加上后将会全部串行)
	// tx2 := pankuTx(0, testUserD_Address, big.NewInt(100), testTxGas, gasPrice, nil, big.NewInt(10000000299), big.NewInt(1), nil, testUserC_Key, testUserC_Address)
	// 用户D 转账给 用户E
	tx3 := pankuTx(0, testUserE_Address, big.NewInt(100), testTxGas, gasPrice, nil, big.NewInt(10000000299), big.NewInt(1), nil, testUserD_Key, testUserD_Address)
	// 用户D 调用 合约2
	tx4 := pankuTx(1, testContractA2, big.NewInt(0), testTxGas, gasPrice, common.FromHex(inputdataB), big.NewInt(10000000199), big.NewInt(1), nil, testUserD_Key, testUserD_Address)
	var txs types.Transactions
	txs = append(txs, tx0)
	txs = append(txs, tx1)
	// txs = append(txs, tx2)
	txs = append(txs, tx3)
	txs = append(txs, tx4)
	return txs
}

// 测试共识和执行的交互
func TestCAE(t *testing.T) {
	// init
	statedb, _ := state.New(types.EmptyRootHash, state.NewDatabase(db), nil)
	statedb.SetBalance(testBankAddress, testBankFunds)
	blockchain := newTestBlockChain(eip1559Config, 10000000, statedb, new(event.Feed))

	// 实例化TxPool
	epool := legacypool.New(testTxPoolConfig, blockchain)
	etxpool, _ := txpool.New(new(big.Int).SetUint64(testTxPoolConfig.PriceLimit), blockchain, []txpool.SubPool{epool})
	defer etxpool.Close()

	ppool := legacypool.New(testTxPoolConfig, blockchain)
	ptxpool, _ := txpool.New(new(big.Int).SetUint64(testTxPoolConfig.PriceLimit), blockchain, []txpool.SubPool{ppool})
	defer ptxpool.Close()

	// 启动共识客户端
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("Failed to dial gRPC server: %v", err)
	}
	p2pClient := pb.NewP2PClient(conn)

	// 实例化executorService
	es := executor.NewExecutorService(etxpool, ptxpool, p2pClient)

	// 启动服务端
	listen, err := net.Listen("tcp", Address)
	if err != nil {
		grpclog.Fatalf("Failed to listen: %v", err)
	}
	defer listen.Close()

	s := grpc.NewServer()
	pb.RegisterExecutorServer(s, es)
	fmt.Println("Listen on " + Address)
	grpclog.Println("Listen on " + Address)

	go s.Serve(listen)

	// 构造随机交易
	var tempTxs []*txpool.Transaction
	for i := 0; i < 2; i++ {
		ptx := &txpool.Transaction{Tx: newRandomTx(i%2 == 0, uint64(i))}
		tempTxs = append(tempTxs, ptx)
	}
	ppool.Add(tempTxs, true, false)

	// go es.SendLoop()
	// go es.ExecuteLoop()

	// es.SendLoop()
	// es.ExecuteLoop()
	for {
	}
}

// TODO: 测试交易执行函数
func TestTxExec1(t *testing.T) {
	// 构造测试环境
	statedb, _ := state.New(types.EmptyRootHash, state.NewDatabase(db), nil)
	statedb.SetBalance(testBankAddress, testBankFunds)
	statedb.SetBalance(testUserA_Address, big.NewInt(1000))
	statedb.SetBalance(testUserA_Address, big.NewInt(1000))
	statedb.SetBalance(testUserA_Address, big.NewInt(1000))
	statedb.SetBalance(testUserA_Address, big.NewInt(1000))
	// blockchain := newTestBlockChain(eip1559Config, 10000000, statedb, new(event.Feed))

	// ? 构造三比确定的交易
	tempTxs := newPankutxs()

	genDb, _, blockchain, err := newCanonical(ethash.NewFaker(), 0, false) // 创建了一个完整链作为parent
	// blockchain作为parent TODO: 总链
	if err != nil {
		fmt.Println("newCanonical方法报错")
	}

	var blockChainB []*types.Block // 所有区块
	// n 表示创建n个区块，返回的区块具有空的交易
	blockChainB = core.MakeBlockChain(eip1559Config, blockchain.GetBlockByHash(blockchain.CurrentBlock().Hash()), 1, ethash.NewFaker(), genDb, 1) // 在parent的基础上创建一个确定性的chain

	// TODO: 交易赋值
	for _, block := range blockChainB {
		block.SetTransactions(tempTxs)
		// fmt.Println("替换交易完成")
	}

	// 从总链上获取最新的区块
	cur := blockchain.CurrentBlock()
	_ = blockchain.GetTd(cur.Hash(), cur.Number.Uint64()) // GetTd通过哈希和数字从数据库中检索规范链中的块的总难度，如果找到则缓存。

	// 只能定义在不是test的函数中，我写到了block.go文件中
	if err := core.MakeBlockChainImport(blockChainB, blockchain, statedb); err != nil {
		// t.Fatalf("failed to import forked block chain: %v", err)
	}
}

// TODO: 合约运行
func TestTxExec2(t *testing.T) {
	// 构造测试环境
	statedb, _ := state.New(types.EmptyRootHash, state.NewDatabase(db), nil)
	statedb.SetBalance(testBankAddress, testBankFunds)
	statedb.SetBalance(testUserA_Address, big.NewInt(9210000006279100))
	statedb.SetBalance(testUserB_Address, big.NewInt(9210000006279100))
	statedb.SetBalance(testUserC_Address, big.NewInt(9210000006279100))
	statedb.SetBalance(testUserD_Address, big.NewInt(9210000006279100))
	// blockchain := newTestBlockChain(eip1559Config, 10000000, statedb, new(event.Feed))

	// ? 构造三比确定的交易
	tempTxs := newPankutxs()

	genDb, _, blockchain, err := newCanonical(ethash.NewFaker(), 0, false) // 创建了一个完整链作为parent
	// blockchain作为parent TODO: 总链
	if err != nil {
		fmt.Println("newCanonical方法报错")
	}

	var blockChainB []*types.Block // 所有区块
	// n 表示创建n个区块，返回的区块具有空的交易
	blockChainB = core.MakeBlockChain(eip1559Config, blockchain.GetBlockByHash(blockchain.CurrentBlock().Hash()), 1, ethash.NewFaker(), genDb, 1) // 在parent的基础上创建一个确定性的chain

	// TODO: 交易赋值
	for _, block := range blockChainB {
		block.SetTransactions(tempTxs)
		// fmt.Println("替换交易完成")
	}

	// 从总链上获取最新的区块
	cur := blockchain.CurrentBlock()
	_ = blockchain.GetTd(cur.Hash(), cur.Number.Uint64()) // GetTd通过哈希和数字从数据库中检索规范链中的块的总难度，如果找到则缓存。

	// 只能定义在不是test的函数中，我写到了block.go文件中
	if err := core.MakeBlockChainImport(blockChainB, blockchain, statedb); err != nil {
		t.Fatalf("failed to import forked block chain: %v", err)
	}
}

// 测试带合约调用的交易并行执行
func TestTxExec3(t *testing.T) {
	// 构造测试环境
	statedb, _ := state.New(types.EmptyRootHash, state.NewDatabase(db), nil)
	statedb.SetBalance(testBankAddress, testBankFunds)
	// statedb.IntermediateRoot(true)
	statedb.SetBalance(testUserA_Address, big.NewInt(9210000006279100))
	// statedb.IntermediateRoot(true)
	statedb.SetBalance(testUserB_Address, big.NewInt(9210000006279100))
	// statedb.IntermediateRoot(true)
	statedb.SetBalance(testUserC_Address, big.NewInt(9210000006279100))
	// statedb.IntermediateRoot(true)
	statedb.SetBalance(testUserD_Address, big.NewInt(9210000006279100))
	// statedb.IntermediateRoot(true)
	// blockchain := newTestBlockChain(eip1559Config, 10000000, statedb, new(event.Feed))
	statedb.SetCode(testContractA1, common.FromHex(testCode))
	statedb.SetNonce(testContractA1, 0)
	statedb.SetBalance(testContractA1, big.NewInt(9210000006279100))
	// statedb.IntermediateRoot(true)
	statedb.SetCode(testContractA2, common.FromHex(testCode1))
	statedb.SetNonce(testContractA2, 0)
	statedb.SetBalance(testContractA2, big.NewInt(9210000006279100))
	statedb.IntermediateRoot(true)
	//statedb.Commit(true)
	tempTxs := newContractTx()
	// tempTxs := newDyftxs()

	genDb, _, blockchain, err := newCanonical(ethash.NewFaker(), 0, false) // 创建了一个完整链作为parent
	// blockchain作为parent TODO: 总链
	if err != nil {
		fmt.Println("newCanonical方法报错")
	}

	var blockChainB []*types.Block // 所有区块
	// n 表示创建n个区块，返回的区块具有空的交易
	blockChainB = core.MakeBlockChain(eip1559Config, blockchain.GetBlockByHash(blockchain.CurrentBlock().Hash()), 1, ethash.NewFaker(), genDb, 1) // 在parent的基础上创建一个确定性的chain

	// TODO: 交易赋值
	for _, block := range blockChainB {
		block.SetTransactions(tempTxs)
		// fmt.Println("替换交易完成")
	}

	// 从总链上获取最新的区块
	cur := blockchain.CurrentBlock()
	_ = blockchain.GetTd(cur.Hash(), cur.Number.Uint64()) // GetTd通过哈希和数字从数据库中检索规范链中的块的总难度，如果找到则缓存。
	// 只能定义在不是test的函数中，我写到了block.go文件中
	if err := core.MakeBlockChainImport(blockChainB, blockchain, statedb); err != nil {
		t.Fatalf("failed to import forked block chain: %v", err)
	}
}

// newCanonical 创建了一个链数据库，并注入了一个确定性的规范链
// 根据full标志，它创建一个完整的块链
// 如果需要更多的测试块，还返回数据库和用于块生成的初始规范
func newCanonical(engine consensus.Engine, n int, full bool) (ethdb.Database, *core.Genesis, *core.BlockChain, error) {
	var (
		genesis = &core.Genesis{
			BaseFee: big.NewInt(params.InitialBaseFee),
			Config:  params.AllEthashProtocolChanges,
		}
	)
	// 仅使用一个创世区块初始化一个新的链
	blockchain, _ := core.NewBlockChain(rawdb.NewMemoryDatabase(), nil, genesis, nil, engine, vm.Config{}, nil, nil)

	// 创建并注入请求的链
	if n == 0 {
		return rawdb.NewMemoryDatabase(), genesis, blockchain, nil
	}
	if full {
		// 创建一条完整链
		genDb, blocks := core.MakeBlockChainWithGenesis(genesis, n, engine, 1) // ? 从genesis创建确定性的chain
		_, err := blockchain.InsertChain(blocks)
		return genDb, genesis, blockchain, err
	} else {
		// 请求仅包含头部的链
		genDb, headers := core.MakeHeaderChainWithGenesis(genesis, n, engine, 1)
		_, err := blockchain.InsertHeaderChain(headers)
		return genDb, genesis, blockchain, err
	}
}

func TestMap(t *testing.T) {
	m1 := make(map[int]string)
	var wg sync.WaitGroup
	wg.Add(2)
	go func(m map[int]string) {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			m[2] = "nihao"
		}
		// m[2] = "nihao"
	}(m1)
	go func(m map[int]string) {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			m[1] = "nihao"
		}
		// m[1] = "nihao"
	}(m1)
	wg.Wait()
}

// 测试带合约调用的交易并行执行
func TestTxExec33(t *testing.T) {
	// 构造测试环境
	statedb, _ := state.New(types.EmptyRootHash, state.NewDatabase(db), nil)
	statedb.SetBalance(testBankAddress, testBankFunds)
	statedb.IntermediateRoot(true)
	statedb.SetBalance(testUserA_Address, big.NewInt(9210000006279100))
	statedb.IntermediateRoot(true)
	statedb.SetBalance(testUserB_Address, big.NewInt(9210000006279100))
	statedb.IntermediateRoot(true)
	statedb.SetBalance(testUserC_Address, big.NewInt(9210000006279100))
	statedb.IntermediateRoot(true)
	statedb.SetBalance(testUserD_Address, big.NewInt(9210000006279100))
	statedb.IntermediateRoot(true)
	// blockchain := newTestBlockChain(eip1559Config, 10000000, statedb, new(event.Feed))
	statedb.SetCode(testContractA1, common.FromHex(testCode))
	statedb.SetNonce(testContractA1, 0)
	statedb.SetBalance(testContractA1, big.NewInt(9210000006279100))
	statedb.IntermediateRoot(true)
	statedb.SetCode(testContractA2, common.FromHex(testCode1))
	statedb.SetNonce(testContractA2, 0)
	statedb.SetBalance(testContractA2, big.NewInt(9210000006279100))
	statedb.IntermediateRoot(true)
	tempTxs := newContractTx()
	// tempTxs := newDyftxs()

	/*  并行组
		- 第一笔交易正常执行，可以并行
		- 第二笔交易正常执行，不可以并行
		- 第三笔交易正常执行，可以并行
		- 第四笔交易正常执行，可以并行
		- 第五笔交易正常执行，不可以并行

		串行组
		- 第一笔交易正常执行，成功修改AccessList
	!	- 第二笔交易执行失败，"空的result" preCheck出错
	*/

	genDb, _, blockchain, err := newCanonical(ethash.NewFaker(), 0, false) // 创建了一个完整链作为parent
	// blockchain作为parent TODO: 总链
	if err != nil {
		fmt.Println("newCanonical方法报错")
	}

	var blockChainB []*types.Block // 所有区块
	// n 表示创建n个区块，返回的区块具有空的交易
	blockChainB = core.MakeBlockChain(eip1559Config, blockchain.GetBlockByHash(blockchain.CurrentBlock().Hash()), 1, ethash.NewFaker(), genDb, 1) // 在parent的基础上创建一个确定性的chain

	// TODO: 交易赋值
	for _, block := range blockChainB {
		block.SetTransactions(tempTxs)
		// fmt.Println("替换交易完成")
	}

	// 从总链上获取最新的区块
	cur := blockchain.CurrentBlock()
	_ = blockchain.GetTd(cur.Hash(), cur.Number.Uint64()) // GetTd通过哈希和数字从数据库中检索规范链中的块的总难度，如果找到则缓存。
	// 只能定义在不是test的函数中，我写到了block.go文件中
	if err := core.MakeBlockChainImport(blockChainB, blockchain, statedb); err != nil {
		t.Fatalf("failed to import forked block chain: %v", err)
	}
}
