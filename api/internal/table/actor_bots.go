package table

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"gopkg.aoctech.app/poker/api/internal/engine/betting"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/pokerbot"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
)

const botFillDelay = 15 * time.Second
const BotReservationTTL = 2 * time.Minute

var botNames = []string{"Lia", "Caio", "Bia", "Nando", "Maya", "Theo", "Iris", "Davi"}

type botRandom struct{}

func (botRandom) Float64() float64 { return rand.Float64() }
func (botRandom) IntN(n int) int   { return rand.IntN(n) }

func (a *Actor) handleReserveBotSeat(ctx context.Context, c ReserveBotSeatCmd) error {
	if err := a.ensureLoaded(ctx, true); err != nil {
		return err
	}
	apply := func() error {
		return a.mutate(func() error {
			if err := a.cached.ReserveBotSeatForActor(c.Reservation, timeNowFunc().UnixMilli()); err != nil {
				return err
			}
			return a.commit(ctx, "", &tablestore.ActionLogEntry{PlayerID: c.Reservation.PlayerID, Action: "reserve_bot_seat"})
		})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		return err
	}
	if c.Result != nil {
		if reservation := a.cached.BotReservationForActor(); reservation != nil {
			c.Result <- *reservation
		}
	}
	a.broadcastAll()
	a.notifySeatsChanged()
	a.armBotReservationTimer()
	return nil
}

func (a *Actor) armBotReservationTimer() {
	if a.cached == nil {
		return
	}
	reservation := a.cached.BotReservationForActor()
	if reservation == nil {
		if a.botReservationTimer != nil {
			a.botReservationTimer.Stop()
		}
		a.botReservationArmedFor = ""
		return
	}
	key := fmt.Sprintf("%s:%d", reservation.ID, reservation.ExpiresAtUnixMs)
	if a.botReservationArmedFor == key {
		return
	}
	if a.botReservationTimer != nil {
		a.botReservationTimer.Stop()
	}
	a.botReservationArmedFor = key
	delay := time.Until(time.UnixMilli(reservation.ExpiresAtUnixMs))
	if delay < 0 {
		delay = 0
	}
	a.botReservationTimer = time.AfterFunc(delay, func() {
		reply := make(chan error, 1)
		if err := a.Dispatch(expireBotReservationCmd{ReservationID: reservation.ID, Reply: reply}); err != nil {
			slog.Warn("table bot reservation expiry dispatch failed", "table_id", a.id, "err", err)
		}
	})
}

func (a *Actor) handleExpireBotReservation(ctx context.Context, c expireBotReservationCmd) error {
	a.botReservationArmedFor = ""
	if err := a.ensureLoaded(ctx, true); err != nil {
		return err
	}
	reservation := a.cached.BotReservationForActor()
	if reservation == nil || reservation.ID != c.ReservationID || reservation.ExpiresAtUnixMs > timeNowFunc().UnixMilli() {
		a.armBotReservationTimer()
		return nil
	}
	if err := a.mutate(func() error {
		a.cached.CancelBotReservationForActor(reservation.ID, reservation.PlayerID)
		return a.commit(ctx, "", &tablestore.ActionLogEntry{PlayerID: reservation.PlayerID, Action: "expire_bot_reservation"})
	}); err != nil {
		return err
	}
	a.armBotFillTimer()
	a.broadcastAll()
	return nil
}

func (a *Actor) handleBotReservationStatus(ctx context.Context, c BotReservationStatusCmd) error {
	if err := a.ensureLoaded(ctx, true); err != nil {
		return err
	}
	reservation := a.cached.BotReservationForActor()
	if reservation == nil || reservation.PlayerID != c.PlayerID {
		c.Status <- BotReservationStatus{}
		return nil
	}
	if reservation.ExpiresAtUnixMs <= timeNowFunc().UnixMilli() {
		if err := a.mutate(func() error {
			a.cached.CancelBotReservationForActor(reservation.ID, c.PlayerID)
			return a.commit(ctx, "", &tablestore.ActionLogEntry{PlayerID: c.PlayerID, Action: "expire_bot_reservation"})
		}); err != nil {
			return err
		}
		a.armBotFillTimer()
		a.armBotReservationTimer()
		c.Status <- BotReservationStatus{}
		return nil
	}
	c.Status <- BotReservationStatus{Reservation: reservation, Ready: a.cached.BotReservationReadyForActor()}
	return nil
}

