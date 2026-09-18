package handeval

import "math/bits"

const (
	// The additive rank key needs 33 bits. Four four-bit suit counters live
	// above it, leaving room for the rank sum without carrying into suits.
	suitKeyShift            = 36
	suitLaneBits            = 4
	suitRankBits            = 16
	suitBias         uint64 = 0x3333 << suitKeyShift
	flushCounterMask uint64 = 0x8888 << suitKeyShift
	packedRankMask   uint64 = (1 << suitKeyShift) - 1
	suitedRankMask   uint64 = (1 << 13) - 1
)

type packedCard struct {
	key  uint64
	mask uint64
}

var packedCards [52]packedCard

// Called once after rankWeights are initialized. Each mask uses one 16-bit
// lane per suit, allowing the flush rank mask to be extracted with one shift.
func initPackedCards() {
	for id := range packedCards {
		rank, suit := id>>2, id&3
		packedCards[id] = packedCard{
			key:  rankWeights[rank] + uint64(1)<<(suitKeyShift+suitLaneBits*suit),
			mask: uint64(1) << (suitRankBits*suit + rank),
		}
	}
}

func packedScore(key, mask uint64) Score {
	// Adding three sets a lane's high bit iff at least five cards have that
	// suit. With at most seven cards no lane can overflow, and at most one
	// suit can be a flush. A flush beats every non-flush subset of seven cards.
	flush := (key + suitBias) & flushCounterMask
	if flush != 0 {
		shift := (bits.TrailingZeros64(flush) - (suitKeyShift + suitLaneBits - 1)) * (suitRankBits / suitLaneBits)
		return Score(flushTable[(mask>>shift)&suitedRankMask])
	}
	return Score(noFlushTable[rankIndex(key&packedRankMask)])
}
