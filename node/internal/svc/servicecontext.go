package svc

import (
	"github.com/SipengXie/pangu/core"
	"github.com/SipengXie/pangu/core/rawdb"
	"github.com/SipengXie/pangu/core/state"
	"github.com/SipengXie/pangu/core/txpool"
	"github.com/SipengXie/pangu/core/txpool/legacypool"
	"github.com/SipengXie/pangu/core/types"
	"github.com/SipengXie/pangu/executor"
	"github.com/SipengXie/pangu/node/internal/config"
	"github.com/SipengXie/pangu/params"
	"github.com/SipengXie/pangu/pb"
	"math/big"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	db = rawdb.NewMemoryDatabase()
)

type ServiceContext struct {
	Config          config.Config
	ExecutorService *executor.ExecutorService
}

func NewServiceContext(c config.Config) *ServiceContext {
	// TODO : 工程上需要进一步构筑blockchain的逻辑
	// 默认起好了一条链
	statedb, _ := state.New(types.EmptyRootHash, state.NewDatabase(db), nil)

	// 模拟一个区块链
	var chainCfg *params.ChainConfig
	chainCfg.ChainID = big.NewInt(1)
	blockchain := core.NewBlokchain(chainCfg, statedb)

	// 实例化两个txpool
	var txpoolCfg legacypool.Config
	txpoolCfg = legacypool.DefaultConfig
	epool := legacypool.New(txpoolCfg, blockchain)
	etxpool, _ := txpool.New(new(big.Int).SetUint64(txpoolCfg.PriceLimit), blockchain, []txpool.SubPool{epool})
	defer etxpool.Close()

	ppool := legacypool.New(txpoolCfg, blockchain)
	ptxpool, _ := txpool.New(new(big.Int).SetUint64(txpoolCfg.PriceLimit), blockchain, []txpool.SubPool{ppool})
	defer ptxpool.Close()

	// 实例化共识客户端
	conn, _ := grpc.Dial("127.0.0.1:9080", grpc.WithTransportCredentials(insecure.NewCredentials()))
	p2pClient := pb.NewP2PClient(conn)

	// 实例化ExecutorService
	executorService := executor.NewExecutorService(etxpool, ptxpool, blockchain, p2pClient)

	return &ServiceContext{
		Config:          c,
		ExecutorService: executorService,
	}
}