func (a *Actor) handleBotMatchStatus(ctx context.Context, c BotMatchStatusCmd) error {
	if err := a.ensureLoaded(ctx, true); err != nil {
		return err
	}
	status := BotMatchStatus{Reservation: a.cached.BotReservationForActor(), BotPolicy: a.cached.BotPolicyForActor()}
	for _, player := range a.cached.PlayersForActor() {
		status.HasBot = status.HasBot || player.IsBot
	}
	c.Status <- status
	return nil
}

func (a *Actor) handleCancelBotReservation(ctx context.Context, c CancelBotReservationCmd) error {
	if err := a.ensureLoaded(ctx, true); err != nil {
		return err
	}
	if err := a.mutate(func() error {
		if !a.cached.CancelBotReservationForActor(c.ReservationID, c.PlayerID) {
			return fmt.Errorf("table: bot reservation not found")
		}
		return a.commit(ctx, "", &tablestore.ActionLogEntry{PlayerID: c.PlayerID, Action: "cancel_bot_reservation"})
	}); err != nil {
		return err
	}
	a.armBotFillTimer()
	a.armBotReservationTimer()
	a.broadcastAll()
	return nil
}

func (a *Actor) handleEnableBots(ctx context.Context, c EnableBotsCmd) error {
	if err := a.ensureLoaded(ctx, true); err != nil {
		return err
	}
	apply := func() error {
		return a.mutate(func() error {
			activateAt := timeNowFunc().Add(botFillDelay).UnixMilli()
			if err := a.cached.ConfigureBotsForActor(c.OwnerID, c.BuyIn, c.MaxSeats, activateAt); err != nil {
				return err
			}
			return a.commit(ctx, "", &tablestore.ActionLogEntry{PlayerID: c.OwnerID, Action: "enable_bots"})
		})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		return err
	}
	a.armBotFillTimer()
	a.broadcastAll()
	return nil
}

func (a *Actor) handleStartBotsNow(ctx context.Context, c StartBotsNowCmd) error {
	if err := a.ensureLoaded(ctx, true); err != nil {
		return err
	}
	apply := func() error {
		return a.mutate(func() error {
			if err := a.cached.ExpediteBotsForActor(c.OwnerID, timeNowFunc().UnixMilli()); err != nil {
				return err
			}
			return a.commit(ctx, "", &tablestore.ActionLogEntry{PlayerID: c.OwnerID, Action: "start_bots_now"})
		})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		return err
	}
	return a.handleFillBots(ctx)
}

func (a *Actor) armBotFillTimer() {
	if a.cached == nil {
		return
	}
	policy := a.cached.BotPolicyForActor()
	if !policy.Enabled || a.cached.BotSeatsNeededForActor() == 0 {
		if a.botFillTimer != nil {
			a.botFillTimer.Stop()
		}
		a.botFillArmedFor = 0
		return
	}
	if a.botFillArmedFor == policy.ActivateAtUnixMs {
		return
	}
	if a.botFillTimer != nil {
		a.botFillTimer.Stop()
	}
	a.botFillArmedFor = policy.ActivateAtUnixMs
	delay := time.Until(time.UnixMilli(policy.ActivateAtUnixMs))
	if delay < 0 {
		delay = 0
	}
	a.botFillTimer = time.AfterFunc(delay, func() {
		reply := make(chan error, 1)
		if err := a.Dispatch(fillBotsCmd{Reply: reply}); err != nil {
			slog.Warn("table bot fill dispatch failed", "table_id", a.id, "err", err)
		}
	})
}

