package logic

import (
	"context"
	"log"
	"math/big"

	"github.com/SipengXie/pangu/accesslist"
	"github.com/SipengXie/pangu/common"
	tp "github.com/SipengXie/pangu/core/types"
	"github.com/SipengXie/pangu/node/internal/svc"
	"github.com/SipengXie/pangu/node/internal/types"
	"github.com/zeromicro/go-zero/core/logx"
)

type SendTransactionLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewSendTransactionLogic(ctx context.Context, svcCtx *svc.ServiceContext) *SendTransactionLogic {
	return &SendTransactionLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// ToTransaction 将 TxArgs 转换成 transaction
func ToTransaction(args *types.TransactionArgs) *tp.Transaction {
	var data tp.TxData
	al := accesslist.NewAccessList()
	if len(args.AccessList) != 0 {
		// 暂定AccessList以Json字节数据的hexString传入
		// 解析AccessList填入
		if err := al.Deserialize(common.Hex2Bytes(args.AccessList)); err != nil {
			log.Fatal(err)
		}
	}

	to := new(common.Address)
	to.SetBytes(common.FromHex(args.To))

	chainid := new(big.Int)
	chainid.SetString(args.ChainID, 10)

	feecap := new(big.Int)
	feecap.SetString(args.MaxFeePerGas, 10)

	tipcap := new(big.Int)
	tipcap.SetString(args.MaxPriorityFeePerGas, 10)

	value := new(big.Int)
	value.SetString(args.Value, 10)

	var txdata []byte
	if len(args.Input) != 0 {
		txdata = make([]byte, len(common.Hex2Bytes(args.Input)))
		copy(txdata, common.Hex2Bytes(args.Input))
	}
	if len(args.Data) != 0 {
		txdata = make([]byte, len(common.Hex2Bytes(args.Data)))
		copy(txdata, common.Hex2Bytes(args.Data))
	}

	// TODO:上层封装签名逻辑

	data = &tp.PanguTransaction{
		To:         to,
		ChainID:    chainid,
		Nonce:      args.Nonce,
		GasLimit:   args.Gas,
		FeeCap:     feecap,
		TipCap:     tipcap,
		Value:      value,
		Data:       txdata,
		SigAlgo:    args.SigAlgo,
		Signature:  common.Hex2Bytes(args.Signature),
		AccessList: al,
	}
	return tp.NewTx(data)
}

func (l *SendTransactionLogic) SendTransaction(req *types.TransactionArgs) (resp *types.BoolRes, err error) {
	// 将TransactionArgs转换成真正的Transaction
	tx := ToTransaction(req)
	// 将交易添加到交易池中（调用txpool.Add方法)
	err = l.svcCtx.ExecutorService.AddTxToPendingPool(tx)
	if err != nil {
		return nil, err
	}
	return &types.BoolRes{Flag: true}, nil
}
