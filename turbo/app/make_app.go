package app

import (
	cli2 "github.com/SipengXie/pangu/turbo/cli"
	"github.com/urfave/cli/v2"
)

func MakeApp(name string, action cli.ActionFunc, cliFlags []cli.Flag) *cli.App {
	app := cli2.NewApp()
	app.Name = name
	app.Usage = name
	app.UsageText = app.Name + ` [command] [flags]`

	return app
}
