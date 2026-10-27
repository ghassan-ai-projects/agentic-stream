package app

import (
	"maps"
	"sync"
)

type bornKinds struct {
	mu      sync.Mutex
	digests map[string]string
}

func (k *bornKinds) record(born map[string]string) {
	if len(born) == 0 {
		return
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.digests == nil {
		k.digests = map[string]string{}
	}
	maps.Copy(k.digests, born)
}

func (k *bornKinds) snapshot() map[string]string {
	k.mu.Lock()
	defer k.mu.Unlock()
	return maps.Clone(k.digests)
}

func (h *Host) BornKindDigests() map[string]string {
	return h.born.snapshot()
}
