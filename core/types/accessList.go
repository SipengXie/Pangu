package types

import (
	"bytes"
	"fmt"
	"github.com/SipengXie/pangu/common"
	"github.com/SipengXie/pangu/core/evm"
)

// AccessList 统一定义一种AccessList形式
type AccessList struct {
	// int = -1表示当前地址没有对应的slot；int >= 0表示Address对应的slot在slots数组中的序号
	Addresses map[common.Address]int
	Slots     []map[common.Hash]struct{}
}

// NewAccessList 新建一个AccessList类型对象
func NewAccessList() *AccessList {
	return &AccessList{
		Addresses: make(map[common.Address]int),
		Slots:     make([]map[common.Hash]struct{}, 0),
	}
}

func (NewAL *AccessList) Len() int {
	return len(NewAL.Addresses)
}

func (NewAL *AccessList) StorageKeys() int {
	var keys int
	for _, slot := range NewAL.Slots {
		keys += len(slot)
	}
	return keys
}

// AccessListIsAddressExce 判断指定地址是否在AccessList中，返回true表示存在，返回false表示不存在
func (NewAL AccessList) AccessListIsAddressExce(address common.Address) bool {
	_, result := NewAL.Addresses[address]
	return result
}

// AccessListAddAddress AccessList添加Address方法，添加后直接将Address的标志位设置为-1
func (NewAL *AccessList) AccessListAddAddress(address common.Address) bool {
	if _, result := NewAL.Addresses[address]; result { // result表示能否从指定key找到对应的value，返回int（slot的数组项序号）和bool
		return false // 该地址已经存在
	}
	NewAL.Addresses[address] = -1 // Address对应的标志位设置为-1
	return true
}

// ContainsAddress returns true if the address is in the access list.
func (NewAL *AccessList) ContainsAddress(address common.Address) bool {
	_, ok := NewAL.Addresses[address]
	return ok
}

// AccessListAddSlot AccessList添加Address对应的slot方法
func (NewAL *AccessList) AccessListAddSlot(address common.Address, slot common.Hash) (addrChange bool, slotChange bool) {
	index, result := NewAL.Addresses[address]
	if !result || index == -1 { // 当address不存在或者address存在但是没有slot（index = -1）
		NewAL.Addresses[address] = len(NewAL.Slots) // 新建一个address-int的kv对或修改已经存在kv对的int值为最新位置（NewAL.slots数组长度）
		slotmap := map[common.Hash]struct{}{slot: {}}
		NewAL.Slots = append(NewAL.Slots, slotmap) // 加入slot数组
		return !result, true
	}
	slotmap := NewAL.Slots[index]
	if _, ok := slotmap[slot]; !ok { // address存在，slot也不为空，但是对应的slot不存在
		slotmap[slot] = struct{}{}
		// Journal add slot change
		return false, true
	}
	// No changes required address与slot都存在
	return false, false
}

// GetTrueAccessList 得到当前操作实际访问到的AccessList，类型归类为*AccessList，表明可以对调用数据进行修改
func (NewAL *AccessList) GetTrueAccessList(op evm.OpCode, scope *evm.ScopeContext) {
	//a := NewAccessList().GetTrueAccessList
	stack := scope.Stack // scope ScopeContext包含每个调用的东西，比如堆栈和内存
	stackData := stack.Data()
	stackLen := len(stackData)
	if (op == evm.SLOAD || op == evm.SSTORE) && stackLen >= 1 {
		slot := common.Hash(stackData[stackLen-1].Bytes32())
		//a.list.addSlot(scope.Contract.Address(), slot)
		NewAL.AccessListAddSlot(scope.Contract.Address(), slot)
	}
	if (op == evm.EXTCODECOPY || op == evm.EXTCODEHASH || op == evm.EXTCODESIZE || op == evm.BALANCE || op == evm.SELFDESTRUCT) && stackLen >= 1 {
		addr := common.Address(stackData[stackLen-1].Bytes20())
		if ok := NewAL.AccessListIsAddressExce(addr); !ok {
			NewAL.AccessListAddAddress(addr)
		}
	}
	if (op == evm.DELEGATECALL || op == evm.CALL || op == evm.STATICCALL || op == evm.CALLCODE) && stackLen >= 5 {
		addr := common.Address(stackData[stackLen-2].Bytes20())
		if ok := NewAL.AccessListIsAddressExce(addr); !ok {
			NewAL.AccessListAddAddress(addr)
		}
	}
	if op == evm.CREATE || op == evm.CREATE2 {
		// TODO: 是否也会访问和修改地址
	}
}

