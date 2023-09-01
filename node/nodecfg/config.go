package nodecfg

import (
	"github.com/c2h5oh/datasize"
	"github.com/ledgerwatch/erigon-lib/common/datadir"
	"github.com/ledgerwatch/erigon-lib/kv"
)

type Config struct {
	Name string `toml:"-"`

	Dirs datadir.Dirs

	DatabaseVerbosity kv.DBVerbosityLvl
	// TODO : 与网络区对接

	// P2P
	// Http httpcfg.HttpCfg
	MdbxPageSize    datasize.ByteSize
	MdbxDBSizeLimit datasize.ByteSize
	MdbxGrowthStep  datasize.ByteSize
}
