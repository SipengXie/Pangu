// Copyright 2020 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package state

import (
	"github.com/ethereum/go-ethereum/common"
)

type AccessList struct {
	Addresses map[common.Address]int
	Slots     []map[common.Hash]struct{}
}

// ContainsAddress returns true if the address is in the access list.
func (al *AccessList) ContainsAddress(address common.Address) bool {
	_, ok := al.Addresses[address]
	return ok
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

// newAccessList creates a new accessList.
func newAccessList() *AccessList {
	return &AccessList{
		Addresses: make(map[common.Address]int),
	}
}

// Copy creates an independent copy of an accessList.
func (a *AccessList) Copy() *AccessList {
	cp := newAccessList()
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

// AddAddress adds an address to the access list, and returns 'true' if the operation
// caused a change (addr was not previously in the list).
func (al *AccessList) AddAddress(address common.Address) bool {
	if _, present := al.Addresses[address]; present {
		return false
	}
	al.Addresses[address] = -1
	return true
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

// DeleteAddress removes an address from the access list. This operation
// needs to be performed in the same order as the addition happened.
// This method is meant to be used  by the journal, which maintains ordering of
// operations.
func (al *AccessList) DeleteAddress(address common.Address) {
	delete(al.Addresses, address)
}
