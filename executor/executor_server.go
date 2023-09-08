package executor

import (
	"context"
	"fmt"
	"math/big"
	"time"

	"github.com/SipengXie/pangu/common"
	"github.com/SipengXie/pangu/core"
	"github.com/SipengXie/pangu/log"
	"github.com/SipengXie/pangu/trie"

	"github.com/SipengXie/pangu/core/txpool"
	"github.com/SipengXie/pangu/core/types"
	"github.com/SipengXie/pangu/event"
	"github.com/SipengXie/pangu/pb"

	"google.golang.org/protobuf/proto"
)

const txChanSize = 4096

var (
	COINBASEBYTE = []byte{1}
	COINBASE     = common.BytesToAddress(COINBASEBYTE)
)

type ExecutorService struct {
	BlockChain *core.Blockchain
	Processer  *core.Processor

	executionTxsCh  chan types.NewTxsEvent
	executionTxsSub event.Subscription // txsSub = pendingPool.SubscribeNewTxsEvent(txsCh)
	executionPool   *txpool.TxPool     // may be the execution pool

	pendingTxsCh  chan types.NewTxsEvent
	pendingTxsSub event.Subscription // txsSub = pendingPool.SubscribeNewTxsEvent(txsCh)
	pendingPool   *txpool.TxPool     // may be the pending pool

	p2pClient pb.P2PClient
	pb.UnimplementedExecutorServer

	// extra channels
	initBlockCh chan struct{}
}

func NewExecutorService(ePool, pPool *txpool.TxPool, bc *core.Blockchain, Cli pb.P2PClient) *ExecutorService {
	es := &ExecutorService{
		BlockChain:     bc,
		executionTxsCh: make(chan types.NewTxsEvent, txChanSize),
		executionPool:  ePool,
		pendingTxsCh:   make(chan types.NewTxsEvent, txChanSize),
		pendingPool:    pPool,
		p2pClient:      Cli,
		initBlockCh:    make(chan struct{}),
	}
	es.Processer = core.NewStateProcessor(es.BlockChain.Config(), es.BlockChain)
	es.executionTxsSub = es.executionPool.SubscribeNewTxsEvent(es.executionTxsCh)
	es.pendingTxsSub = es.pendingPool.SubscribeNewTxsEvent(es.pendingTxsCh)
	fmt.Println("go send loop")
	go es.SendLoop()
	fmt.Println("go execute loop")
	go es.ExecuteLoop()
	return es
}

func (e *ExecutorService) AddTxToExecutionPool(tx *types.Transaction) error {
	fmt.Println("Add Transaction to execution txpool")
	var txs []*txpool.Transaction
	txs = append(txs, &txpool.Transaction{Tx: tx})
	errs := e.executionPool.Add(txs, true, false)
	return errs[0]
}

func (e *ExecutorService) AddTxToPendingPool(tx *types.Transaction) error {
	fmt.Println("Add Transaction to pending txpool")
	var txs []*txpool.Transaction
	txs = append(txs, &txpool.Transaction{Tx: tx})
	errs := e.pendingPool.Add(txs, true, false)
	return errs[0]
}

// need add a loop routine to sendTx to consensus layer, when txsCh has new txs
func (e *ExecutorService) sendTx(tx *types.Transaction) (*pb.Empty, error) {
	data, err := tx.MarshalBinary()
	if err != nil {
		return nil, err
	}
	ptx := &pb.Transaction{
		Type:    pb.TransactionType_NORMAL,
		Payload: data,
	}
	btx, err := proto.Marshal(ptx)
	if err != nil {
		return nil, err
	}
	request := &pb.Request{Tx: btx}
	rawRequest, err := proto.Marshal(request)
	if err != nil {
		return nil, err
	}
	packet := &pb.Packet{
		Msg:         rawRequest,
		ConsensusID: -1,
		Epoch:       -1,
		Type:        pb.PacketType_CLIENTPACKET,
	}
	_, err = e.p2pClient.Send(context.Background(), packet)
	if err != nil {
		return nil, err
	}
	return &pb.Empty{}, nil
}

