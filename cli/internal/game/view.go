package game

import "gopkg.aoctech.app/poker/cli/internal/proto"

// PlayerView is one seat, flattened for rendering.
type PlayerView struct {
	ID         string
	Name       string
	Stack      int64
	Committed  int64
	Position   string // D, SB, BB, D/SB, UTG, UTG+1, UTG+2, MP, HJ, CO, or ""
	Folded     bool
	SittingOut bool
	IsYou      bool
	TimeBankMS int64 // durable decision reserve remaining; 0 = exhausted/none
}

// TableView is one TableSnapshot flattened into what the terminal renders.
type TableView struct {
	RoomID    string
	RoomName  string
	RealMoney bool

	SmallBlind int64
	BigBlind   int64
	Seated     int
	MaxSeats   int

	Pot   int64
	Board []string
	Stage string

	Players       []PlayerView
	You           PlayerView
	CurrentPlayer PlayerView
	YourHole      []string
	YourStrength  string
	YourEquity    float64 // -1 when the snapshot carries no equity for the viewer

	IsYourTurn bool
	Legal      *proto.LegalActions // nil when it isn't a betting decision point

	ActionDeadlineMS int64 // final decision deadline (base clock + time bank)
	BaseDeadlineMS   int64 // end of the base room clock; the gap to ActionDeadlineMS is the actor's time bank
	IdleRemovalMS    int64 // when the idle actor gets removed; 0 = no pending removal

	// Optimistic-concurrency preconditions for the next `act` message.
	SnapshotVersion uint64
	HandID          string
}

// NewTableView flattens s for viewer youID. blinds is [small, big]; realMoney
// controls whether the renderer prefixes amounts with a currency symbol.
func NewTableView(s *proto.TableSnapshot, youID, roomName string, realMoney bool, blinds [2]int64, maxSeats int, mode CardMode) TableView {
	seatIDs := make([]string, 0, len(s.Seats))
	for _, seat := range s.Seats {
		seatIDs = append(seatIDs, seat.PlayerId)
	}
	positions := Positions(seatIDs, s.DealerPlayerId, s.SmallBlindPlayerId, s.BigBlindPlayerId)

	v := TableView{
		RoomID:           roomName,
		RoomName:         roomName,
		RealMoney:        realMoney,
		SmallBlind:       blinds[0],
		BigBlind:         blinds[1],
		Seated:           len(s.Seats),
		MaxSeats:         maxSeats,
		Board:            s.Board,
		Stage:            s.Stage,
		IsYourTurn:       s.CurrentPlayerId == youID && youID != "",
		YourEquity:       -1,
		ActionDeadlineMS: s.ActionDeadlineUnixMs,
		BaseDeadlineMS:   s.ActionBaseDeadlineUnixMs,
		IdleRemovalMS:    s.IdleRemovalUnixMs,
		SnapshotVersion:  s.SnapshotVersion,
		HandID:           s.HandId,
	}
	for _, p := range s.Pots {
		v.Pot += p.Amount
	}
	v.Legal = s.LegalActions

	for _, seat := range s.Seats {
		pv := PlayerView{
			ID:         seat.PlayerId,
			Name:       seat.Name,
			Stack:      seat.Stack,
			Committed:  seat.Contributed,
			Position:   positions[seat.PlayerId],
			Folded:     seat.State == "folded",
			SittingOut: seat.State == "sitting_out" || (seat.Ready != nil && !*seat.Ready),
			IsYou:      seat.PlayerId == youID,
			TimeBankMS: seat.TimeBankMs,
		}
		v.Players = append(v.Players, pv)
		if pv.IsYou {
			v.You = pv
			v.YourHole = seat.HoleCards
			if len(seat.HoleCards) == 2 {
				v.YourStrength = HandStrength(seat.HoleCards, s.Board)
			}
			if seat.Equity != nil {
				v.YourEquity = *seat.Equity
			}
		}
		if pv.ID == s.CurrentPlayerId {
			v.CurrentPlayer = pv
		}
	}
	return v
}

// earlyLabels name seats forward from UTG; lateLabels name the two seats
// right of the button. Any seat between the two groups is the middle ("MP").
var earlyLabels = []string{"UTG", "UTG+1", "UTG+2", "UTG+3", "UTG+4"}
var lateLabels = []string{"CO", "HJ"}

// Positions assigns a role tag to every seat id. seatIDs must be in table
// seating order. Heads-up collapses D and SB onto one seat. Non-blind,
// non-button seats are named from both ends — CO/HJ/MP approaching the
// button, UTG/UTG+1/... leaving the big blind — so the same list gives the
// conventional 6-max (UTG/MP/CO) and 9-max (UTG..UTG+2/MP/HJ/CO) names.
func Positions(seatIDs []string, dealerID, sbID, bbID string) map[string]string {
	out := make(map[string]string, len(seatIDs))
	n := len(seatIDs)
	if n == 0 {
		return out
	}

	if n == 2 {
		for _, id := range seatIDs {
			switch id {
			case dealerID:
				out[id] = "D/SB"
			case bbID:
				out[id] = "BB"
			}
		}
		return out
	}

	for _, id := range seatIDs {
		switch id {
		case dealerID:
			out[id] = "D"
		case sbID:
			out[id] = "SB"
		case bbID:
			out[id] = "BB"
		}
	}

	// The remaining seats, in seating order starting just after the big blind.
	bbIdx := indexOf(seatIDs, bbID)
	if bbIdx < 0 {
		bbIdx = (indexOf(seatIDs, dealerID) + 2) % n
	}
	var early []string
	for step := 1; step <= n; step++ {
		id := seatIDs[(bbIdx+step)%n]
		if _, taken := out[id]; taken {
			continue
		}
		early = append(early, id)
	}
	if len(early) == 0 {
		return out
	}

	// CO/HJ take the seats right of the button; UTG/UTG+1/... fill forward
	// from the big blind; any single seat left between them is the middle.
	// HJ only appears once the table is big enough that CO+HJ+MP+UTG are all
	// distinct seats (7-handed and up); 6-max names its three middle seats
	// UTG/MP/CO with no hijack.
	lateCount := 0
	switch {
	case len(early) >= 4:
		lateCount = 2
	case len(early) >= 2:
		lateCount = 1
	}
	for i := 0; i < lateCount; i++ {
		out[early[len(early)-1-i]] = lateLabels[i]
	}
	middle := early[:len(early)-lateCount]
	for i, id := range middle {
		switch {
		case i == len(middle)-1 && len(middle) >= 2:
			out[id] = "MP"
		case i < len(earlyLabels):
			out[id] = earlyLabels[i]
		default:
			out[id] = "MP"
		}
	}
	return out
}

func indexOf(ss []string, s string) int {
	for i, v := range ss {
		if v == s {
			return i
		}
	}
	return -1
}
