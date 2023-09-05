package handler

import (
	"net/http"

	"github.com/SipengXie/pangu/node/internal/logic"
	"github.com/SipengXie/pangu/node/internal/svc"
	"github.com/SipengXie/pangu/node/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func sendTransactionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.TransactionArgs
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := logic.NewSendTransactionLogic(r.Context(), svcCtx)
		resp, err := l.SendTransaction(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
