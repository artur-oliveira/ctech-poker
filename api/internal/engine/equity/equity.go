// Package equity estimates poker equity against random opponent ranges.
package equity

import (
	"container/list"
	"fmt"
	"math/rand/v2"
	"sort"
	"sync"

	"gopkg.aoctech.app/poker/api/internal/engine/deck"
	"gopkg.aoctech.app/poker/api/internal/engine/handeval"
)

type cacheKey struct {
	tableID    string
	hole       [2]uint8
	board      [5]uint8
	boardLen   uint8
	opponents  uint8
	iterations int
}

type cacheEntry struct {
	key         cacheKey
	value       float64
	approxBytes int64
}

type lruCache struct {
	mu       sync.Mutex
	maxBytes int64
	bytes    int64
	items    map[cacheKey]*list.Element
	evict    *list.List
}

// cacheEntryBaseBytes is a deliberately conservative estimate of the map
// bucket slot, list.Element, cacheEntry and key/value storage retained by one
// entry. tableID bytes are added separately. The cache is a safety bound, not
// an allocator accounting API, so erring high is preferable to retaining more
// heap than its configured budget suggests.
const cacheEntryBaseBytes int64 = 160

func newLRUCache(maxBytes int64) *lruCache {
	return &lruCache{
		maxBytes: maxBytes,
		items:    make(map[cacheKey]*list.Element),
		evict:    list.New(),
	}
}

func (c *lruCache) Get(key cacheKey) (float64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.items[key]; ok {
		c.evict.MoveToFront(elem)
		return elem.Value.(*cacheEntry).value, true
	}
	return 0, false
}

func (c *lruCache) Put(key cacheKey, value float64) (evicted bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.evict.MoveToFront(elem)
		elem.Value.(*cacheEntry).value = value
		return false
	}

	entryBytes := cacheEntryBaseBytes + int64(len(key.tableID))
	if entryBytes > c.maxBytes {
		return false
	}

	for c.bytes+entryBytes > c.maxBytes {
		oldest := c.evict.Back()
		if oldest == nil {
			break
		}
		oldEntry := oldest.Value.(*cacheEntry)
		c.evict.Remove(oldest)
		delete(c.items, oldEntry.key)
		c.bytes -= oldEntry.approxBytes
		evicted = true
	}

	entry := &cacheEntry{key: key, value: value, approxBytes: entryBytes}
	elem := c.evict.PushFront(entry)
	c.items[key] = elem
	c.bytes += entryBytes
	return evicted
}

func (c *lruCache) EvictTable(tableID string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	removed := 0
	for key, elem := range c.items {
		if key.tableID != tableID {
			continue
		}
		entry := elem.Value.(*cacheEntry)
		c.evict.Remove(elem)
		delete(c.items, key)
		c.bytes -= entry.approxBytes
		removed++
	}
	return removed
}

const globalEquityCacheMaxBytes int64 = 4 << 20

var globalEquityCache = newLRUCache(globalEquityCacheMaxBytes)

func makeCacheKey(hole [2]deck.Card, board, deadCards []deck.Card, numOpponents, iterations int) (cacheKey, bool) {
	if len(deadCards) > 0 || len(board) > 5 || numOpponents > 255 || iterations > 20000 {
		return cacheKey{}, false
	}
	h1 := handeval.CardID(hole[0])
	h2 := handeval.CardID(hole[1])
	if h1 > h2 {
		h1, h2 = h2, h1
	}
	var k cacheKey
	k.hole = [2]uint8{h1, h2}
	k.boardLen = uint8(len(board))
	for i, c := range board {
		k.board[i] = handeval.CardID(c)
	}
	// Sort board card IDs to normalize key
	sort.Slice(k.board[:k.boardLen], func(i, j int) bool {
		return k.board[i] < k.board[j]
	})
	k.opponents = uint8(numOpponents)
	k.iterations = iterations
	return k, true
}

// rng64 is a 64-bit XorShift64Star PRNG with 32-bit caching.
// It generates two 32-bit pseudo-random values per 64-bit step (~0.5ns per output).
type rng64 struct {
	state uint64
	cache uint32
	has   bool
}

func (r *rng64) next32() uint32 {
	if r.has {
		r.has = false
		return r.cache
	}
	x := r.state
	if x == 0 {
		x = 0x853c49e6748fea9b
	}
	x ^= x >> 12
	x ^= x << 25
	x ^= x >> 27
	r.state = x
	val := x * 0x2545F4914F6CDD1D
	r.cache = uint32(val >> 32)
	r.has = true
	return uint32(val)
}

func (r *rng64) intn(k uint32) uint32 {
	m := uint64(r.next32()) * uint64(k)
	return uint32(m >> 32)
}

