package executor

import (
	"context"
	"fmt"

	"github.com/SipengXie/pangu/core"
	"github.com/SipengXie/pangu/core/txpool"
	"github.com/SipengXie/pangu/core/types"
	"github.com/SipengXie/pangu/event"
	"github.com/SipengXie/pangu/pb"

	"google.golang.org/protobuf/proto"
)

const txChanSize = 4096

type ExecutorService struct {
	executionTxsCh  chan core.NewTxsEvent
	executionTxsSub event.Subscription // txsSub = pendingPool.SubscribeNewTxsEvent(txsCh)
	executionPool   *txpool.TxPool     // may be the execution pool

	pendingTxsCh  chan core.NewTxsEvent
	pendingTxsSub event.Subscription // txsSub = pendingPool.SubscribeNewTxsEvent(txsCh)
	pendingPool   *txpool.TxPool     // may be the pending pool

	p2pClient pb.P2PClient
	pb.UnimplementedExecutorServer
}

func NewExecutorService(ePool, pPool *txpool.TxPool, Cli pb.P2PClient) *ExecutorService {
	es := &ExecutorService{
		executionTxsCh: make(chan core.NewTxsEvent, txChanSize),
		executionPool:  ePool,
		pendingTxsCh:   make(chan core.NewTxsEvent, txChanSize),
		pendingPool:    pPool,
		p2pClient:      Cli,
	}
	es.executionTxsSub = es.executionPool.SubscribeNewTxsEvent(es.executionTxsCh)
	es.pendingTxsSub = es.pendingPool.SubscribeNewTxsEvent(es.pendingTxsCh)
	go es.SendLoop()
	go es.ExecuteLoop()
	return es
}

func (e *ExecutorService) AddTx(tx *types.Transaction) error {
	var txs []*txpool.Transaction
	txs = append(txs, &txpool.Transaction{Tx: tx})
	errs := e.executionPool.Add(txs, true, false)
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
	for {
		select {
		case ev := <-e.executionTxsCh:
			for _, tx := range ev.Txs {
				// TODO : 执行交易
				fmt.Println("get a Tx from consensus layer:", tx)
			}
		case <-e.executionTxsSub.Err():
			return // if error then exit
		}
	}
}
