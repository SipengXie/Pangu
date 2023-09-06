package core

import (
	"bytes"
	"math/big"
	"sort"

	"github.com/SipengXie/pangu/common"
	"github.com/SipengXie/pangu/core/types"
	mapset "github.com/deckarep/golang-set/v2"
)

type TxClass struct {
	Tx         *types.Transaction
	ID         int
	TxResource mapset.Set[string]
}

// NewTxClassList 生成一个TxClass用于对交易列表进行分类
func NewTxClassList(txs types.Transactions) []TxClass {
	var txClass []TxClass
	for i := 0; i < len(txs); i++ {
		var temp TxClass
		temp.Tx = txs[i]
		temp.ID = i
		temp.TxResource = MergeAccessList(txs[i].AccessList())
		txClass = append(txClass, temp)
	}
	return txClass
}

// NewTxResource 建每个交易的资源集合
func NewTxResource(txs types.Transactions) map[*types.Transaction]mapset.Set[string] {
	// 使用map构建每个交易的资源集合
	txSet := make(map[*types.Transaction]mapset.Set[string])
	for _, tx := range txs {
		al := tx.AccessList()
		txSet[tx] = MergeAccessList(al)
	} // 现在每个tx后面跟着一个资源Set
	return txSet
}

// MergeAccessList 将AccessList中的Address和StorageKey进行合并得到一个集合
func MergeAccessList(al *types.AccessList) mapset.Set[string] {
	set := mapset.NewSet[string]()
	for addr, num := range al.Addresses {
		addrStr := addr.Hex()
		if num > 0 { // num > 0说明有 slots
			for key, _ := range al.Slots[num] {
				keyStr := key.Hex()
				set.Add(addrStr + keyStr)
			}
		} else {
			set.Add(addrStr)
		}
	}
	return set
}

// IsConflict 判断两个集合是否存在交集
func IsConflict(set1, set2 mapset.Set[string]) bool {
	temp := set1.Intersect(set2)
	if temp.Cardinality() != 0 {
		return true
	} else {
		return false
	}
}

// ClassifyTx TODO: 对交易按资源是否冲突进行分类
// 返回的map中每个int对应一个类，但不保证int是连续单增的，即int是什么不重要
// 其中每个类别的交易序列按地址进行排序，若地址相同则按其Nonce从小到大排序
func ClassifyTx(txs types.Transactions, signer types.Signer) []types.Transactions {
	// 对txClassList的ID进行处理
	txClassList := NewTxClassList(txs)
	TxClassListLen := len(txClassList)
	for i := 0; i < TxClassListLen; i++ {
		for j := i + 1; j < TxClassListLen; j++ {
			if !IsConflict(txClassList[i].TxResource, txClassList[j].TxResource) {
				continue // 不存在冲突则继续循环
			} else { // 若存在冲突，两个交易合为一类
				txClassList[j].ID = txClassList[i].ID // 分类的ID进行改变
				txClassList[i].TxResource = txClassList[i].TxResource.Union(txClassList[j].TxResource)
				txClassList[j].TxResource = txClassList[i].TxResource // 交易的资源合并再继续与之后的交易进行比较
			}
		}
	}
	// 对交易进行分组
	groups := make(map[int][]*types.Transaction)
	for i := 0; i < TxClassListLen; i++ {
		if _, ok := groups[txClassList[i].ID]; ok {
			// key 存在，可以使用 value
			groups[txClassList[i].ID] = append(groups[txClassList[i].ID], txClassList[i].Tx)
		} else {
			// key 不存在，增加该 key
			groups[txClassList[i].ID] = make([]*types.Transaction, 0)
			groups[txClassList[i].ID] = append(groups[txClassList[i].ID], txClassList[i].Tx)

		}
	}
	// 对每个组按From地址排序
	for _, txList := range groups {
		sort.Slice(txList, func(i, j int) bool {
			acct1, _ := types.Sender(signer, txList[i])
			acct2, _ := types.Sender(signer, txList[j])
			if bytes.Compare(acct1.Bytes(), acct2.Bytes()) < 0 {
				return true
			} else if bytes.Compare(acct1.Bytes(), acct2.Bytes()) > 0 {
				return false
			}
			return txList[i].Nonce() < txList[j].Nonce()
		})
	}
	groupsRes := make([]types.Transactions, 0)
	for _, txList := range groups {
		groupsRes = append(groupsRes, txList)
	}
	return groupsRes
}

// FindMaxGasPrice
func FindMaxGasPrice(txMap map[common.Address][]*types.Transaction) common.Address {
	var maxGasPriceAddr common.Address
	maxGasPrice := big.NewInt(0)
	for addr, txs := range txMap {
		if txs[0].GasPrice().Cmp(maxGasPrice) > 0 {
			maxGasPriceAddr = addr
			maxGasPrice = txs[0].GasPrice()
		}
	}
	return maxGasPriceAddr
}

// TODO: SelectTxs 选出一些Transaction，这些Transaction的GasLimit的总和要最大且不超过整体的GasLimit
func SelectTxs(txMap map[common.Address][]*types.Transaction, gasLimit uint64) types.Transactions {
	// copy一个副本
	temp := make(map[common.Address][]*types.Transaction)
	for addr, txs := range txMap {
		txsCopy := make([]*types.Transaction, len(txs))
		copy(txsCopy, txs)
		temp[addr] = txsCopy
	}
	var sum uint64
	var selectedTxs types.Transactions
	for {
		if sum > gasLimit {
			break
		}
		selectedAddr := FindMaxGasPrice(temp)
		selectedTxs = append(selectedTxs, temp[selectedAddr][0])
		temp[selectedAddr] = temp[selectedAddr][1:] // 去掉对应地址中的第一个交易
	}
	return selectedTxs
}