func (a *Actor) handleFillBots(ctx context.Context) error {
	a.botFillArmedFor = 0
	if err := a.ensureLoaded(ctx, true); err != nil {
		return err
	}
	policy := a.cached.BotPolicyForActor()
	if !policy.Enabled || policy.ActivateAtUnixMs > timeNowFunc().UnixMilli() || a.cached.BotSeatsNeededForActor() == 0 {
		a.armBotFillTimer()
		return nil
	}
	if a.botFundingAvailable != nil {
		available, err := a.botFundingAvailable(ctx, policy.OwnerID)
		if err != nil || !available {
			if err != nil {
				slog.Warn("bot funding availability failed closed", "table_id", a.id, "err", err)
			}
			if commitErr := a.mutate(func() error {
				a.cached.DisableBotsForActor()
				return a.commit(ctx, "", &tablestore.ActionLogEntry{PlayerID: policy.OwnerID, Action: "disable_bots_funding_limit"})
			}); commitErr != nil {
				return commitErr
			}
			a.broadcastAll()
			return nil
		}
	}
	apply := func() error {
		return a.mutate(func() error {
			needed := a.cached.BotSeatsNeededForActor()
			for i := 0; i < needed; i++ {
				index := nextBotIndex(a.cached.PlayersForActor(), policy.OwnerID)
				profile := pokerbot.Profiles[index%len(pokerbot.Profiles)]
				if err := a.cached.AddBotForActor(
					fmt.Sprintf("bot:%s:%d", policy.OwnerID, index),
					botNames[index%len(botNames)], profile.ID,
				); err != nil {
					return err
				}
			}
			a.tryStartHand(ctx)
			return a.commit(ctx, "", &tablestore.ActionLogEntry{Action: "fill_bots"})
		})
	}
	if err := a.retryOnConflict(ctx, apply); err != nil {
		return err
	}
	a.notifySeatsChanged()
	a.broadcastAll()
	return nil
}

func (a *Actor) enforceBotFunding(ctx context.Context) {
	if a.cached == nil || a.cached.Stage() != hand.Complete || a.handID == "" || a.botFundingCheckedFor == a.handID {
		return
	}
	if a.botFundingRecord == nil {
		return
	}
	outcome := a.cached.LastOutcomeForActor()
	if outcome == nil || !outcome.ContainsBot {
		return
	}
	if a.cached.BotPolicyForActor().FundingCheckedHandID == a.handID {
		a.botFundingCheckedFor = a.handID
		return
	}
	playerID := ""
	for _, id := range outcome.Participants {
		if !strings.HasPrefix(id, "bot:") {
			if playerID != "" {
				// Mixed human/bot hands are forbidden by policy. If an old rollout
				// produces one, stop bots rather than attribute profit ambiguously.
				playerID = ""
				break
			}
			playerID = id
		}
	}
	allowed := false
	if playerID != "" {
		delta := outcome.Payouts[playerID] - outcome.Contributions[playerID]
		var err error
		allowed, err = a.botFundingRecord(ctx, playerID, a.handID, delta)
		if err != nil {
			slog.Warn("bot funding update failed closed", "table_id", a.id, "hand_id", a.handID, "err", err)
		}
	}
	if err := a.mutate(func() error {
		a.cached.MarkBotFundingCheckedForActor(a.handID)
		if !allowed {
			if err := a.cached.RetireBotsForHumanArrival(); err != nil {
				return err
			}
			a.cached.DisableBotsForActor()
		}
		return a.commit(ctx, "", &tablestore.ActionLogEntry{PlayerID: playerID, Action: "check_bot_funding"})
	}); err != nil {
		slog.Error("failed to retire bots after funding decision", "table_id", a.id, "hand_id", a.handID, "err", err)
		return
	}
	a.botFundingCheckedFor = a.handID
}

func nextBotIndex(players []*hand.Player, ownerID string) int {
	for i := 0; ; i++ {
		candidate := fmt.Sprintf("bot:%s:%d", ownerID, i)
		found := false
		for _, p := range players {
			found = found || p.ID == candidate
		}
		if !found {
			return i
		}
	}
}

func (a *Actor) armBotActionTimer() {
	if a.cached == nil {
		return
	}
	current := a.cached.CurrentPlayerIDForActor()
	bot, ok := a.cached.BotForActor(current)
	key := fmt.Sprintf("%s:%s:%d", a.handID, current, a.version)
	if !ok || current == "" {
		if a.botActionTimer != nil {
			a.botActionTimer.Stop()
		}
		a.botActionArmedFor = ""
		return
	}
	if a.botActionArmedFor == key {
		return
	}
	if a.botActionTimer != nil {
		a.botActionTimer.Stop()
	}
	remaining := a.turnDeadline.Sub(timeNowFunc())
	if remaining <= 0 {
		remaining = a.turnTimeout
	}
	delay := pokerbot.ThinkDelay(profileByID(bot.BotProfile), randomThinkKind(), remaining, botRandom{})
	a.botActionArmedFor = key
	handID := a.handID
	version := a.version
	a.botActionTimer = time.AfterFunc(delay, func() {
		reply := make(chan error, 1)
		if err := a.Dispatch(botActCmd{PlayerID: current, HandID: handID, Version: version, Reply: reply}); err != nil {
			slog.Warn("table bot action dispatch failed", "table_id", a.id, "player_id", current, "err", err)
		}
	})
}

