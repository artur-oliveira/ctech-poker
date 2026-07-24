package table

import (
	"time"

	"gopkg.aoctech.app/poker/api/internal/roomstore"
)

type escalateCmd struct{ Reply chan error }

func (c escalateCmd) reply() chan error { return c.Reply }

// StartEscalation starts the private-room blind clock. Mutations are sent
// through the actor loop and the worker exits when the actor exits.
func (a *Actor) StartEscalation(cfg roomstore.BlindEscalation) {
	a.escalationCfg = cfg
	interval := time.Duration(cfg.IntervalMinutes) * time.Minute
	if a.escalationInterval > 0 {
		interval = a.escalationInterval
	}
	if interval <= 0 || cfg.Multiplier <= 100 || cfg.Max <= 0 {
		return
	}
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				reply := make(chan error, 1)
				select {
				case a.cmds <- escalateCmd{Reply: reply}:
				case <-a.done:
					return
				}
				select {
				case err := <-reply:
					if err != nil {
						return
					}
				case <-a.done:
					return
				}
			case <-a.done:
				return
			}
		}
	}()
}