func (e *ExecutorService) CommitBlock(ctx context.Context, pbBlock *pb.ExecBlock) (*pb.Empty, error) {
	var Localtxs []*txpool.Transaction
	var Remotetxs []*txpool.Transaction
	var err error = nil
	for _, pbtx := range pbBlock.GetTxs() {
		tx1 := new(pb.Transaction)
		_ = proto.Unmarshal(pbtx, tx1)
		btx := tx1.Payload
		tx := new(types.Transaction)
		err = tx.UnmarshalBinary(btx)
		if err != nil {
			continue
		}
		fmt.Println(tx.Nonce())
		ptx := &txpool.Transaction{Tx: tx}
		if e.pendingPool.IsLocalTx(tx) {
			Localtxs = append(Localtxs, ptx)
		} else {
			Remotetxs = append(Remotetxs, ptx)
		}
	}
	e.executionPool.Add(Localtxs, true, false)
	e.executionPool.Add(Remotetxs, false, false)
	return &pb.Empty{}, err
}

func (e *ExecutorService) VerifyTx(ctx context.Context, pTx *pb.Transaction) (*pb.Result, error) {
	if pTx.Type != pb.TransactionType_NORMAL && pTx.Type != pb.TransactionType_UPGRADE {
		return &pb.Result{Success: false}, nil
	}
	tx := new(types.Transaction)
	err := tx.UnmarshalBinary(pTx.Payload)
	if err != nil {
		return &pb.Result{Success: false}, nil
	}
	// default all txs here are remote
	err = e.executionPool.ValidateTx(tx, false)
	if err != nil {
		return &pb.Result{Success: false}, nil
	}
	return &pb.Result{Success: true}, nil
}

func (e *ExecutorService) SendLoop() {
	defer e.pendingTxsSub.Unsubscribe()
	for {
		select {
		case ev := <-e.pendingTxsCh:
			fmt.Println("start send tx")
			fmt.Println(len(ev.Txs))
			for _, tx := range ev.Txs {
				e.sendTx(tx) // send tx to consensus layer
			}
		case <-e.pendingTxsSub.Err():
			return // if error then exit
		}
	}
}

func (e *ExecutorService) ExecuteLoop() {
	defer e.executionTxsSub.Unsubscribe()
	var header *types.Header
	var txs types.Transactions
	e.initBlockCh <- struct{}{}
	for {
		select {
		case <-e.initBlockCh:
			txs = make(types.Transactions, 0)
			header = types.CopyHeader(e.initHeader(COINBASE, 12345678))
		case ev := <-e.executionTxsCh:
			// txs := make(map[common.Address][]*types.Transaction, len(ev.Txs))
			// 将交易打包进区块
			// TODO : 完善打包区块的逻辑
			for _, tx := range ev.Txs {
				txs = append(txs, tx)
			}
			if len(txs) >= 10 {
				signer := types.MakeSigner(e.BlockChain.Config(), header.Number, header.Time)
				// 交易分组
				blockTxs := core.ClassifyTx(txs, signer)
				block := types.InitBlock(header, blockTxs)
				// 将区块发送执行
				statedb, _ := e.BlockChain.StateAt(e.BlockChain.CurrentBlock().StateRoot)
				processRes, err := e.Processer.Process(block, statedb, e.BlockChain.VmConfig())
				if err != nil {
					panic(err)
				}
				// 生成可上链的block
				okBlock := types.NewBlock(block.Header(), blockTxs, processRes.Receipt, processRes.RootHash, trie.NewStackTrie(nil))

				// 执行后将block传入一个管道，然后上链
				status, err := e.BlockChain.WriteBlockAndSetHead(okBlock, processRes.Receipt, processRes.Logs, statedb, true)
				log.Info("writeBlock status : ", status)
				if err != nil {
					panic(err)
				}
				// 发送新建block的请求（写入initBlockCH）
				e.initBlockCh <- struct{}{}
			} else {
				continue
			}
		case <-e.executionTxsSub.Err():
			return // if error then exit
		}
	}
}

func (e *ExecutorService) fillTransactionsToBlock() {
}

func (e *ExecutorService) initHeader(coinBase common.Address, gasLimit uint64) *types.Header {
	blockNum := big.NewInt(0)
	header := &types.Header{
		ParentHash: e.BlockChain.CurrentBlock().Hash(),
		Time:       uint64(time.Now().Unix()),
		Number:     blockNum.Add(e.BlockChain.CurrentBlock().Number, big.NewInt(1)),
		Coinbase:   coinBase,
		GasLimit:   gasLimit,
	}
	return header
}
