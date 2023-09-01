package pangu

import (
	"fmt"
	"os"

	"github.com/SipengXie/pangu/node/nodecfg"
	"github.com/SipengXie/pangu/turbo/app"
	pangucli "github.com/SipengXie/pangu/turbo/cli"
	"github.com/SipengXie/pangu/turbo/node"
	"github.com/ledgerwatch/log/v3"
	"github.com/urfave/cli/v2"
)

func main() {
	defer func() {
		panicRes := recover()
		if panicRes == nil {
			return
		}
		log.Error("catch panic", "err", panicRes, "stact")
		os.Exit(1)
	}()
	app := app.MakeApp("pangu", runPangu, pangucli.DefaultFlags)
	if err := app.Run(os.Args); err != nil {
		_, printErr := fmt.Fprintln(os.Stderr, err)
		if printErr != nil {
			log.Warn("Fprintln error", "err", printErr)
		}
		os.Exit(1)
	}
}

func runPangu(cliCtx *cli.Context) error {
	var logger log.Logger
	// TODO : 获取（创建）配置文件
	//
	var nodeConfig *nodecfg.Config

	// 获取盘古节点（包括node和backend）
	panguNode, err := node.New(nodeConfig, logger)
	if err != nil {
		log.Error("Pangu startup", "err", err)
		return err
	}

	// 启动节点服务
	err = panguNode.Serve()
	if err != nil {
		log.Error("error while serving an Pangu node", "err", err)
	}

	return err
}
