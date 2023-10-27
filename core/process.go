// 执行器 整体重构

package core

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"sync/atomic"

	"github.com/SipengXie/pangu/accesslist"
	"github.com/SipengXie/pangu/common"
	evmparams "github.com/SipengXie/pangu/core/evm/params"
	"github.com/SipengXie/pangu/crypto"
	"github.com/SipengXie/pangu/params"

	"github.com/SipengXie/pangu/core/evm"
	"github.com/SipengXie/pangu/core/state"
	"github.com/SipengXie/pangu/core/types"
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

// 解密函数
func DecTX(t types.GuarantorTX) []*types.Transaction {
	// 后续实现
	// 解密 EncContent []byte 字段为[]Transaction
	return nil
}

// 分组函数
func Classify(t []*types.Transaction) []types.Transactions {
	// 后续实现
	// 分组结果为二维数组
	return nil
}

// Process * 注意，该函数不是验证函数
// Process 执行函数，该函数作为第一次执行交易的函数，而不是验证函数；验证函数接收到的分组结果应该是Process函数中最后运行的结果
func (p *Processor) Process(block *types.Block, statedb *state.StateDB, cfg evm.Config) (*ProcessReturnMsg, error) {
	// 获取到的当前区块所有的交易序列（已分好组）
	TXS := block.Transactions2D()

	EvmContext := NewEVMBlockContext(block.Header(), p.blockchain, nil) // evm环境

	// 获取当前区块所有交易的加密数组
	EncGuarantorTX := block.GetGuaranteeTX()
	fmt.Println(EncGuarantorTX)

	// 解密所有交易 按理说是交易在执行前再解密，但是我们这里还不知道协议的设计，所以提前解密，后续再修改
	// HashToSig 定义为 map数据类型 hash - 担保人，不选择担保人交易的直接写交易发起者
	var HashToSig map[atomic.Value][]byte
	var DecGuarantorTX []*types.Transaction // 所有解密后的交易
	// 新建evm
	EachEvm := evm.NewEVM(EvmContext, evm.TxContext{}, statedb, new(evmparams.ChainConfig).FromGlobal(p.config), cfg)

	for i := 0; i < len(EncGuarantorTX); i++ {
		temp := DecTX(EncGuarantorTX[i]) // 当前循环处理的担保人交易组

		// 第一步 安全检查
		// ① 一个担保交易内的交易是否IsGua统一
		IsGua := temp[0].IsGuarantee()
		IsGuaResult := true
		for _, v := range temp {
			if v.IsGuarantee() != IsGua {
				fmt.Printf("\n%sERROR MSG%s   一个担保交易内的所有交易并不是同样的IsGuarantee值\n", types.FRED, types.FRESET)
				IsGuaResult = false
			}
			// // ② 当IsGuarantee = false时，GSig和用户签名是否一致，防止偷换 其实不用，因为交易是加密的，担保人就算拿走了也无法解密
			// GSig := EncGuarantorTX[i].GuarSig // 担保人签名
			// USig := v.RawSigValues()          // 用户签名
			// types.Sender()
		}
		if !IsGuaResult {
			// 第二步 记录所有交易
			// 出现错误，当前这组交易不能执行，只有当没有出现错误的时候，当前这组交易才能执行
			// DecGuarantorTX = append(DecGuarantorTX, temp...)
		}

		// 第五步 检查担保人是否有足够的余额支付汽油费
		GSig := EncGuarantorTX[i].GuarSig                                                // 担保人签名
		GR, GS, GV := types.DecodeSignature2(GSig)                                       // 担保人r,s,v
		GAddress, Gerr := types.RecoverPlain2(EncGuarantorTX[i].GHash, GR, GS, GV, true) // 担保人地址
		GBalance := EachEvm.StateDB.GetBalance(GAddress)                                 // 担保人账户余额
		if Gerr != nil {
			fmt.Printf("\n%sERROR MSG%s   从担保人签名求地址出现了错误，错误原因是 %v\n", types.FRED, types.FRESET, Gerr)
			return nil, nil
		}
		UAllCost := new(big.Int) // 执行担保人所有交易花费的最大汽油费
		for j := 0; j < len(temp); j++ {
			// 第四步 收取担保费

			USig := temp[j].RawSigValues() // 用户签名

			UR, US, UV := types.DecodeSignature2(USig) // 用户r,s,v

			UAddress, Uerr := types.RecoverPlain2(temp[j].Hash(), UR, US, UV, true) // 用户地址，可能存在问题，哈希值到底是哪个，是哪种格式
			if Uerr != nil {
				fmt.Printf("\n%sERROR MSG%s   从用户签名求地址出现了错误，错误原因是 %v\n", types.FRED, types.FRESET, Uerr)
				return nil, nil
			}
			GuaranteeFee := new(big.Int) // 担保费的计算方法待定
			if EachEvm.StateDB.GetBalance(UAddress).Cmp(GuaranteeFee) == -1 {
				fmt.Printf("\n%sERROR MSG%s   用户余额不足以支付担保费\n", types.FRED, types.FRESET)
				continue
			} else {
				// 新增交易
				DecGuarantorTX = append(DecGuarantorTX, temp[j])
				// 第三步 生成hash-sig对应关系map
				HashToSig[temp[j].GetTXHash()] = EncGuarantorTX[i].GuarSig // 赋值
			}
			EachEvm.StateDB.AddBalance(GAddress, GuaranteeFee) // 支付担保费
			EachEvm.StateDB.SubBalance(UAddress, GuaranteeFee) // 从用户地址扣除担保费，扣除的担保费与交给担保人的担保费金额一样

			// 计算最大花费汽油费，暂时不知道同时增加担保人余额会不会产生问题
			UAllCost.Add(UAllCost, new(big.Int).Mul(new(big.Int).SetUint64(temp[j].GasLimit()), temp[j].GasFeeCap()))

			if UAllCost.Cmp(GBalance) == 1 {
				fmt.Printf("\n%sERROR MSG%s   担保人余额不足以支付所有交易的费用\n", types.FRED, types.FRESET)
				return nil, nil
			}
		}
	}

	TXS = Classify(DecGuarantorTX) // 赋值

	var (
		Receipts         types.Receipts // 收据树
		UsedGas          = new(uint64)  // 记录交易实际花了多少汽油费
		Header           = block.Header()
		BlockHash        = block.Hash()
		BlockNumber      = block.Number()
		AllLogs          []*types.Log
		Signer           = types.MakeSigner(p.config, Header.Number, Header.Time) // 签名者
		GroupNum         = len(TXS)                                               // 组数
		PReturnMsg       = new(ProcessReturnMsg)                                  // 函数返回值
		ReturnChan1      = make(chan MessageReturn, GroupNum)                     // 并行组返回通道
		ReturnChan2      = make(chan MessageReturn, 1)                            // 串行组返回通道
		wg1              sync.WaitGroup                                           // 并行组等待组
		wg2              sync.WaitGroup                                           // 串行组等待组
		AllEvm           []*evm.EVM                                               // 存储所有线程的evm
		SerialTxList     []*types.Transaction                                     // 串行交易队列
		ErrorTxList      []*TxErrorMessage                                        // 执行出现错误的交易队列
		AccessListTxList []*TxAccessListMessage                                   // 串行队列AccessList不一致的交易
		// AllStateDB  []*state.StateDB                     // 存储所有线程的stateDB
	)

	// 为每个线程拷贝一份EVM
	for range TXS {
		// 现在的p.config是global，转为evm的config
		EachEvm := evm.NewEVM(EvmContext, evm.TxContext{}, statedb, new(evmparams.ChainConfig).FromGlobal(p.config), cfg)
		AllEvm = append(AllEvm, EachEvm)
		// 等待组+1
		wg1.Add(1)
	}

	fmt.Printf("\n%sSTAGE CHANGE%s   交易开始并行处理 <<< \n", types.FBLUE, types.FRESET) // ! 统一消息提示格式：英文概要大写   （空三格）中文描述具体内容 1 ERROR MSG 红色，错误提示信息；2 %sSTAGE CHANGE%s 蓝色，程序执行步骤阶段提示；3 PROMPT MSG 绿色，提示信息，用于一些小的信息提示

	for i, EachTXS := range TXS {
		EachThreadMessage := NewThreadMessage(p.config, BlockNumber, BlockHash, UsedGas, AllEvm[i], Signer, Header)
		go TxThread(i, EachTXS, &wg1, ReturnChan1, EachThreadMessage, true) // IsParallel = true 表示并行分组
	}

	wg1.Wait()

	fmt.Printf("\n%sSTAGE CHANGE%s   并行交易结果处理 <<< \n", types.FBLUE, types.FRESET)

	// 提交stateDB状态到内存
	for _, value := range AllEvm {
		value.StateDB.Finalise(true)
		spo, so := value.StateDB.GetPendingObj()
		statedb.SetPendingObj(spo)
		statedb.UpdateStateObj(so)
	}

	if len(ReturnChan1) == 0 {
		fmt.Printf("%sERROR MSG%s   并行线程的返回通道中没有返回值", types.FRED, types.FRESET)
		return PReturnMsg, errors.New("no Return Message in Parallel Channel")
	}
	for value := range ReturnChan1 {
		fmt.Printf("%sPROMPT MSG%s   收到一组并行交易的返回值，开始处理\n", types.FGREEN, types.FRESET)
		Receipts = append(Receipts, value.NewReceipt...)       // 收据树
		AllLogs = append(AllLogs, value.NewLogs...)            // 日志
		SerialTxList = append(SerialTxList, value.TxSerial...) // 串行队列
		ErrorTxList = append(ErrorTxList, value.TxError...)    // 执行出现错误的交易队列
		if len(ReturnChan1) == 0 {
			break
		}
	}

	// 开始执行交易串行队列
	if len(SerialTxList) != 0 {
		fmt.Printf("\n%sSTAGE CHANGE%s   开始执行串行队列交易 <<< \n", types.FBLUE, types.FRESET)
		fmt.Printf("%sPROMPT MSG%s   串行队列中存在 %d 笔交易\n", types.FGREEN, types.FRESET, len(SerialTxList))

		SortSerialTX(SerialTxList) // 串行队列排序
		EachEvm := evm.NewEVM(EvmContext, evm.TxContext{}, statedb, new(evmparams.ChainConfig).FromGlobal(p.config), cfg)
		EachThreadMessage := NewThreadMessage(p.config, BlockNumber, BlockHash, UsedGas, EachEvm, Signer, Header)

		// 执行串行交易
		wg2.Add(1)
		go TxThread(1, SerialTxList, &wg2, ReturnChan2, EachThreadMessage, false)
		wg2.Wait()

		fmt.Printf("\n%sSTAGE CHANGE%s   串行交易结果处理 <<< \n", types.FBLUE, types.FRESET)
		if len(ReturnChan2) == 0 {
			fmt.Printf("%sERROR MSG%s   串行线程的返回通道中没有返回值\n", types.FRED, types.FRESET)
			return PReturnMsg, errors.New("no Return Message in Serial Channel")
		}

		value := <-ReturnChan2
		Receipts = append(Receipts, value.NewReceipt...)
		AllLogs = append(AllLogs, value.NewLogs...)
		ErrorTxList = append(ErrorTxList, value.TxError...)
		AccessListTxList = append(AccessListTxList, value.TxAccessList...)
		EachEvm.StateDB.Finalise(true)
	} else {
		fmt.Printf("\n%sSTAGE CHANGE%s   串行交易队列长度为零，不需要执行串行队列交易 <<< \n", types.FBLUE, types.FRESET)
	}

	// stateDB commit
	RootHash, err := statedb.Commit(true) // ? BlockHash 放在哪里？
	fmt.Printf("%sPROMPT MSG%s   RootHash = %v\n", types.FGREEN, types.FRESET, RootHash)
	if err != nil {
		fmt.Printf("%sERROR MSG%s   Commit函数出错\n", types.FRED, types.FRESET)
		return PReturnMsg, errors.New("commit函数出错")
	}

	fmt.Printf("\n%sSTAGE CHANGE%s   Process函数执行完成 <<< \n", types.FBLUE, types.FRESET)
	PReturnMsg = NewProcessReturnMsg(Receipts, AllLogs, ErrorTxList, AccessListTxList, UsedGas, RootHash) // Process函数返回值
	return PReturnMsg, nil
}

