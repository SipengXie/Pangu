// 执行器 整体重构

package core

import (
	"github.com/SipengXie/pangu/core/evm"
	"github.com/SipengXie/pangu/core/types"
	"github.com/SipengXie/pangu/params"
)

// Processor 执行器
type Processor struct {
	config     *params.ChainConfig // 链配置
	blockchain *types.Blockchain   // 区块链
}

// NewStateProcessor 初始化一个交易执行器
func NewStateProcessor(config *params.ChainConfig, bc *types.Blockchain) *Processor {
	return &Processor{
		config:     config,
		blockchain: bc,
	}
}

// Process 执行函数
func (p *Processor) Process(block *types.Block, statedb *state.StateDB, cfg evm.Config) (types.Receipts, []*types.Log, uint64, error) {
	var (
		receipts    types.Receipts // 收据树
		usedGas     = new(uint64)  // 记录交易实际花了多少汽油费
		header      = block.Header()
		blockHash   = block.Hash()
		blockNumber = block.Number()
		allLogs     []*types.Log
		evmcontext  = NewEVMBlockContext(header, p.blockchain, nil) // evm环境
		// vmenv   = vm.NewEVM(context, vm.TxContext{}, statedb, p.config, cfg) // ! ERROR
		signer = types.MakeSigner(p.config, header.Number, header.Time)
	)

	// 获取到的当前区块所有的交易序列
	allTX := block.Transactions() // allTx好像是二维数组？

	// 调用分组函数
	txs := ClassifyTx(allTX, signer)

	return nil, nil, 0, nil
}
