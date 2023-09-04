package types

import (
	"fmt"
	"io"
	"math/big"
	"sync/atomic"

	"github.com/SipengXie/pangu/common"
	"github.com/SipengXie/pangu/rlp"
)

type Header struct {
	ParentHash common.Hash `json:"parentHash"       gencodec:"required"`
	Time       uint64      `json:"timestamp"        gencodec:"required"`
	Number     *big.Int    `json:"number"           gencodec:"required"`

	Coinbase    common.Address `json:"miner"`
	StateRoot   common.Hash    `json:"stateRoot"        gencodec:"required"`
	TxRoot      common.Hash    `json:"transactionsRoot" gencodec:"required"` // Merkel
	ReceiptRoot common.Hash    `json:"receiptsRoot"     gencodec:"required"` // Merkel
	Bloom       Bloom          `json:"logsBloom"        gencodec:"required"`

	BaseFee  *big.Int `json:"baseFeePerGas"    rlp:"optional"`
	GasLimit uint64   `json:"gasLimit"         gencodec:"required"`
	GasUsed  uint64   `json:"gasUsed"          gencodec:"required"`

	Extra []byte `json:"extraData"        gencodec:"required"` // this field is used for extension
}

// Hash returns the block hash of the header, which is simply the keccak256 hash of its
// RLP encoding.
func (h *Header) Hash() common.Hash {
	return rlpHash(h)
}

// SanityCheck checks a few basic things -- these checks are way beyond what
// any 'sane' production values should hold, and can mainly be used to prevent
// that the unbounded fields are stuffed with junk data to add processing
// overhead
func (h *Header) SanityCheck() error {
	if h.Number != nil && !h.Number.IsUint64() {
		return fmt.Errorf("too large block number: bitlen %d", h.Number.BitLen())
	}
	if eLen := len(h.Extra); eLen > 100*1024 {
		return fmt.Errorf("too large block extradata: size %d", eLen)
	}
	if h.BaseFee != nil {
		if bfLen := h.BaseFee.BitLen(); bfLen > 256 {
			return fmt.Errorf("too large base fee: bitlen %d", bfLen)
		}
	}
	return nil
}

// CopyHeader creates a deep copy of a block header to prevent side effects from
// modifying a header variable.
func CopyHeader(h *Header) *Header {
	cpy := *h
	if cpy.Number = new(big.Int); h.Number != nil {
		cpy.Number.Set(h.Number)
	}
	if h.BaseFee != nil {
		cpy.BaseFee = new(big.Int).Set(h.BaseFee)
	}
	if len(h.Extra) > 0 {
		cpy.Extra = make([]byte, len(h.Extra))
		copy(cpy.Extra, h.Extra)
	}
	return &cpy
}

type Body struct {
	// Grouped Txs for parallel execution
	transactions []Transactions
}

type Block struct {
	header *Header
	Body

	hash atomic.Value // for SPV, this value should be returned
	size atomic.Value
}

// used for RLP encoding/decoding
type extblock struct {
	Header *Header
	Txs    []Transactions
}

// NewBlock creates a new block. The input data is copied,
// changes to header and to the field values will not affect the
// block.
//
// The values of TxHash, UncleHash, ReceiptHash and Bloom in header
// are ignored and set to values derived from the given txs, uncles
// and receipts.
func NewBlock(header *Header, txs []Transactions, receipts []*Receipt, hasher TrieHasher) *Block {
	b := &Block{header: CopyHeader(header)}

	// TODO: panic if len(txs) != len(receipts)
	if len(txs) == 0 {
		b.header.TxRoot = EmptyTxsHash
	} else {
		// !Txs是二维的，这里转成一维
		txList := new(Transactions)
		for _, tx := range txs {
			*txList = append(*txList, tx...)
		}
		b.header.TxRoot = DeriveSha(txList, hasher)

		b.transactions = make([]Transactions, len(txs))
		copy(b.transactions, txs)
	}

	if len(receipts) == 0 {
		b.header.ReceiptRoot = EmptyReceiptsHash
	} else {
		b.header.ReceiptRoot = DeriveSha(Receipts(receipts), hasher)
		b.header.Bloom = CreateBloom(receipts)
	}

	return b
}

