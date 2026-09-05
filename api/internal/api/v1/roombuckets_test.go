package v1

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/table"
)

func bucketReq() JoinOrCreateRoomRequest {
	return JoinOrCreateRoomRequest{SmallBlind: 10, BigBlind: 20, MaxSeats: 6, CurrencyMode: "sandbox", Amount: 1000}
}

func bucketRoom(id string, seatsTaken int) roomstore.Room {
	return roomstore.Room{
		ID: id, Visibility: "public", CurrencyMode: "sandbox", SmallBlind: 10, BigBlind: 20,
		MaxSeats: 6, BuyInMin: 400, BuyInMax: 2000, SeatsTaken: seatsTaken,
	}
}

func TestOpenRoomsInBucketExcludesFullMismatchedAndOutOfRangeRooms(t *testing.T) {
	other := bucketRoom("other-stakes", 0)
	other.BigBlind = 50
	private := bucketRoom("private", 0)
	private.Visibility = "private"
	realMoney := bucketRoom("real", 0)
	realMoney.CurrencyMode = "real"
	narrow := bucketRoom("narrow", 0)
	narrow.BuyInMax = 500

	got := openRoomsInBucket([]roomstore.Room{
		bucketRoom("full", 6), other, private, realMoney, narrow,
		bucketRoom("empty", 0), bucketRoom("nearly-full", 5),
	}, bucketReq())

	if len(got) != 2 {
		t.Fatalf("expected only the two joinable sandbox rooms, got %+v", got)
	}
	// Fullest first, so players consolidate instead of scattering one per table.
	if got[0].ID != "nearly-full" || got[1].ID != "empty" {
		t.Fatalf("candidates must be ordered fullest-first, got %s then %s", got[0].ID, got[1].ID)
	}
}

// The seat race: the lobby page (or a rival join that landed first) says the
// room is open, the actor says otherwise. The caller must never see that.
func TestSeatInBucketSkipsRoomsThatLostTheSeatRace(t *testing.T) {
	var attempted []string
	roomID, created, err := seatInBucket(
		[]roomstore.Room{bucketRoom("stale", 5), bucketRoom("free", 0)},
		func(room roomstore.Room) error {
			attempted = append(attempted, room.ID)
			if room.ID == "stale" {
				return table.ErrNoSeatsAvailable
			}
			return nil
		},
		func() (string, error) { return "", errors.New("must not create when a table could seat the player") },
	)
	if err != nil || roomID != "free" || created {
		t.Fatalf("expected fallthrough to the next open table, got %q created=%v err=%v", roomID, created, err)
	}
	if len(attempted) != 2 {
		t.Fatalf("expected both candidates tried, got %v", attempted)
	}
}

func TestSeatInBucketCreatesWhenEveryCandidateIsFull(t *testing.T) {
	roomID, created, err := seatInBucket(
		[]roomstore.Room{bucketRoom("a", 5), bucketRoom("b", 5)},
		func(roomstore.Room) error { return table.ErrNoSeatsAvailable },
		func() (string, error) { return "fresh", nil },
	)
	if err != nil || roomID != "fresh" || !created {
		t.Fatalf("expected a fresh table, got %q created=%v err=%v", roomID, created, err)
	}
}

func TestSeatInBucketPropagatesRealBuyInFailures(t *testing.T) {
	// A wallet failure must never be retried against a sibling table.
	wallet := errors.New("insufficient funds")
	_, _, err := seatInBucket(
		[]roomstore.Room{bucketRoom("a", 0), bucketRoom("b", 0)},
		func(roomstore.Room) error { return wallet },
		func() (string, error) { return "", errors.New("must not create") },
	)
	if !errors.Is(err, wallet) {
		t.Fatalf("expected the buy-in error to surface, got %v", err)
	}
}

func TestAggregateBucketsCountsEveryPageAndScopesByCurrencyMode(t *testing.T) {
	realRoom := bucketRoom("real", 0)
	realRoom.CurrencyMode = "real"
	legacy := bucketRoom("legacy-no-mode", 1)
	legacy.CurrencyMode = "" // predates the field: sandbox by construction
	bigger := bucketRoom("bigger", 2)
	bigger.SmallBlind, bigger.BigBlind = 25, 50

	buckets := aggregateBuckets([]roomstore.Room{
		bucketRoom("page1", 3), bucketRoom("page2-full", 6), legacy, realRoom, bigger,
	}, "sandbox")

	if len(buckets) != 2 {
		t.Fatalf("expected one bucket per (blinds, seats), got %+v", buckets)
	}
	first := buckets[0]
	if first.BigBlind != 20 || first.Rooms != 3 || first.OpenRooms != 2 {
		t.Fatalf("open-room count must span every page and skip full tables: %+v", first)
	}
	if first.SeatsTaken != 10 || first.SeatsAvailable != 8 {
		t.Fatalf("seat aggregate wrong: %+v", first)
	}
	if buckets[1].BigBlind != 50 {
		t.Fatalf("buckets must be ordered by big blind, got %+v", buckets)
	}
}

