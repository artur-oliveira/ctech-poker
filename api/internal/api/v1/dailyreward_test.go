package v1

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/dailyreward"
)

type fakeDailyWallet struct{ fail bool }

func (f *fakeDailyWallet) Credit(context.Context, string, int64, string, string) error {
	if f.fail {
		return errors.New("wallet unavailable")
	}
	return nil
}

type fakeDailyStore struct {
	claims map[string]dailyreward.DailyRewardRecord
	streak dailyreward.StreakRecord
}

func (f *fakeDailyStore) Claim(_ context.Context, _, day string, amount int64, streak dailyreward.StreakRecord, _ time.Time) (dailyreward.DailyRewardRecord, error) {
	if r, ok := f.claims[day]; ok {
		return r, nil
	}
	r := dailyreward.DailyRewardRecord{Amount: amount, Status: dailyreward.StatusPending}
	f.claims[day] = r
	f.streak = streak
	return r, nil
}

func (f *fakeDailyStore) Complete(_ context.Context, _, day string, _ time.Time) error {
	r := f.claims[day]
	r.Status = dailyreward.StatusCompleted
	f.claims[day] = r
	return nil
}

func (f *fakeDailyStore) Get(_ context.Context, _, day string) (dailyreward.DailyRewardRecord, error) {
	return f.claims[day], nil
}

func (f *fakeDailyStore) LoadStreak(context.Context, string) (dailyreward.StreakRecord, error) {
	return f.streak, nil
}

func postDailyReward(t *testing.T, app *fiber.App) (int, int64) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest(http.MethodPost, "/v1.0/sandbox-credits/", nil))
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		Amount int64 `json:"amount"`
	}
	_ = json.NewDecoder(resp.Body).Decode(&body)
	return resp.StatusCode, body.Amount
}

// A claim whose wallet credit failed has already advanced the streak, so the
// handler must not read "claimed today" as "paid today": the next POST has to
// reach Spin's retry path and pay the pending prize.
func TestDailyRewardPostRetriesPendingClaim(t *testing.T) {
	wallet := &fakeDailyWallet{fail: true}
	store := &fakeDailyStore{claims: map[string]dailyreward.DailyRewardRecord{}}
	app := fiber.New()
	auth := func(c fiber.Ctx) error { c.Locals(localsUserID, "player-1"); return c.Next() }
	RegisterDailyReward(app.Group("/v1.0"), auth, dailyreward.NewService(wallet, store), nil)

	if code, _ := postDailyReward(t, app); code == http.StatusOK {
		t.Fatal("first claim should fail on the wallet")
	}
	wallet.fail = false
	code, amount := postDailyReward(t, app)
	if code != http.StatusOK || amount != dailyreward.FirstAward {
		t.Fatalf("retry: status=%d amount=%d, want 200 %d", code, amount, dailyreward.FirstAward)
	}
	if code, amount := postDailyReward(t, app); code != http.StatusOK || amount != 0 {
		t.Fatalf("paid day: status=%d amount=%d, want 200 0", code, amount)
	}
}
