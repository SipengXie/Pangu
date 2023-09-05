package types

import "github.com/SipengXie/pangu/common"

// AccessList 统一定义一种AccessList形式
type AccessList struct {
	// int = -1表示当前地址没有对应的slot；int >= 0表示Address对应的slot在slots数组中的序号
	Addresses map[common.Address]int
	Slots     []map[common.Hash]struct{}
}

// 其他方法等地后续添加
func (al *AccessList) Len() int {
	return len(al.Addresses)
}

func (al *AccessList) StorageKeys() int {
	var keys int
	for _, slot := range al.Slots {
		keys += len(slot)
	}
	return keys
}