func TestAggregateBucketsExcludesOtherCurrencyMode(t *testing.T) {
	if got := aggregateBuckets([]roomstore.Room{bucketRoom("sandbox-only", 0)}, "real"); len(got) != 0 {
		t.Fatalf("sandbox rooms must never appear in the real-money lobby: %+v", got)
	}
}

// The buckets aggregate is the only endpoint left that walks the whole public
// index, so it must not walk it once per request (#213).
func TestBucketCacheWalksThePublicIndexOncePerTTL(t *testing.T) {
	var cache bucketCache
	walks := 0
	load := func() ([]RoomBucket, error) {
		walks++
		return []RoomBucket{{BigBlind: 20}}, nil
	}
	for range 5 {
		if _, err := cache.get(roomstore.CurrencyModeSandbox, load); err != nil {
			t.Fatalf("get: %v", err)
		}
	}
	if walks != 1 {
		t.Fatalf("expected one index walk within the TTL, got %d", walks)
	}
	// Each currency mode is its own aggregate: one must never serve the other.
	if _, err := cache.get(roomstore.CurrencyModeReal, load); err != nil {
		t.Fatalf("get real: %v", err)
	}
	if walks != 2 {
		t.Fatalf("expected the real-money aggregate to load on its own, got %d walks", walks)
	}
	// A stale entry reloads instead of being served forever.
	cache.entries[roomstore.CurrencyModeSandbox] = bucketCacheEntry{
		at: time.Now().Add(-2 * bucketsCacheTTL), buckets: nil,
	}
	if _, err := cache.get(roomstore.CurrencyModeSandbox, load); err != nil {
		t.Fatalf("get after expiry: %v", err)
	}
	if walks != 3 {
		t.Fatalf("expected an expired aggregate to reload, got %d walks", walks)
	}
}

func TestBucketCacheDoesNotCacheAFailedWalk(t *testing.T) {
	var cache bucketCache
	boom := errors.New("dynamo down")
	if _, err := cache.get(roomstore.CurrencyModeSandbox, func() ([]RoomBucket, error) {
		return nil, boom
	}); !errors.Is(err, boom) {
		t.Fatalf("expected the load error to surface, got %v", err)
	}
	got, err := cache.get(roomstore.CurrencyModeSandbox, func() ([]RoomBucket, error) {
		return []RoomBucket{{BigBlind: 50}}, nil
	})
	if err != nil || len(got) != 1 || got[0].BigBlind != 50 {
		t.Fatalf("a failed walk must not be cached, got %+v err=%v", got, err)
	}
}

func postJoinOrCreate(t *testing.T, body string) int {
	t.Helper()
	app := fiber.New()
	h := &roomHandlers{}
	app.Post("/rooms/join-or-create", func(c fiber.Ctx) error { c.Locals(localsUserID, "u1"); return c.Next() }, h.joinOrCreate)
	req := httptest.NewRequest(fiber.MethodPost, "/rooms/join-or-create", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode
}

func TestJoinOrCreateRejectsOffCatalogStake(t *testing.T) {
	if got := postJoinOrCreate(t, `{"small_blind":11,"big_blind":23,"max_seats":6,"amount":460}`); got != fiber.StatusBadRequest {
		t.Fatalf("got %d", got)
	}
}

func TestJoinOrCreateRejectsBuyInOutsideThePublicWindow(t *testing.T) {
	// 20 BB is the public-table minimum; 300 is below it.
	if got := postJoinOrCreate(t, `{"small_blind":10,"big_blind":20,"max_seats":6,"amount":300}`); got != fiber.StatusBadRequest {
		t.Fatalf("got %d", got)
	}
}

func TestJoinOrCreateRejectsUnsupportedSeatCount(t *testing.T) {
	if got := postJoinOrCreate(t, `{"small_blind":10,"big_blind":20,"max_seats":4,"amount":1000}`); got != fiber.StatusBadRequest {
		t.Fatalf("got %d", got)
	}
}
