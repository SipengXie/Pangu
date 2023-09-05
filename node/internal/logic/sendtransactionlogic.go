package logic

import (
	"context"

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
	var tx tp.Transaction
	return &tx
}

func (l *SendTransactionLogic) SendTransaction(req *types.TransactionArgs) (resp *types.BoolRes, err error) {
	// 将TransactionArgs转换成真正的Transaction
	tx := ToTransaction(req)
	// 将交易添加到交易池中（调用txpool.Add方法)
	err = l.svcCtx.ExecutorService.AddTx(tx)
	if err != nil {
		return nil, err
	}
	return &types.BoolRes{Flag: true}, nil
}
