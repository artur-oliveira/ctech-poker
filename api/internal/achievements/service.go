package achievements

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/cache"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
)

type progressStore interface {
	Increment(context.Context, string, string, string, int) (previous, current int, err error)
	IncrementStreak(context.Context, string, string, string, bool, int) (current int, err error)
	ListAchievements(ctx context.Context, playerID, mode string, limit int, startKey map[string]types.AttributeValue) ([]PlayerAchievementProgress, map[string]types.AttributeValue, error)
	UpdateTableStreak(ctx context.Context, playerID, mode, tableID string, won bool) (current int, err error)
}

type Service struct {
	store progressStore
	// cache is the cross-instance home of lastPocketPair below (Valkey in
	// production). Any instance can run the hand that completes, so a
	// process-local memory of "which pocket pair did this player last win
	// with" made KeySamePocketPairStreak reset or continue depending purely
	// on which instance happened to serve the hand. nil in dev/tests, where
	// the map is the whole fleet's memory.
	cache cache.Backend
	mu    sync.Mutex
	// lastPocketPair is the no-cache fallback only — see cache above.
	lastPocketPair map[string]byte
}

// pocketPairKeyPrefix namespaces the shared copy of lastPocketPair.
const pocketPairKeyPrefix = "poker:achv:lastpocketpair:"

// pocketPairTTL keeps a player's last winning pocket pair alive across a
// normal session gap. Expiry only resets the streak, which is also what a
// non-qualifying hand does, so it can never over-award.
const pocketPairTTL = 30 * 24 * time.Hour

// SetCache wires the shared store for cross-instance service state. Set once,
// at construction, by app wiring.
func (s *Service) SetCache(c cache.Backend) { s.cache = c }

// lastPocketPairFor reads the rank this player last won with, from the shared
// cache when there is one. A read error is reported as "no previous pair",
// which resets the streak — the same safe direction the rest of RecordHand
// takes on missing data.
func (s *Service) lastPocketPairFor(ctx context.Context, stateKey string) byte {
	if s.cache == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		return s.lastPocketPair[stateKey]
	}
	raw, found, err := s.cache.Get(ctx, pocketPairKeyPrefix+stateKey)
	if err != nil {
		slog.Warn("achievements: last pocket pair load failed", "key", stateKey, "err", err)
		return 0
	}
	if !found || len(raw) != 1 {
		return 0
	}
	return raw[0]
}

// storeLastPocketPair records rank as this player's last winning pocket pair,
// or clears it when rank is 0 (the hand did not qualify).
func (s *Service) storeLastPocketPair(ctx context.Context, stateKey string, rank byte) {
	if s.cache == nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if rank == 0 {
			delete(s.lastPocketPair, stateKey)
		} else {
			s.lastPocketPair[stateKey] = rank
		}
		return
	}
	key := pocketPairKeyPrefix + stateKey
	var err error
	if rank == 0 {
		err = s.cache.Delete(ctx, key)
	} else {
		err = s.cache.Set(ctx, key, []byte{rank}, int(pocketPairTTL.Seconds()))
	}
	if err != nil {
		slog.Warn("achievements: last pocket pair save failed", "key", stateKey, "err", err)
	}
}

type HandMetric struct {
	PlayerID string
	VPIP     bool
	ThreeBet bool
	// Peeked is true when this player looked at their own hole cards at any
	// point this hand (client-reported "peek_cards" action; see PeekCardsCmd
	// in api/internal/table). Gates KeyAllInBlind/KeyBlindMagic below.
	Peeked bool
	// TimeBankMs is this player's time-bank milliseconds consumed during the
	// hand, summed from the action log by app.go's onHandComplete. Drives
	// KeyNoRush below. Zero means either "decided in time" or "no action log
	// to read", and both correctly award nothing.
	TimeBankMs int64
}

type TierUnlock struct {
	PlayerID string
	Key      string
	Stars    int
}

func NewService(store *Store) *Service                 { return newService(store) }
func NewServiceWithStore(store progressStore) *Service { return newService(store) }
func newService(store progressStore) *Service {
	return &Service{store: store, lastPocketPair: make(map[string]byte)}
}