// TxThread 线程池执行交易，新增参数 IsParallel bool 表明当前交易队列是否是并行 true -> 并行，false -> 串行
func TxThread(id int, txs []*types.Transaction, wg *sync.WaitGroup, msgReturn chan MessageReturn, trMessage *ThreadMessage, IsParallel bool) {
	if IsParallel {
		fmt.Printf("%sPROMPT MSG%s   当前第 %d 组并行线程中总共需要执行 %d 笔交易\n", types.FGREEN, types.FRESET, id, len(txs))
	} else {
		fmt.Printf("%sPROMPT MSG%s   当前串行线程中总共需要执行 %d 笔交易\n", types.FGREEN, types.FRESET, len(txs))
	}

	defer wg.Done()

	var (
		ThreadReceipt  []*types.Receipt       // 线程执行的收据树
		ThreadLogs     []*types.Log           // 线程执行的log
		ThreadSerialTx []*types.Transaction   // 线程执行的串行队列
		ErrReturnMsg   []*TxErrorMessage      // 返回值
		TxAccessList   []*TxAccessListMessage // 需要更改AccessList的交易
	)

	// 开始执行交易
	for i := 0; i < len(txs); i++ {
		// 每次循环处理的交易
		tx := txs[i]
		// 新建执行交易的信息结构体
		msg, err := TransactionToMessage(tx, trMessage.Signer, trMessage.Header.BaseFee, IsParallel)
		if err != nil {
			EachErrMsg := NewTxErrorMessage(tx, "function TransactionToMessage err", err)
			ErrReturnMsg = append(ErrReturnMsg, EachErrMsg) // 将错误交易信息保存到返回值中
			fmt.Printf("%sERROR MSG%s   交易执行出错 in TransactionToMessage function\n", types.FRED, types.FRESET)
			continue
		}
		// 执行交易
		Receipt, TrueAccessList, err := ExecuteTx(msg, trMessage.BlockNumber, trMessage.BlockHash, tx, trMessage.UsedGas, trMessage.EVMenv)

		// 错误处理
		if err != nil {
			EachErrMsg := NewTxErrorMessage(tx, "function ExecuteTx err", err)
			ErrReturnMsg = append(ErrReturnMsg, EachErrMsg) // 将错误交易信息保存到返回值中
			continue
		}
		if Receipt == nil {
			EachErrMsg := NewTxErrorMessage(tx, "TX can not execute in parallel thread", nil)
			ErrReturnMsg = append(ErrReturnMsg, EachErrMsg) // 将错误交易信息保存到返回值中

			// 将交易放到串行组中
			ThreadSerialTx = append(ThreadSerialTx, tx)
			// 将同一Address的交易全部去除
			tempAddress, _ := trMessage.Signer.Sender(tx) // 获取交易发送地址
			index := i + 1
			for ; index < len(txs); index++ {
				tempAddressNext, _ := trMessage.Signer.Sender(txs[index])
				if bytes.Compare(tempAddress[:], tempAddressNext[:]) == 0 {
					fmt.Printf("%sPROMPT MSG%s   因当前交易无法并行执行，将一个地址相同的交易也放到串行交易数组\n", types.FGREEN, types.FRESET)
					ThreadSerialTx = append(ThreadSerialTx, txs[index])
				} else {
					break
				}
			}
			txs = txs[index:]
			i = -1

			continue
		}

		// 交易没有错误
		fmt.Printf("%sPROMPT MSG%s   恭喜您，一笔交易在并行组中成功执行\n", types.FGREEN, types.FRESET)
		ThreadReceipt = append(ThreadReceipt, Receipt)
		ThreadLogs = append(ThreadLogs, Receipt.Logs...)
		if !msg.IsParallel {
			TxAccessList = append(TxAccessList, &TxAccessListMessage{
				Tx:             tx,
				TrueAccessList: TrueAccessList,
			})
		}
	}

	// 汇总最后的返回信息
	messageReturn := MessageReturn{
		NewReceipt:   ThreadReceipt,
		NewLogs:      ThreadLogs,
		TxSerial:     ThreadSerialTx,
		TxError:      ErrReturnMsg,
		TxAccessList: TxAccessList,
	}
	msgReturn <- messageReturn
}