// DecodeRLP decodes the Ethereum
func (b *Block) DecodeRLP(s *rlp.Stream) error {
	var eb extblock
	_, size, _ := s.Kind()
	if err := s.Decode(&eb); err != nil {
		return err
	}
	b.header, b.transactions = eb.Header, eb.Txs
	b.size.Store(rlp.ListSize(size))
	return nil
}

// EncodeRLP serializes b into the Ethereum RLP block format.
func (b *Block) EncodeRLP(w io.Writer) error {
	return rlp.Encode(w, extblock{
		Header: b.header,
		Txs:    b.transactions,
	})
}

func (b *Block) Transactions() []Transactions { return b.transactions }

func (b *Block) Transaction(hash common.Hash) *Transaction {
	for _, txs := range b.transactions {
		for _, tx := range txs {
			if tx.Hash() == hash {
				return tx
			}
		}
	}
	return nil
}

func (b *Block) Number() *big.Int         { return new(big.Int).Set(b.header.Number) }
func (b *Block) GasLimit() uint64         { return b.header.GasLimit }
func (b *Block) GasUsed() uint64          { return b.header.GasUsed }
func (b *Block) Time() uint64             { return b.header.Time }
func (b *Block) NumberU64() uint64        { return b.header.Number.Uint64() }
func (b *Block) Bloom() Bloom             { return b.header.Bloom }
func (b *Block) Coinbase() common.Address { return b.header.Coinbase }
func (b *Block) StateRoot() common.Hash   { return b.header.StateRoot }
func (b *Block) ParentHash() common.Hash  { return b.header.ParentHash }
func (b *Block) TxRoot() common.Hash      { return b.header.TxRoot }
func (b *Block) ReceiptRoot() common.Hash { return b.header.ReceiptRoot }
func (b *Block) Extra() []byte            { return common.CopyBytes(b.header.Extra) }
func (b *Block) Header() *Header          { return b.header } // TODO: 新增Header()方法

func (b *Block) BaseFee() *big.Int {
	if b.header.BaseFee == nil {
		return nil
	}
	return new(big.Int).Set(b.header.BaseFee)
}

// Size returns the true RLP encoded storage size of the block, either by encoding
// and returning it, or returning a previously cached value.
func (b *Block) Size() uint64 {
	if size := b.size.Load(); size != nil {
		return size.(uint64)
	}
	c := writeCounter(0)
	rlp.Encode(&c, b)
	b.size.Store(uint64(c))
	return uint64(c)
}

// Hash returns the keccak256 hash of b's header.
// The hash is computed on the first call and cached thereafter.
func (b *Block) Hash() common.Hash {
	if hash := b.hash.Load(); hash != nil {
		return hash.(common.Hash)
	}
	v := b.header.Hash()
	b.hash.Store(v)
	return v
}

// SanityCheck can be used to prevent that unbounded fields are
// stuffed with junk data to add processing overhead
func (b *Block) SanityCheck() error {
	return b.header.SanityCheck()
}

type Blocks []*Block

// HeaderParentHashFromRLP returns the parentHash of an RLP-encoded
// header. If 'header' is invalid, the zero hash is returned.
func HeaderParentHashFromRLP(header []byte) common.Hash {
	// parentHash is the first list element.
	listContent, _, err := rlp.SplitList(header)
	if err != nil {
		return common.Hash{}
	}
	parentHash, _, err := rlp.SplitString(listContent)
	if err != nil {
		return common.Hash{}
	}
	if len(parentHash) != 32 {
		return common.Hash{}
	}
	return common.BytesToHash(parentHash)
}
