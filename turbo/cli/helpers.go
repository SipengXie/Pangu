package cli

import (
	"github.com/urfave/cli/v2"
)

const VERSION = "0.1.0"

// NewApp creates an app with sane defaults.
func NewApp() *cli.App {
	app := cli.NewApp()
	app.Version = VERSION
	return app
}