// ConflictDetection 冲突检测函数，检测stateDB中的AccessList和真实的AccessList之间有没有不一样的部分，如果出现了不一样，就需要将该交易放到串行队列中
// 返回值：是否发生冲突，发生冲突项是否有Slot，地址是多少，Slot是多少。注意，后三项返回值只需要在result为false才有意义
func (NewAL *AccessList) ConflictDetection(UserAL *AccessList) (result bool, haveSlot bool, address common.Address, slot []common.Hash) {
	for key, value := range NewAL.Addresses {
		// 不存在slot，address不相同则发生冲突
		if value == -1 {
			result := UserAL.ContainsAddress(key) // false则表示超出了范围
			if !result {                          // AL中不包含这个地址，超出范围
				fmt.Printf("%sERROR MSG%s   某个运行访问到的address 不存在于 用户自定义的AccessList中", FRED, FRESET)
				return false, false, key, []common.Hash{}
			}
		} else {
			// 存在slot，address和slot都不相同则发生冲突
			// 先判断地址是否存在，不存在直接报错
			result := UserAL.ContainsAddress(key) // false则表示超出了范围
			var tempNew []common.Hash
			for key := range NewAL.Slots[value] {
				tempNew = append(tempNew, key) // ! tempTrue中存储了 真实的 对应的slot值
			}
			if !result { // 地址不存在
				fmt.Printf("%sERROR MSG%s   某个运行访问到的address 不存在于 用户自定义的AccessList中", FRED, FRESET)
				return false, true, key, tempNew
			} else {
				// 地址存在，判断slot
				// 拿到用户表中对应address的slot
				var tempUser []common.Hash
				for key := range UserAL.Slots[UserAL.Addresses[key]] { // UserAL.Addresses[key]是stateDB的AccessList保存地址对应的slot序号
					tempUser = append(tempNew, key) // ! tempUser中存储了 用户 对应的slot值
				}
				// 判断slot是否一样
				if len(tempNew) != len(tempUser) { // 长度不一样
					fmt.Printf("%sERROR MSG%s   AccessList中的slot信息不同", FRED, FRESET)
					return false, true, key, tempNew
				}
				// 长度一样，则详细检查
				for _, a := range tempUser {
					isSlotEqual := false
					for i := 0; i < len(tempNew); i++ {
						if bytes.Compare(a[:], tempNew[i][:]) == 0 {
							// 存在相等的
							isSlotEqual = true
							break
						}
					}
					if !isSlotEqual {
						fmt.Printf("%sERROR MSG%s   某个运行访问到的address 不存在于 用户自定义的AccessList中", FRED, FRESET)
						// 不存在相等的slot，访问了AccessList列表之外的地址信息
						return false, true, key, tempNew
					}
				}
			}
		}
	}
	return true, true, common.Address{}, []common.Hash{} // 没有出现冲突，返回true，此时只需要关心result的结果就行，后面三个结果不需要关心
}

// CombineTrueAccessList 构造完整的AccessList函数，将传入的部分AccessList合并到总的AccessList中
func (TrueAL *AccessList) CombineTrueAccessList(NewAL *AccessList) bool {
	for key, value := range NewAL.Addresses {
		// 添加address
		TrueAL.AccessListAddAddress(key)
		// 添加slot
		if value != -1 {
			for slotkey, _ := range NewAL.Slots[value] {
				TrueAL.AccessListAddSlot(key, slotkey)
			}
		} else {
			// 只有address没有slot
		}
	}
	return true
}

// DeleteAddress removes an address from the access list. This operation
// needs to be performed in the same order as the addition happened.
// This method is meant to be used  by the journal, which maintains ordering of
// operations.
func (al *AccessList) DeleteAddress(address common.Address) {
	delete(al.Addresses, address)
}

// DeleteSlot removes an (address, slot)-tuple from the access list.
// This operation needs to be performed in the same order as the addition happened.
// This method is meant to be used  by the journal, which maintains ordering of
// operations.
func (al *AccessList) DeleteSlot(address common.Address, slot common.Hash) {
	idx, addrOk := al.Addresses[address]
	// There are two ways this can fail
	if !addrOk {
		panic("reverting slot change, address not present in list")
	}
	slotmap := al.Slots[idx]
	delete(slotmap, slot)
	// If that was the last (first) slot, remove it
	// Since additions and rollbacks are always performed in order,
	// we can delete the item without worrying about screwing up later indices
	if len(slotmap) == 0 {
		al.Slots = al.Slots[:idx]
		al.Addresses[address] = -1
	}
}

// Contains checks if a slot within an account is present in the access list, returning
// separate flags for the presence of the account and the slot respectively.
func (al *AccessList) Contains(address common.Address, slot common.Hash) (addressPresent bool, slotPresent bool) {
	idx, ok := al.Addresses[address]
	if !ok {
		// no such address (and hence zero slots)
		return false, false
	}
	if idx == -1 {
		// address yes, but no slots
		return true, false
	}
	_, slotPresent = al.Slots[idx][slot]
	return true, slotPresent
}

// AddSlot adds the specified (addr, slot) combo to the access list.
// Return values are:
// - address added
// - slot added
// For any 'true' value returned, a corresponding journal entry must be made.
func (al *AccessList) AddSlot(address common.Address, slot common.Hash) (addrChange bool, slotChange bool) {
	idx, addrPresent := al.Addresses[address]
	if !addrPresent || idx == -1 {
		// Address not present, or addr present but no slots there
		al.Addresses[address] = len(al.Slots)
		slotmap := map[common.Hash]struct{}{slot: {}}
		al.Slots = append(al.Slots, slotmap)
		return !addrPresent, true
	}
	// There is already an (address,slot) mapping
	slotmap := al.Slots[idx]
	if _, ok := slotmap[slot]; !ok {
		slotmap[slot] = struct{}{}
		// Journal add slot change
		return false, true
	}
	// No changes required
	return false, false
}

// AddAddress adds an address to the access list, and returns 'true' if the operation
// caused a change (addr was not previously in the list).
func (al *AccessList) AddAddress(address common.Address) bool {
	if _, present := al.Addresses[address]; present {
		return false
	}
	al.Addresses[address] = -1
	return true
}

// Copy creates an independent copy of an accessList.
func (a *AccessList) Copy() *AccessList {
	cp := NewAccessList()
	for k, v := range a.Addresses {
		cp.Addresses[k] = v
	}
	cp.Slots = make([]map[common.Hash]struct{}, len(a.Slots))
	for i, slotMap := range a.Slots {
		newSlotmap := make(map[common.Hash]struct{}, len(slotMap))
		for k := range slotMap {
			newSlotmap[k] = struct{}{}
		}
		cp.Slots[i] = newSlotmap
	}
	return cp
}
