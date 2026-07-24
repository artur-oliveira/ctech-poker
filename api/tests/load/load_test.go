//go:build load

package load

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/table"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
)

func TestMultiTableChaosLoad(t *testing.T) {
	backend := cache.NewMemoryBackend(1024)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	numManagers := 3
	numTables := 10
	managers := make([]*tablemanager.Manager, numManagers)
	for i := 0; i < numManagers; i++ {
		leases := tablelease.NewService(backend)
		managers[i] = tablemanager.NewManager(leases, nil, nil, nil)
	}

	seed := func() *hand.Table {
		tbl := hand.NewTable(nil, 10, 20)
		_ = tbl.AddWaitingPlayer(&hand.Player{ID: "p1", Stack: 1000})
		_ = tbl.AddWaitingPlayer(&hand.Player{ID: "p2", Stack: 1000})
		_ = tbl.StartHand()
		return tbl
	}

	var wg sync.WaitGroup
	for tableIdx := 0; tableIdx < numTables; tableIdx++ {
		tableID := fmt.Sprintf("load-table-%d", tableIdx)
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			for step := 0; step < 20; step++ {
				mgrIdx := (step + tableIdx) % numManagers
				mgr := managers[mgrIdx]
				actor, err := mgr.GetOrCreateActor(ctx, id, seed)
				if err != nil {
					continue
				}
				_ = actor.Dispatch(table.ActCmd{
					PlayerID: "p1", ActionID: fmt.Sprintf("act-%d-%d", tableIdx, step), Action: "check", Amount: 0,
					Reply: make(chan error, 1),
				})
				time.Sleep(10 * time.Millisecond)
			}
		}(tableID)
	}

	wg.Wait()
}
