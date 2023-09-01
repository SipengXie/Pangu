package utils

var (
	// General settings
	DataDirFlag = flags.DirectoryFlag{
		Name:  "datadir",
		Usage: "Data directory for the databases",
		Value: flags.DirectoryString(paths.DefaultDataDir()),
	}
)
