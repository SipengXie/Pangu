package params

import "math/big"

type ChainConfig struct {
	ChainID    *big.Int `json:"chainId"` // chainId identifies the current chain and is used for replay protection
	chainRules Rules    // 暂时定义一个空的
}

// Rules 暂时将所有执行用到这个地方的判断全部删除了
type Rules struct {
}

// Rules ensures c's ChainID is not nil.
func (c *ChainConfig) Rules(num *big.Int, isMerge bool, timestamp uint64) Rules {
	return Rules{}
}

// evm测试文件中用到的变量，临时定义
var (
	// AllEthashProtocolChanges contains every protocol change (EIPs) introduced
	// and accepted by the Ethereum core developers into the Ethash consensus.
	AllEthashProtocolChanges = &ChainConfig{
		ChainID: big.NewInt(1337), // TODO: 很多都舍弃了
	}

	// TestChainConfig contains every protocol change (EIPs) introduced
	// and accepted by the Ethereum core developers for testing proposes.
	TestChainConfig = &ChainConfig{
		ChainID: big.NewInt(1337),
	}
)
