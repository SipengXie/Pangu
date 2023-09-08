package svc

import (
	"fmt"
	"math/big"
	"net"

	"github.com/SipengXie/pangu/common"
	"github.com/SipengXie/pangu/crypto"

	"github.com/SipengXie/pangu/core"
	"github.com/SipengXie/pangu/core/evm"
	"github.com/SipengXie/pangu/core/rawdb"
	"github.com/SipengXie/pangu/core/state"
	"github.com/SipengXie/pangu/core/txpool"
	"github.com/SipengXie/pangu/core/txpool/legacypool"
	"github.com/SipengXie/pangu/core/types"
	"github.com/SipengXie/pangu/executor"
	"github.com/SipengXie/pangu/node/internal/config"
	"github.com/SipengXie/pangu/params"
	"github.com/SipengXie/pangu/pb"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/grpclog"
)

var (
	db         = rawdb.NewMemoryDatabase()
	BankKeyHex = "c3914129fade8d775d22202702690a8a0dcb178040bcb232a950c65b84308828"
)

type ServiceContext struct {
	Config          config.Config
	ExecutorService *executor.ExecutorService
}

func NewServiceContext(c config.Config) *ServiceContext {
	// TODO : 工程上需要进一步构筑blockchain的逻辑
	// 默认起好了一条链
	statedb, _ := state.New(types.EmptyRootHash, state.NewDatabase(db), nil)
	// 初始化一个账户
	BankAKey, _ := crypto.ToECDSA(common.Hex2Bytes(BankKeyHex))
	BankAddress := crypto.PubkeyToAddress(BankAKey.PublicKey)
	statedb.SetBalance(BankAddress, big.NewInt(9000000000000000000))

	// 模拟一个区块链
	chainCfg := &params.ChainConfig{
		ChainID: big.NewInt(1337),
	}
	blockchain := core.NewBlokchain(chainCfg, statedb, evm.Config{})

	// 实例化两个txpool
	var txpoolCfg legacypool.Config
	txpoolCfg = legacypool.DefaultConfig
	epool := legacypool.New(txpoolCfg, blockchain)
	etxpool, _ := txpool.New(new(big.Int).SetUint64(txpoolCfg.PriceLimit), blockchain, []txpool.SubPool{epool})
	// defer etxpool.Close()

	ppool := legacypool.New(txpoolCfg, blockchain)
	ptxpool, _ := txpool.New(new(big.Int).SetUint64(txpoolCfg.PriceLimit), blockchain, []txpool.SubPool{ppool})
	// defer ptxpool.Close()

	// 实例化共识客户端
	conn, err := grpc.Dial("127.0.0.1:9080", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		fmt.Println("Failed to dial gRPC server: %v", err)
	}
	p2pClient := pb.NewP2PClient(conn)

	// 实例化ExecutorService
	executorService := executor.NewExecutorService(etxpool, ptxpool, blockchain, p2pClient)
	fmt.Println("creat executor Service")

	listen, _ := net.Listen("tcp", "127.0.0.1:9876")
	s := grpc.NewServer()
	pb.RegisterExecutorServer(s, executorService)
	fmt.Println("Listen on " + "127.0.0.1:9876")
	grpclog.Println("Listen on " + "127.0.0.1:9876")

	go s.Serve(listen)

	return &ServiceContext{
		Config:          c,
		ExecutorService: executorService,
	}
}
