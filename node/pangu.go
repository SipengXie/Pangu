package main

import (
	"flag"
	"fmt"

	"github.com/SipengXie/pangu/node/internal/config"
	"github.com/SipengXie/pangu/node/internal/handler"
	"github.com/SipengXie/pangu/node/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"
)

var configFile = flag.String("f", "/home/xiaowk/Project/Pangu/node/etc/pangu.yaml", "the config file")

func main() {

	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()

	ctx := svc.NewServiceContext(c)
	// 关掉ExecutorService的两个pool
	defer ctx.ExecutorService.Stop()
	// TODO: 关掉数据库

	handler.RegisterHandlers(server, ctx)

	fmt.Printf("Starting server at %s:%d...\n", c.Host, c.Port)
	server.Start()
}
