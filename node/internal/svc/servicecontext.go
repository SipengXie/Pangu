package svc

import (
	"github.com/SipengXie/pangu/core/rawdb"
	"github.com/SipengXie/pangu/executor"
	"github.com/SipengXie/pangu/node/internal/config"
)

var (
	db = rawdb.NewMemoryDatabase()
)

type ServiceContext struct {
	Config          config.Config
	ExecutorService executor.ExecutorService
}

func NewServiceContext(c config.Config) *ServiceContext {
	// TODO : 工程上需要进一步构筑blockchain的逻辑
	// 默认起好了一条链
	//statedb, _ := state.New(types.EmptyRootHash, state.NewDatabase(db), nil)
	//
	// TODO:实例化ExecutorService
	var executorService executor.ExecutorService
	return &ServiceContext{
		Config:          c,
		ExecutorService: executorService,
	}
}