func (s *Service) RecordHand(ctx context.Context, tableID, mode string, outcome hand.HandOutcome, metricSets ...[]HandMetric) ([]TierUnlock, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("achievements: progress store is required")
	}
	var unlocks []TierUnlock
	bumpBy := func(playerID, key string, by int) error {
		previous, current, err := s.store.Increment(ctx, playerID, mode, key, by)
		if err != nil {
			return fmt.Errorf("achievements: table %s player %s key %s: %w", tableID, playerID, key, err)
		}
		if stars, crossed := TierCrossed(key, previous, current); crossed {
			unlocks = append(unlocks, TierUnlock{PlayerID: playerID, Key: key, Stars: stars})
		}
		return nil
	}
	bump := func(playerID, key string) error { return bumpBy(playerID, key, 1) }
	streak := func(playerID, key string, reset bool, resetTo int) error {
		previous := 0
		if !reset {
			previous = -1 // only exact threshold crossing is needed below
		}
		current, err := s.store.IncrementStreak(ctx, playerID, mode, key, reset, resetTo)
		if err != nil {
			return fmt.Errorf("achievements: table %s player %s key %s: %w", tableID, playerID, key, err)
		}
		if reset {
			previous = resetTo
		} else {
			previous = current - 1
		}
		if stars, crossed := TierCrossed(key, previous, current); crossed {
			unlocks = append(unlocks, TierUnlock{PlayerID: playerID, Key: key, Stars: stars})
		}
		return nil
	}
	handsTotals := make(map[string]int)
	for _, id := range dedupe(outcome.Participants) {
		previous, current, err := s.store.Increment(ctx, id, mode, KeyHandsPlayed, 1)
		if err != nil {
			return nil, err
		}
		handsTotals[id] = current
		if stars, crossed := TierCrossed(KeyHandsPlayed, previous, current); crossed {
			unlocks = append(unlocks, TierUnlock{PlayerID: id, Key: KeyHandsPlayed, Stars: stars})
		}
	}
	if len(metricSets) > 0 {
		for _, metric := range metricSets[0] {
			if metric.TimeBankMs <= 0 {
				continue
			}
			if err := bumpBy(metric.PlayerID, KeyNoRush, int(metric.TimeBankMs)); err != nil {
				return nil, err
			}
		}
	}
	winnerSet := stringSet(outcome.Winners)
	allInSet := stringSet(outcome.AllInPlayers)
	// reportedPeek is deliberately separate from "not present in peekedSet":
	// when pokerstats.Analyze had no action log to read, metricSets carries no
	// entry for anyone, and treating that as "definitely didn't peek" would
	// wrongly grant KeyBlindMagic/KeyAllInBlind on missing data instead of
	// just skipping them, the same safe failure mode KeyFoldedStreak already
	// has (it only bumps for players actually present in metricSets).
	reportedPeek := make(map[string]bool)
	peekedSet := make(map[string]bool)
	if len(metricSets) > 0 {
		for _, metric := range metricSets[0] {
			reportedPeek[metric.PlayerID] = true
			if metric.Peeked {
				peekedSet[metric.PlayerID] = true
			}
		}
	}
	for _, id := range dedupe(outcome.Winners) {
		if err := bump(id, KeyWins); err != nil {
			return nil, err
		}
		if reportedPeek[id] && !peekedSet[id] {
			if err := bump(id, KeyBlindMagic); err != nil {
				return nil, err
			}
		}
		category := outcome.WinningCategory
		if result, ok := outcome.ShowdownResults[id]; ok && result.Category != "" {
			category = result.Category
		}
		if category != "" {
			if err := bump(id, KeyWinByCategory(category)); err != nil {
				return nil, err
			}
		}
		if payout := int(outcome.Payouts[id]); payout > 0 {
			key := KeySandboxChipsEarned
			if mode == "real" {
				key = KeyRealMoneyEarned
			}
			if err := bumpBy(id, key, payout); err != nil {
				return nil, err
			}
		}
		if hi, ok := outcome.PlayerHands[id]; ok && isPocketPair(hi.HoleCards) {
			if err := bump(id, KeyWonWithPocketPair); err != nil {
				return nil, err
			}
		}
		if len(dedupe(outcome.Participants)) == 9 {
			if err := bump(id, KeyWonFullTable); err != nil {
				return nil, err
			}
		}
		if len(dedupe(outcome.Participants)) == 2 {
			if err := bump(id, KeyWonHeadsUp); err != nil {
				return nil, err
			}
		}
		if allInSet[id] && handsTotals[id] == 1 {
			if err := bump(id, KeyFirstHandAllInWin); err != nil {
				return nil, err
			}
		}
		if beatOpponent(outcome, id, func(opp string, result hand.ShowdownResult) bool {
			hi, ok := outcome.PlayerHands[opp]
			return ok && isPocketAces(hi.HoleCards)
		}) {
			if err := bump(id, KeyBeatPocketAces); err != nil {
				return nil, err
			}
		}
		if beatOpponent(outcome, id, func(_ string, result hand.ShowdownResult) bool {
			return categoryAtLeast(result.Category, "three_of_a_kind")
		}) {
			if err := bump(id, KeyBeatTripsOrBetter); err != nil {
				return nil, err
			}
		}
		if hasCompleteCards(outcome, id) {
			if isNuts(outcome, id) {
				if err := bump(id, KeyWonWithNuts); err != nil {
					return nil, err
				}
			}
			if wonRunnerRunner(outcome, id) {
				if err := bump(id, KeyWonRunnerRunner); err != nil {
					return nil, err
				}
			}
		}
	}
	for _, id := range dedupe(outcome.AllInPlayers) {
		if err := bump(id, KeyAllIn); err != nil {
			return nil, err
		}
		if reportedPeek[id] && !peekedSet[id] {
			if err := bump(id, KeyAllInBlind); err != nil {
				return nil, err
			}
		}
	}
	for _, id := range dedupe(outcome.ComebackWinners) {
		if err := bump(id, KeyComeback); err != nil {
			return nil, err
		}
		if wonAgainstBiggerStack(outcome, id) {
			if err := bump(id, KeyGiantSlayer); err != nil {
				return nil, err
			}
		}
	}
	for id, result := range outcome.ShowdownResults {
		if err := bump(id, KeyShowdownWarrior); err != nil {
			return nil, err
		}
		if result.Won {
			if result.Tied || result.SplitPot {
				if err := bump(id, KeyTied); err != nil {
					return nil, err
				}
			}
			continue
		}
		if result.Category == "straight_flush" && winnerHasCategory(outcome, "royal_flush") {
			if err := bump(id, KeyLostStraightFlushToRoyal); err != nil {
				return nil, err
			}
		}
		if hasCompleteCards(outcome, id) && lostRiverAfterLeadingTurn(outcome, id) {
			if err := bump(id, KeyLostRiverAfterLeadingTurn); err != nil {
				return nil, err
			}
		}
		if err := bump(id, KeyLooser); err != nil {
			return nil, err
		}
		if lostToSameCategory(outcome, id, result.Category) {
			if err := bump(id, KeyAlmostWinner); err != nil {
				return nil, err
			}
		}
		if categoryAtLeast(result.Category, "three_of_a_kind") {
			if err := bump(id, KeyBadBeat); err != nil {
				return nil, err
			}
		}
		if categoryAtLeast(result.Category, "full_house") {
			if err := bump(id, KeyCooler); err != nil {
				return nil, err
			}
		}
		if hi, ok := outcome.PlayerHands[id]; ok && isPocketAces(hi.HoleCards) {
			if err := bump(id, KeyCrackedAces); err != nil {
				return nil, err
			}
		}
		if hi, ok := outcome.PlayerHands[id]; ok && isPocketKings(hi.HoleCards) {
			if err := bump(id, KeyFallenKing); err != nil {
				return nil, err
			}
		}
	}
	var metrics []HandMetric
	if len(metricSets) > 0 {
		metrics = metricSets[0]
	}
	for _, metric := range metrics {
		if metric.PlayerID == "" {
			continue
		}
		if outcome.WonWithoutShowdown && winnerSet[metric.PlayerID] && metric.ThreeBet {
			if err := bump(metric.PlayerID, KeyThreeBetWonNoShowdown); err != nil {
				return nil, err
			}
		}
		if err := streak(metric.PlayerID, KeyFoldedStreak, metric.VPIP, 0); err != nil {
			return nil, err
		}
	}
	for _, id := range dedupe(outcome.Participants) {
		if !hasCompleteCards(outcome, id) {
			continue
		}
		cards, _ := playerCards(outcome, id)
		if fourToRoyalMissed(cards) {
			if err := bump(id, KeyFourToRoyalMissed); err != nil {
				return nil, err
			}
		}
		if fourToStraightFlushMissed(cards) {
			if err := bump(id, KeyFourToStraightFlushMissed); err != nil {
				return nil, err
			}
		}
		if _, ok := outcome.ShowdownResults[id]; ok && riverDrawMissed(cards[:6], cards[6]) {
			if err := bump(id, KeyPaidRiverDrawMissed); err != nil {
				return nil, err
			}
		}
		rank, paired := pocketPairRank(outcome.PlayerHands[id].HoleCards)
		stateKey := mode + "#" + id
		last := s.lastPocketPairFor(ctx, stateKey)
		qualifies := winnerSet[id] && paired
		reset, resetTo := true, 0
		next := byte(0)
		if qualifies {
			resetTo = 1
			if last == rank {
				reset = false
			}
			next = rank
		}
		s.storeLastPocketPair(ctx, stateKey, next)
		if err := streak(id, KeySamePocketPairStreak, reset, resetTo); err != nil {
			return nil, err
		}
	}
	return unlocks, nil
}

