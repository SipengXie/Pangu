// 执行器 整体重构

package core

import (
	"fmt"
	"sync"

	"github.com/SipengXie/pangu/core/evm"
	"github.com/SipengXie/pangu/core/state"
	"github.com/SipengXie/pangu/core/types"
	"github.com/SipengXie/pangu/params"
)

// Processor 执行器
type Processor struct {
	config     *params.ChainConfig // 链配置
	blockchain *Blockchain         // 区块链
}

// NewStateProcessor 初始化一个交易执行器
func NewStateProcessor(config *params.ChainConfig, bc *Blockchain) *Processor {
	return &Processor{
		config:     config,
		blockchain: bc,
	}
}

// Process * 注意，该函数不是验证函数
// Process 执行函数，该函数作为第一次执行交易的函数，而不是验证函数；验证函数接收到的分组结果应该是Process函数中最后运行的结果
func (p *Processor) Process(block *types.Block, statedb *state.StateDB, cfg evm.Config) (types.Receipts, []*types.Log, uint64, error) {
	// 获取到的当前区块所有的交易序列（已分好组）
	TXS := block.Transactions()

	var (
		Receipts    types.Receipts // 收据树
		UsedGas     = new(uint64)  // 记录交易实际花了多少汽油费
		Header      = block.Header()
		BlockHash   = block.Hash()
		BlockNumber = block.Number()
		AllLogs     []*types.Log
		EvmContext  = NewEVMBlockContext(Header, p.blockchain, nil) // evm环境
		// vmenv   = vm.NewEVM(context, vm.TxContext{}, statedb, p.config, cfg) // ! ERROR
		Signer   = types.MakeSigner(p.config, Header.Number, Header.Time)
		GroupNum = len(TXS) // 组数
	)

	// 调用分组函数
	// txs := ClassifyTx(allTX, signer) // TODO: 如果传来的是二维数组，则说明在传入之前已经调用了分组函数

	var (
		ReturnChan1 = make(chan MessageReturn, GroupNum) // 并行组返回通道
		ReturnChan2 = make(chan MessageReturn, 1)        // 串行组返回通道
		wg1         sync.WaitGroup                       // 并行组等待组
		wg2         sync.WaitGroup                       // 串行组等待组
		AllEvm      []*evm.EVM                           // 存储所有线程的evm
		// AllStateDB  []*state.StateDB                     // 存储所有线程的stateDB
	)

	// 为每个线程拷贝一份EVM
	for range TXS {
		EachEvm := evm.NewEVM(EvmContext, evm.TxContext{}, statedb, p.config, cfg)
		AllEvm = append(AllEvm, EachEvm)
		// 等待组+1
		wg1.Add(1)
	}

	fmt.Printf("STAGE CHANGE   MSG 交易开始并行处理 << \n")

	for i, EachTXS := range TXS {
		EachThreadMessage := NewThreadMessage(p.config, BlockNumber, BlockHash, UsedGas, AllEvm[i], Signer, Header)
		go TxThread(i, EachTXS, &wg1, ReturnChan1, EachThreadMessage, true) // IsParallel = true 表示并行分组
	}

	return nil, nil, 0, nil
}

// TxThread 线程池执行交易，新增参数 IsParallel bool 表明当前交易队列是否是并行 true -> 并行，false -> 串行
func TxThread(id int, txs []*types.Transaction, wg *sync.WaitGroup, msgReturn chan MessageReturn, trMessage *ThreadMessage, IsParallel bool) {
	if IsParallel {
		fmt.Printf("PROMPT MSG 当前第 %d 组并行线程中总共需要执行 %d 笔交易\n", id, len(txs))
	} else {
		fmt.Printf("PROMPT MSG 当前串行线程中总共需要执行 %d 笔交易\n", len(txs))
	}

	defer wg.Done()
}
