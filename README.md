## Data Strcuture Design

### Transaction

```golang

type Transaction interface {
	Type() byte
	GetChainID() *big.Int
	Time() time.Time
    
    GetSender() (common.Address, bool)
	GetNonce() uint64
    GetGas() uint64
	GetPrice() *big.Int
    GetValue() *big.Int
    GetSigAlgo() byte
	GetRawSignature() []byte

    GetEncryptionAlgo() byte
    GetRawEncryptedContent() []byte
	GetTo() *common.Address
    GetData() []byte
	GetAccessList() AccessList

	// AsMessage(s Signer, baseFee *big.Int, rules *chain.Rules) (Message, error)

	Hash() common.Hash
	SigningHash(chainID *big.Int) common.Hash
    Cost() *big.Int

	EncodingSize() int
	EncodeRLP(w io.Writer) error
	MarshalBinary(w io.Writer) error
}

type TransactionMisc struct {
    hash atomic.Value
	size atomic.Value
	from atomic.Value
}

type PanguTransaction struct {
    TransactionMisc
    TxCoverMisc
    
    Nonce       uint64
    Value       *big.Int
    Gas         uint64
    GasPrice    *big.Int

    SigAlgo     byte
    Signature   []byte
    // Sender      // 用户
    // Ensurance   // 担保人

    TxContent
    // ↑
    // ↓
    EncryptionAlgo  byte
    EncryptedContent []byte

    // Payload []byte // 序列化的PanguTx(作为一个囊括其他交易的容器)，可能有多个交易

}

type TxCoverMisc struct {
    Type byte
    ChainId *big.Int
}


type TxContent struct {
    VmType      byte
    To          *common.Address
    Data        []byte
    AccessList  AccessList
}
```

### Block

```golang
type Header struct {
	ParentHash  common.Hash    `json:"parentHash"       gencodec:"required"`
	Coinbase    common.Address `json:"miner"`
	StateRoot   common.Hash    `json:"stateRoot"        gencodec:"required"`
	TxRoot      common.Hash    `json:"transactionsRoot" gencodec:"required"` // Merkkel
	ReceiptRoot common.Hash    `json:"receiptsRoot"     gencodec:"required"` // Merkkel
	Bloom       Bloom          `json:"logsBloom"        gencodec:"required"`
	Number      *big.Int       `json:"number"           gencodec:"required"`
	GasLimit    uint64         `json:"gasLimit"         gencodec:"required"`
	GasUsed     uint64         `json:"gasUsed"          gencodec:"required"`
	Time        uint64         `json:"timestamp"        gencodec:"required"`
	Extra       []byte         `json:"extraData"        gencodec:"required"`
	// MixDigest   common.Hash    `json:"mixHash"`
}

type Body struct {
    []transactions
}

type Block struct {
    Header
    Body

    hash atomic.Value
    size atomic.Value
}
```

### Blockchain

```golang
type Blockchain interface {
    Config() *params.ChainConfig

	// CurrentBlock returns the current head of the chain.
	CurrentBlock() *types.Header

	// GetBlock retrieves a specific block, used during pool resets.
	GetBlock(hash common.Hash, number uint64) *types.Block

	// StateAt returns a state database for a given root hash (generally the head).
	StateAt(root common.Hash) (*state.StateDB, error)

    WriteBlockAndSetHead(block *types.Block, receipts []*types.Receipt, logs []*types.Log, state *state.StateDB, emitHeadEvent bool) (status WriteStatus, err error) 
}
```