// RecordTableStreak advances every participant's running per-table win/loss
// streak (positive = consecutive wins, negative = consecutive losses) and
// returns each player's new value. Unlike RecordHand this is not a
// tiered/starred achievement — it is display state the Seat badge reads
// directly (Table.CurrentStreak), so there is no TierCrossed/unlock here.
func (s *Service) RecordTableStreak(ctx context.Context, tableID, mode string, outcome hand.HandOutcome) (map[string]int, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("achievements: progress store is required")
	}
	winnerSet := stringSet(outcome.Winners)
	streaks := make(map[string]int, len(outcome.Participants))
	for _, id := range dedupe(outcome.Participants) {
		current, err := s.store.UpdateTableStreak(ctx, id, mode, tableID, winnerSet[id])
		if err != nil {
			return nil, fmt.Errorf("achievements: table %s player %s streak: %w", tableID, id, err)
		}
		streaks[id] = current
	}
	return streaks, nil
}

func lostToSameCategory(outcome hand.HandOutcome, playerID, category string) bool {
	if len(outcome.PotResults) == 0 {
		return category != "" && category == outcome.WinningCategory
	}
	for _, pot := range outcome.PotResults {
		eligible := slices.Contains(pot.EligiblePlayerIDs, playerID)
		if !eligible {
			continue
		}
		for _, winner := range pot.Winners {
			if result, ok := outcome.ShowdownResults[winner]; ok && result.Category == category {
				return true
			}
		}
	}
	return false
}