// ExecuteTx 交易执行入口函数，代替原applyTransaction函数
func ExecuteTx(msg *TxMessage, blockNumber *big.Int, blockHash common.Hash, tx *types.Transaction, usedGas *uint64, evm *evm.EVM) (*types.Receipt, *accesslist.AccessList, error) {
	EvmTxContext := NewEVMTxContext(msg)
	evm.TxContext = EvmTxContext

	// 设置交易哈希
	evm.StateDB.SetTxContext(tx.Hash())

	// 执行交易
	executionResult := ApplyTransaction(msg, evm) // err是交易执行外发生的错误

	// 错误处理
	if executionResult.IsParallelError {
		fmt.Printf("%sERROR MSG%s   当前交易无法在并行队列中并行执行\n", types.FRED, types.FRESET)
		return nil, nil, nil
	}
	//if executionResult == nil && err != nil {
	//	fmt.Printf("%sERROR MSG%s   当前交易在执行前的检查阶段发生错误\n", types.FRED, types.FRESET)
	//	return nil, err
	//}
	if executionResult.Err != nil {
		fmt.Printf("%sERROR MSG%s   当前交易在执行中发生错误\n", types.FRED, types.FRESET)
		return nil, nil, executionResult.Err
	}

	*usedGas += executionResult.UsedGas

	var root []byte
	receipt := &types.Receipt{Type: tx.Type(), PostState: root, CumulativeGasUsed: *usedGas}
	if executionResult.Err != nil {
		receipt.Status = types.ReceiptStatusFailed
	} else {
		receipt.Status = types.ReceiptStatusSuccessful
	}
	receipt.TxHash = tx.Hash()
	receipt.GasUsed = executionResult.UsedGas

	if msg.To == nil {
		receipt.ContractAddress = crypto.CreateAddress(evm.TxContext.Origin, tx.Nonce())
	}

	receipt.Logs = evm.StateDB.GetLogs(tx.Hash(), blockNumber.Uint64(), blockHash)
	receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
	receipt.BlockHash = blockHash
	receipt.BlockNumber = blockNumber
	return receipt, executionResult.TrueAccessList, nil
}