func randomThinkKind() pokerbot.ThinkKind {
	x := rand.Float64()
	switch {
	case x < .12:
		return pokerbot.ThinkRoutine
	case x < .84:
		return pokerbot.ThinkNormal
	case x < .97:
		return pokerbot.ThinkLarge
	default:
		return pokerbot.ThinkHesitation
	}
}

func profileByID(id string) pokerbot.Profile {
	for _, profile := range pokerbot.Profiles {
		if profile.ID == id {
			return profile
		}
	}
	return pokerbot.Profiles[0]
}

func (a *Actor) handleBotAct(ctx context.Context, c botActCmd) error {
	a.botActionArmedFor = ""
	if err := a.ensureLoaded(ctx, true); err != nil {
		return err
	}
	if a.handID != c.HandID || a.version != c.Version || a.cached.CurrentPlayerIDForActor() != c.PlayerID {
		a.armBotActionTimer()
		return nil
	}
	bot, ok := a.cached.BotForActor(c.PlayerID)
	if !ok {
		return nil
	}
	view := a.cached.ViewFor(c.PlayerID)
	strength := .5
	if hole, board, available := a.cached.HoleAndBoardForActor(c.PlayerID); available {
		opponents := 0
		for _, seat := range view.Seats {
			if seat.PlayerID != c.PlayerID && (seat.State == "active" || seat.State == "all_in") {
				opponents++
			}
		}
		if opponents > 0 {
			if estimate, estimated := a.equityFor(hole, board, opponents); estimated {
				strength = estimate
			}
		}
	}
	var pot int64
	for _, p := range view.Pots {
		pot += p.Amount
	}
	decision, err := pokerbot.Decide(profileByID(bot.BotProfile), pokerbot.Context{
		Legal: view.LegalActions, Strength: strength, Pot: pot, Stack: bot.Stack,
	}, botRandom{})
	if err != nil {
		return err
	}
	action := betting.Action(strings.ToLower(decision.Action))
	actionID := fmt.Sprintf("bot-act-%s-%s-%d", a.handID, c.PlayerID, a.version)
	_, err = a.applyActAndCommit(ctx, ActCmd{PlayerID: c.PlayerID, ActionID: actionID, Action: action, Amount: decision.Amount})
	if err != nil {
		return err
	}
	if err := a.commitOutcomeLogEntries(ctx); err != nil {
		return err
	}
	a.broadcastAll()
	return nil
}

func (a *Actor) armBotPostHandTimer() {
	if a.cached == nil || a.cached.Stage() != hand.Complete || a.handID == "" || a.botPostHandArmedFor == a.handID {
		return
	}
	hasBot := false
	for _, p := range a.cached.PlayersForActor() {
		hasBot = hasBot || p.IsBot
	}
	if !hasBot {
		return
	}
	a.botPostHandArmedFor = a.handID
	handID, version := a.handID, a.version
	delay := 800*time.Millisecond + time.Duration(rand.Float64()*1200)*time.Millisecond
	a.botPostHandTimer = time.AfterFunc(delay, func() {
		reply := make(chan error, 1)
		if err := a.Dispatch(botPostHandCmd{HandID: handID, Version: version, Reply: reply}); err != nil {
			slog.Warn("table bot post-hand dispatch failed", "table_id", a.id, "hand_id", handID, "err", err)
		}
	})
}