// categoryAtLeast reports whether category is at least as strong as floor,
// per categoryOrder (catalog.go).
func categoryAtLeast(category, floor string) bool {
	at, af := -1, -1
	for i, c := range categoryOrder {
		if c == category {
			at = i
		}
		if c == floor {
			af = i
		}
	}
	return at >= 0 && af >= 0 && at >= af
}

func isPocketAces(holeCards [2]string) bool {
	return len(holeCards[0]) == 2 && len(holeCards[1]) == 2 &&
		holeCards[0][0] == 'A' && holeCards[1][0] == 'A'
}

func isPocketKings(holeCards [2]string) bool {
	return len(holeCards[0]) == 2 && len(holeCards[1]) == 2 &&
		holeCards[0][0] == 'K' && holeCards[1][0] == 'K'
}

func isPocketPair(cards [2]string) bool {
	_, ok := pocketPairRank(cards)
	return ok
}

func pocketPairRank(cards [2]string) (byte, bool) {
	return cards[0][0], len(cards[0]) == 2 && len(cards[1]) == 2 && cards[0][0] == cards[1][0]
}

func stringSet(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func winnerHasCategory(outcome hand.HandOutcome, category string) bool {
	for _, id := range outcome.Winners {
		if outcome.ShowdownResults[id].Category == category {
			return true
		}
	}
	return false
}

func beatOpponent(outcome hand.HandOutcome, winner string, predicate func(string, hand.ShowdownResult) bool) bool {
	for id, result := range outcome.ShowdownResults {
		if id != winner && !result.Won && predicate(id, result) {
			return true
		}
	}
	return false
}

// wonAgainstBiggerStack reports whether playerID (already known to be a
// comeback winner, i.e. won after going all-in) faced an opponent who
// contributed more chips to this hand than they did — a proxy for "beat a
// bigger stack" without needing pre-hand stack snapshots.
func wonAgainstBiggerStack(outcome hand.HandOutcome, playerID string) bool {
	self := outcome.Contributions[playerID]
	for _, opp := range outcome.Participants {
		if opp != playerID && outcome.Contributions[opp] > self {
			return true
		}
	}
	return false
}

func TierCrossed(key string, previousTotal, newTotal int) (int, bool) {
	for _, achievement := range Catalog {
		if achievement.Key != key {
			continue
		}
		stars := 0
		for _, tier := range achievement.Tiers {
			if previousTotal < tier.Threshold && newTotal >= tier.Threshold && tier.Stars > stars {
				stars = tier.Stars
			}
		}
		return stars, stars > 0
	}
	return 0, false
}

func dedupe(ids []string) []string {
	seen := make(map[string]bool, len(ids))
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		if id != "" && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