type EstimateStats struct {
	CacheHit bool
	Evicted  bool
}

func Estimate(hole [2]deck.Card, board, deadCards []deck.Card, numOpponents, iterations int) (float64, error) {
	value, _, err := EstimateWithStats(hole, board, deadCards, numOpponents, iterations)
	return value, err
}

func EstimateWithStats(hole [2]deck.Card, board, deadCards []deck.Card, numOpponents, iterations int) (float64, EstimateStats, error) {
	return EstimateForTableWithStats("", hole, board, deadCards, numOpponents, iterations)
}

// EstimateForTableWithStats scopes cached results to the actor that requested
// them. The equity value itself is table-independent, but carrying tableID in
// the key lets actor teardown promptly release everything that table retained.
func EstimateForTableWithStats(tableID string, hole [2]deck.Card, board, deadCards []deck.Card, numOpponents, iterations int) (float64, EstimateStats, error) {
	if numOpponents < 1 || iterations < 1 {
		return 0, EstimateStats{}, fmt.Errorf("equity: opponents and iterations must be positive")
	}
	if len(board) > 5 {
		return 0, EstimateStats{}, fmt.Errorf("equity: board has %d cards, maximum is 5", len(board))
	}

	key, cacheable := makeCacheKey(hole, board, deadCards, numOpponents, iterations)
	if cacheable {
		key.tableID = tableID
		if val, ok := globalEquityCache.Get(key); ok {
			return val, EstimateStats{CacheHit: true}, nil
		}
	}

	var pool [52]uint8
	poolLen, err := buildPool(hole, board, deadCards, &pool)
	if err != nil {
		return 0, EstimateStats{}, err
	}

	boardNeeded := 5 - len(board)
	need := boardNeeded + numOpponents*2
	if need > poolLen {
		return 0, EstimateStats{}, fmt.Errorf("equity: not enough cards to sample %d opponents", numOpponents)
	}

	hero1ID := handeval.CardID(hole[0])
	hero2ID := handeval.CardID(hole[1])

	var baseBoardState handeval.BoardState
	for _, c := range board {
		baseBoardState.AddCard(c)
	}

	rng := rng64{state: rand.Uint64()}

	var cards [52]uint8
	copy(cards[:poolLen], pool[:poolLen])

	var shares float64

	for range iterations {
		for i := range need {
			j := i + int(rng.intn(uint32(poolLen-i)))
			cards[i], cards[j] = cards[j], cards[i]
		}

		boardState := baseBoardState
		for i := range boardNeeded {
			boardState.AddCardID(cards[i])
		}
		boardState.Finalize()

		myScore := boardState.Eval2IDs(hero1ID, hero2ID)
		bestScore := myScore
		tiedWinners := 1

		for opponent := range numOpponents {
			offset := boardNeeded + opponent*2
			score := boardState.Eval2IDs(cards[offset], cards[offset+1])
			if score > myScore {
				bestScore = score
				break
			}
			if score == myScore {
				tiedWinners++
			}
		}

		if bestScore == myScore {
			if tiedWinners == 1 {
				shares += 1.0
			} else {
				shares += 1.0 / float64(tiedWinners)
			}
		}
	}

	res := shares / float64(iterations)
	stats := EstimateStats{}
	if cacheable {
		stats.Evicted = globalEquityCache.Put(key, res)
	}
	return res, stats, nil
}

// EvictTable releases all process-global equity results associated with a
// table. tablemanager calls it whenever that table's actor is torn down.
func EvictTable(tableID string) int { return globalEquityCache.EvictTable(tableID) }

func buildPool(hole [2]deck.Card, board, dead []deck.Card, pool *[52]uint8) (int, error) {
	var seen uint64
	checkAndAdd := func(c deck.Card) error {
		if c.Rank < deck.Two || c.Rank > deck.Ace || c.Suit < deck.Clubs || c.Suit > deck.Spades {
			return fmt.Errorf("equity: invalid card %+v", c)
		}
		id := handeval.CardID(c)
		mask := uint64(1) << id
		if (seen & mask) != 0 {
			return fmt.Errorf("equity: duplicate known card %+v", c)
		}
		seen |= mask
		return nil
	}

	if err := checkAndAdd(hole[0]); err != nil {
		return 0, err
	}
	if err := checkAndAdd(hole[1]); err != nil {
		return 0, err
	}
	for _, c := range board {
		if err := checkAndAdd(c); err != nil {
			return 0, err
		}
	}
	for _, c := range dead {
		if err := checkAndAdd(c); err != nil {
			return 0, err
		}
	}

	poolLen := 0
	for id := uint8(0); id < 52; id++ {
		if (seen & (uint64(1) << id)) == 0 {
			pool[poolLen] = id
			poolLen++
		}
	}
	return poolLen, nil
}
