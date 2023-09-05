package params

import (
	"math/big"

	global "github.com/SipengXie/pangu/params"
)

type ChainConfig struct {
	ChainID *big.Int `json:"chainId"` // chainId identifies the current chain and is used for replay protection
}

func (c *ChainConfig) Rules(num *big.Int, timestamp uint64) Rules {
	return Rules{}
}

func (cfg *ChainConfig) FromGlobal(gcfg global.ChainConfig) {
	cfg.ChainID = new(big.Int).Set(gcfg.ChainID)
}

func (cfg *ChainConfig) ToGlobal() global.ChainConfig {
	return global.ChainConfig{}
}

// Rules 暂时将所有执行用到这个地方的判断全部删除了
type Rules struct {
}

var (
	AllEthashProtocolChanges = &ChainConfig{
		ChainID: big.NewInt(1337), // TODO: 很多都舍弃了
	}

	TestChainConfig = &ChainConfig{
		ChainID: big.NewInt(1337),
	}
)
