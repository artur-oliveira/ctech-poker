package presence

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/valkey-io/valkey-go"
)

const (
	connectionKeyPrefix = "poker:presence:connections:"
	tableKeyPrefix      = "poker:presence:table:"
	tableStateTTL       = 30 * 24 * time.Hour
)

const upsertConnectionScript = `
local before = redis.call('ZCARD', KEYS[1])
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])
local active = redis.call('ZCARD', KEYS[1])
redis.call('ZADD', KEYS[1], ARGV[2], ARGV[3])
redis.call('EXPIRE', KEYS[1], ARGV[4])
if active == 0 then return 1 end
return 0`

const closeConnectionScript = `
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])
local removed = redis.call('ZREM', KEYS[1], ARGV[2])
if removed == 1 and redis.call('ZCARD', KEYS[1]) == 0 then
  redis.call('DEL', KEYS[1])
  return 1
end
return 0`

const setTableStateScript = `
local before = redis.call('GET', KEYS[1])
if ARGV[1] == '' then
  redis.call('DEL', KEYS[1])
else
  redis.call('SET', KEYS[1], ARGV[1], 'EX', ARGV[2])
end
local after = redis.call('GET', KEYS[1])
if before == after then return 0 end
return 1`

// readStatusScript returns "<status>|<room id>". A table key written before
// rooms were tracked holds '1'; it still reads as in_table, just with no room,
// so it offers no join target and expires on its own TTL.
const readStatusScript = `
redis.call('ZREMRANGEBYSCORE', KEYS[1], '-inf', ARGV[1])
if redis.call('ZCARD', KEYS[1]) == 0 then
  redis.call('DEL', KEYS[1])
  return 'offline|'
end
local room = redis.call('GET', KEYS[2])
if room then
  if room == '1' then return 'in_table|' end
  return 'in_table|' .. room
end
return 'online|'`

// ValkeyStore uses one sorted set per player so close order and multiple API
// replicas cannot incorrectly mark a still-connected player offline.
type ValkeyStore struct{ client valkey.Client }

func NewValkeyStore(client valkey.Client) *ValkeyStore { return &ValkeyStore{client: client} }

func connectionKey(playerID string) string { return connectionKeyPrefix + playerID }
func tableKey(playerID string) string      { return tableKeyPrefix + playerID }

func (s *ValkeyStore) upsert(ctx context.Context, playerID, connectionID string, expiresAt time.Time) (bool, error) {
	now := time.Now().UnixMilli()
	ttl := max(int64(time.Until(expiresAt).Seconds())+1, 1)
	value, err := s.client.Do(ctx, s.client.B().Eval().Script(upsertConnectionScript).Numkeys(1).
		Key(connectionKey(playerID)).Arg(strconv.FormatInt(now, 10), strconv.FormatInt(expiresAt.UnixMilli(), 10), connectionID, strconv.FormatInt(ttl, 10)).Build()).ToInt64()
	return value == 1, err
}

func (s *ValkeyStore) Open(ctx context.Context, playerID, connectionID string, expiresAt time.Time) (bool, error) {
	return s.upsert(ctx, playerID, connectionID, expiresAt)
}

func (s *ValkeyStore) Heartbeat(ctx context.Context, playerID, connectionID string, expiresAt time.Time) (bool, error) {
	return s.upsert(ctx, playerID, connectionID, expiresAt)
}

func (s *ValkeyStore) Close(ctx context.Context, playerID, connectionID string) (bool, error) {
	value, err := s.client.Do(ctx, s.client.B().Eval().Script(closeConnectionScript).Numkeys(1).
		Key(connectionKey(playerID)).Arg(strconv.FormatInt(time.Now().UnixMilli(), 10), connectionID).Build()).ToInt64()
	return value == 1, err
}

func (s *ValkeyStore) SetInTable(ctx context.Context, playerID, roomID string) (bool, error) {
	changed, err := s.client.Do(ctx, s.client.B().Eval().Script(setTableStateScript).Numkeys(1).
		Key(tableKey(playerID)).Arg(roomID, strconv.FormatInt(int64(tableStateTTL.Seconds()), 10)).Build()).ToInt64()
	return changed == 1, err
}

func (s *ValkeyStore) GetMany(ctx context.Context, playerIDs []string) (map[string]PlayerPresence, error) {
	result := make(map[string]PlayerPresence, len(playerIDs))
	commands := make([]valkey.Completed, 0, len(playerIDs))
	now := strconv.FormatInt(time.Now().UnixMilli(), 10)
	for _, playerID := range playerIDs {
		commands = append(commands, s.client.B().Eval().Script(readStatusScript).Numkeys(2).
			Key(connectionKey(playerID), tableKey(playerID)).Arg(now).Build())
	}
	for i, response := range s.client.DoMulti(ctx, commands...) {
		raw, err := response.ToString()
		if err != nil {
			return nil, fmt.Errorf("presence: read %s: %w", playerIDs[i], err)
		}
		status, room, _ := strings.Cut(raw, "|")
		result[playerIDs[i]] = PlayerPresence{PlayerID: playerIDs[i], Status: Status(status), RoomID: room}
	}
	return result, nil
}
