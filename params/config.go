package params

import "math/big"

type ChainConfig struct {
	ChainID *big.Int `json:"chainId"` // chainId identifies the current chain and is used for replay protection
}
type Rules struct {
}

func (c *ChainConfig) Rules(num *big.Int, isMerge bool, timestamp uint64) Rules {
	return Rules{}
}

var (
	AllEthashProtocolChanges = &ChainConfig{
		ChainID: big.NewInt(1337), // TODO: 很多都舍弃了
	}

	TestChainConfig = &ChainConfig{
		ChainID: big.NewInt(1337),
	}
)
