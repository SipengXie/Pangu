package node

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/SipengXie/pangu/rpc/rpccfg"
	"github.com/ledgerwatch/log/v3"
)

// StartHTTPEndpoint starts the HTTP RPC endpoint.
func StartHTTPEndpoint(endpoint string, timeouts rpccfg.HTTPTimeouts, handler http.Handler) (*http.Server, net.Addr, error) {
	// start the HTTP listener
	var (
		listener net.Listener
		err      error
	)

	if listener, err = net.Listen("tcp", endpoint); err != nil {
		return nil, nil, err
	}
	// 启动Http服务
	httpSrv := &http.Server{
		Handler:           handler,
		ReadTimeout:       timeouts.ReadTimeout,
		WriteTimeout:      timeouts.WriteTimeout,
		IdleTimeout:       timeouts.IdleTimeout,
		ReadHeaderTimeout: timeouts.ReadTimeout,
	}
	go func() {
		serveErr := httpSrv.Serve(listener)
		if serveErr != nil &&
			!(errors.Is(serveErr, context.Canceled) || errors.Is(serveErr, errors.New("stopped")) || errors.Is(serveErr, http.ErrServerClosed)) {
			log.Warn("Failed to serve http endpoint", "err", serveErr)
		}
	}()
	return httpSrv, listener.Addr(), err
}

// CheckTimeouts ensures that timeout values are meaningful
func CheckTimeouts(timeouts *rpccfg.HTTPTimeouts) {
	if timeouts.ReadTimeout < time.Second {
		log.Warn("Sanitizing invalid HTTP read timeout", "provided", timeouts.ReadTimeout, "updated", rpccfg.DefaultHTTPTimeouts.ReadTimeout)
		timeouts.ReadTimeout = rpccfg.DefaultHTTPTimeouts.ReadTimeout
	}
	if timeouts.WriteTimeout < time.Second {
		log.Warn("Sanitizing invalid HTTP write timeout", "provided", timeouts.WriteTimeout, "updated", rpccfg.DefaultHTTPTimeouts.WriteTimeout)
		timeouts.WriteTimeout = rpccfg.DefaultHTTPTimeouts.WriteTimeout
	}
	if timeouts.IdleTimeout < time.Second {
		log.Warn("Sanitizing invalid HTTP idle timeout", "provided", timeouts.IdleTimeout, "updated", rpccfg.DefaultHTTPTimeouts.IdleTimeout)
		timeouts.IdleTimeout = rpccfg.DefaultHTTPTimeouts.IdleTimeout
	}
}
