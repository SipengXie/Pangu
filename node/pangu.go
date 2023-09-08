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

var configFile = flag.String("f", "F:\\学习\\学术研究\\跨链项目\\Pangu\\node\\etc\\pangu.yaml", "the config file")

func main() {

	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()

	ctx := svc.NewServiceContext(c)
	handler.RegisterHandlers(server, ctx)

	fmt.Printf("Starting server at %s:%d...\n", c.Host, c.Port)
	server.Start()
}
