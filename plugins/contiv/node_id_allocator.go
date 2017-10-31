// Copyright (c) 2017 Cisco and/or its affiliates.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at:
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package contiv

import (
	"encoding/json"
	"fmt"
	"github.com/contiv/vpp/flavors/ksr"
	"github.com/contiv/vpp/plugins/contiv/model/uid"
	"github.com/ligato/cn-infra/db/keyval"
	"github.com/ligato/cn-infra/db/keyval/etcdv3"
	"github.com/ligato/cn-infra/servicelabel"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	allocatedIDsKeyPrefix = "allocatedIDs/"
	maxAttempts           = 10
)

var (
	errInvalidKey         = fmt.Errorf("invalid key for nodeID")
	errUnableToAllocateID = fmt.Errorf("unable to allocate unique id for node (max attempt limit reached)")
	errNoIDallocated      = fmt.Errorf("there is no ID allocated for the node")
)

// idAllocator manages allocation/deallocation of unique number identifying a node in the k8s cluster.
// Retrieved identifier is used as input of IPAM module for the node.
// (AllocatedID is represented by an entry in ETCD. The process of allocation leverages etcd transaction
// to atomically check if the key exists  and if not, a new key-value pair representing
// the allocation is inserted)
type idAllocator struct {
	sync.Mutex
	etcd         *etcdv3.Plugin
	serviceLabel string
	broker       keyval.ProtoBroker

	allocated bool

	ID uint32
}

// newIDAllocator creates new instance of idAllocator
func newIDAllocator(etcd *etcdv3.Plugin, serviceLabel string) *idAllocator {
	return &idAllocator{
		etcd:         etcd,
		serviceLabel: serviceLabel,
		broker:       etcd.NewBroker(servicelabel.GetDifferentAgentPrefix(ksr.MicroserviceLabel)),
	}
}

// getID returns unique number for the given node
func (ia *idAllocator) getID() (id uint32, err error) {
	ia.Lock()
	defer ia.Unlock()

	if ia.allocated {
		return ia.ID, nil
	}

	// check if there is already assign ID for the serviceLabel
	existingEntry, err := ia.findExistingEntry(ia.broker)
	if err != nil {
		return 0, err
	}

	if existingEntry != nil {
		ia.allocated = true
		ia.ID = existingEntry.Id
		return ia.ID, nil
	}

	attempts := 0
	for {
		ids, err := listAllIDs(ia.broker)
		if err != nil {
			return 0, err
		}
		sort.Ints(ids)

		attempts++
		ia.ID = uint32(findFirstAvailableIndex(ids))

		succ, err := ia.writeIfNotExists(ia.ID)
		if err != nil {
			return 0, err
		}
		if succ {
			ia.allocated = true
			break
		}

		if attempts > maxAttempts {
			return 0, errUnableToAllocateID
		}

	}

	return ia.ID, nil
}

// releaseID returns allocated ID back to the pool
func (ia *idAllocator) releaseID() error {
	ia.Lock()
	defer ia.Unlock()

	if !ia.allocated {
		return errNoIDallocated
	}

	_, err := ia.broker.Delete(createKey(ia.ID))
	if err == nil {
		ia.allocated = false
	}

	return err
}

func (ia *idAllocator) writeIfNotExists(id uint32) (succeeded bool, err error) {

	value := &uid.Identifier{Name: ia.serviceLabel, Id: id}

	encoded, err := json.Marshal(value)
	if err != nil {
		return false, err
	}

	succeeded, err = ia.etcd.PutIfNotExists(servicelabel.GetDifferentAgentPrefix(ksr.MicroserviceLabel)+createKey(id), encoded)

	return succeeded, err

}

// findExistingEntry lists all allocated entries and check if the etcd contains ID assigned
// to the serviceLabel
func (ia *idAllocator) findExistingEntry(broker keyval.ProtoBroker) (id *uid.Identifier, err error) {
	var existingEntry *uid.Identifier
	it, err := broker.ListValues(allocatedIDsKeyPrefix)
	if err != nil {
		return nil, err
	}

	for {
		item := &uid.Identifier{}
		kv, stop := it.GetNext()

		if stop {
			break
		}

		err := kv.GetValue(item)
		if err != nil {
			return nil, err
		}

		if item.Name == ia.serviceLabel {
			existingEntry = item
			break
		}
	}

	return existingEntry, nil

}

// findFirstAvailableIndex returns the smallest int that is not assigned to a node
func findFirstAvailableIndex(ids []int) int {
	res := 1
	for _, v := range ids {

		if res == v {
			res++
		} else {
			break
		}
	}
	return res
}

// listAllIDs returns slice that contains allocated ids i.e.: ids assigned to a node
func listAllIDs(broker keyval.ProtoBroker) (ids []int, err error) {
	it, err := broker.ListKeys(allocatedIDsKeyPrefix)
	if err != nil {
		return nil, err
	}

	for {

		key, _, stop := it.GetNext()

		if stop {
			break
		}

		id, err := extractIndexFromKey(key)
		if err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, nil

}

func extractIndexFromKey(key string) (int, error) {
	if strings.HasPrefix(key, allocatedIDsKeyPrefix) {
		return strconv.Atoi(strings.Replace(key, allocatedIDsKeyPrefix, "", 1))

	}
	return 0, errInvalidKey
}

func createKey(index uint32) string {
	str := strconv.FormatUint(uint64(index), 10)
	return allocatedIDsKeyPrefix + str
}