func (a *Actor) handleBotPostHand(ctx context.Context, c botPostHandCmd) error {
	if err := a.ensureLoaded(ctx, true); err != nil {
		return err
	}
	// Cosmetic activity or the funding check can advance the table version
	// while this timer waits. The hand ID and stage are the durable stale-work
	// boundary; a version mismatch alone must not suppress post-hand behavior.
	if a.handID != c.HandID || a.cached.Stage() != hand.Complete {
		return nil
	}
	outcome := a.cached.LastOutcomeForActor()
	if outcome == nil {
		return nil
	}
	winners := stringSet(outcome.Winners)
	allIn := stringSet(outcome.AllInPlayers)
	changed := false
	err := a.mutate(func() error {
		for _, bot := range a.cached.PlayersForActor() {
			if !bot.IsBot || bot.PendingExit {
				continue
			}
			profile := profileByID(bot.BotProfile)
			switch pokerbot.RevealChoice(profile, botRandom{}) {
			case pokerbot.RevealBoth:
				if applied, revealErr := a.cached.RevealHoleCard(bot.ID, nil); revealErr == nil {
					changed = changed || applied
				}
			case pokerbot.RevealOne:
				index := int32(rand.IntN(2))
				if applied, revealErr := a.cached.RevealHoleCard(bot.ID, &index); revealErr == nil {
					changed = changed || applied
				}
			}
			wonAllIn := winners[bot.ID] && allIn[bot.ID]
			if pokerbot.ShouldCashOut(profile, pokerbot.ExitContext{
				WonAllIn: wonAllIn, HitAndRun: profile.ID == "volatile",
				StackTooShort: bot.Stack < max64(1, bot.BuyInAmount/4),
			}, botRandom{}) {
				if exitErr := a.cached.RequestExit(bot.ID); exitErr != nil {
					return exitErr
				}
				changed = true
			}
		}
		if !changed {
			return nil
		}
		return a.commit(ctx, fmt.Sprintf("bot-post-%s", c.HandID), &tablestore.ActionLogEntry{Action: "bot_post_hand"})
	})
	if err != nil {
		return err
	}
	if changed {
		a.broadcastAll()
	}
	a.maybeReactAfterBotHand(ctx, c.HandID, outcome)
	return nil
}

func (a *Actor) maybeReactAfterBotHand(ctx context.Context, handID string, outcome *hand.HandOutcome) {
	policy := a.cached.BotPolicyForActor()
	if !outcome.ContainsBot || policy.LastReactionHandID == handID ||
		timeNowFunc().UnixMilli()-policy.LastReactionAtUnixMs < int64((90*time.Second).Milliseconds()) {
		return
	}
	var botID, humanID string
	humanPriority := -1
	botWon := false
	winners := stringSet(outcome.Winners)
	participants := stringSet(outcome.Participants)
	allInPlayers := stringSet(outcome.AllInPlayers)
	for _, p := range a.cached.PlayersForActor() {
		if !participants[p.ID] {
			continue
		}
		if p.IsBot {
			if botID == "" || winners[p.ID] {
				botID = p.ID
			}
			botWon = botWon || winners[p.ID]
		} else {
			priority := 0
			if allInPlayers[p.ID] {
				priority = 1
			}
			if winners[p.ID] {
				priority = 2
			}
			if priority > humanPriority {
				humanID, humanPriority = p.ID, priority
			}
		}
	}
	if botID == "" || humanID == "" {
		return
	}
	moment := pokerbot.ReactionHumanWon
	if botWon {
		moment = pokerbot.ReactionBotWon
	}
	if len(outcome.AllInPlayers) > 0 {
		moment = pokerbot.ReactionAllIn
	} else if outcome.WonWithoutShowdown {
		moment = pokerbot.ReactionUncontested
	}
	choice, ok := pokerbot.ChooseReaction(moment, botRandom{})
	if !ok {
		return
	}
	target := ""
	if choice.Targeted {
		target = humanID
	}
	if err := a.handleReaction(ctx, ReactionCmd{
		PlayerID: botID, ActionID: "bot-reaction-" + handID,
		ReactionID: choice.ID, TargetPlayerID: target,
		BotGenerated: true, BotHandID: handID,
	}); err != nil {
		slog.Warn("table bot reaction skipped", "table_id", a.id, "hand_id", handID, "err", err)
	}
}

func stringSet(ids []string) map[string]bool {
	out := make(map[string]bool, len(ids))
	for _, id := range ids {
		out[id] = true
	}
	return out
}

func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
