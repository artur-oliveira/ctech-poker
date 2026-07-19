// Package tableowner advertises which instance currently owns a table's
// write lease, so a WebSocket connection that lands on the wrong instance
// knows where to proxy to (see internal/api/v1/tableproxy.go). This is
// advisory routing information only — tablelease.Service remains the sole
// source of truth for who is actually allowed to write.
package tableowner

import (
	"context"
	"time"

	"gopkg.aoctech.app/api-commons/cache"
)

const keyPrefix = "table_owner:"

type Registry struct {
	c   cache.Backend
	ttl time.Duration
}

func NewRegistry(c cache.Backend, ttl time.Duration) *Registry {
	return &Registry{c: c, ttl: ttl}
}

// Advertise records instanceAddr as table_id's current owner. Only ever
// called by the instance that just won (or renewed) the table's lease — an
// unconditional Set is safe because the caller already proved ownership via
// tablelease.Service before reaching here.
func (r *Registry) Advertise(ctx context.Context, tableID, instanceAddr string) error {
	return r.c.Set(ctx, keyPrefix+tableID, []byte(instanceAddr), int(r.ttl.Seconds()))
}

// Lookup returns the advertised owner address, if any. A false ok means
// either no instance has ever advertised for tableID, or the advertisement
// expired (the owning instance stopped renewing — implies its lease lapsed
// too, since both are refreshed on the same heartbeat tick).
func (r *Registry) Lookup(ctx context.Context, tableID string) (string, bool, error) {
	v, ok, err := r.c.Get(ctx, keyPrefix+tableID)
	if err != nil || !ok {
		return "", false, err
	}
	return string(v), true, nil
}
