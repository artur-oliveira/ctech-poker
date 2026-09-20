//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/api-commons/cache"
	v1 "gopkg.aoctech.app/poker/api/internal/api/v1"
	"gopkg.aoctech.app/poker/api/internal/buyin"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/engine/hand"
	"gopkg.aoctech.app/poker/api/internal/roomstore"
	"gopkg.aoctech.app/poker/api/internal/tablelease"
	"gopkg.aoctech.app/poker/api/internal/tablemanager"
	"gopkg.aoctech.app/poker/api/internal/tablestore"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

type rejectingBotWallet struct{ debits int }

func (w *rejectingBotWallet) Credit(context.Context, string, int64, string, string) error { return nil }
func (w *rejectingBotWallet) Debit(context.Context, string, int64, string, string) error {
	w.debits++
	return errors.New("wallet debit rejected")
}
func (w *rejectingBotWallet) HoldGame(context.Context, string, int64, string, string, string) (string, error) {
	return "", errors.New("unexpected real-money hold")
}
func (w *rejectingBotWallet) ReleaseHold(context.Context, string) error { return nil }
func (w *rejectingBotWallet) CashoutGame(context.Context, string, int64, string, []string, string, string) error {
	return errors.New("unexpected cashout")
}
func (w *rejectingBotWallet) DebitReal(context.Context, string, int64, string, string) error {
	return errors.New("unexpected real-money debit")
}
func (w *rejectingBotWallet) Balances(context.Context, string) (*walletclient.Balances, error) {
	return &walletclient.Balances{SandboxBalance: 10_000}, nil
}

func TestReservationHTTPDebitFailureReleasesSeat(t *testing.T) {
	ctx := context.Background()
	db := testDynamoClient(t)
	env := "flow_test"
	mustCreatePokerTables(t, db, env)
	roomTable := env + "_poker_rooms"
	_, err := db.CreateTable(ctx, &dynamodb.CreateTableInput{TableName: aws.String(roomTable),
		AttributeDefinitions: []types.AttributeDefinition{{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS}, {AttributeName: aws.String("sk"), AttributeType: types.ScalarAttributeTypeS}},
		KeySchema:            []types.KeySchemaElement{{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash}, {AttributeName: aws.String("sk"), KeyType: types.KeyTypeRange}},
		BillingMode:          types.BillingModePayPerRequest})
	var exists *types.ResourceInUseException
	if err != nil && !errors.As(err, &exists) {
		t.Fatal(err)
	}
	roomID := uniqueTableID(t)
	rooms := roomstore.NewStore(db, env)
	if err := rooms.Create(ctx, roomstore.Room{ID: roomID, Visibility: "public", CurrencyMode: "sandbox",
		SmallBlind: 25, BigBlind: 50, MaxSeats: 2, BuyInMin: 1_000, BuyInMax: 5_000, Status: "waiting", CreatedBy: "owner"}); err != nil {
		t.Fatal(err)
	}
	game := hand.NewTable([]*hand.Player{{ID: "owner", Stack: 4_000, Ready: true}}, 25, 50)
	game.ConfigureRake("sandbox")
	if err := game.ConfigureBotsForActor("owner", 4_000, 2, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if err := game.AddBotForActor("bot:owner:0", "Lia", "tag"); err != nil {
		t.Fatal(err)
	}
	reservation := hand.BotReservation{ID: "http-failed-debit", PlayerID: "visitor", Amount: 4_000,
		IdempotencyKey: "visitor-entry", ExpiresAtUnixMs: time.Now().Add(time.Minute).UnixMilli()}
	if err := game.ReserveBotSeatForActor(reservation, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := game.RemovePlayerForActor("bot:owner:0"); err != nil {
		t.Fatal(err)
	}
	if !game.BotReservationReadyForActor() {
		t.Fatal("fixture must be ready to confirm")
	}
	store := tablestore.NewStore(db, env)
	if err := store.SeedTable(ctx, roomID, game.ExportState()); err != nil {
		t.Fatal(err)
	}
	mgr := tablemanager.NewManager(tablelease.NewService(cache.NewMemoryBackend(16)), store, nil, nil)
	wallet := &rejectingBotWallet{}
	service := buyin.NewService(wallet, mgr, rooms)
	app := fiber.New()
	v1.RegisterRooms(app, func(c fiber.Ctx) error { c.Locals("user_id", "visitor"); return c.Next() }, rooms, service, mgr, nil,
		&config.Config{SandboxBotsEnabled: true}, nil, nil, nil, nil)
	request := httptest.NewRequest(fiber.MethodGet, "/rooms/"+roomID+"/reservations/"+reservation.ID, nil)
	response, err := app.Test(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var result v1.BotReservationResponse
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != fiber.StatusOK || result.Status != "failed" || wallet.debits != 1 {
		t.Fatalf("response=%d result=%+v debits=%d", response.StatusCode, result, wallet.debits)
	}
	seated, _, err := service.Seated(ctx, roomID, "visitor")
	if err != nil || seated {
		t.Fatalf("failed debit occupied a seat: seated=%v err=%v", seated, err)
	}
	// A browser retry must see the released reservation, without attempting
	// another debit or trapping the player on the waiting screen.
	retry, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/rooms/"+roomID+"/reservations/"+reservation.ID, nil))
	if err != nil {
		t.Fatal(err)
	}
	defer retry.Body.Close()
	if err := json.NewDecoder(retry.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "expired" || wallet.debits != 1 {
		t.Fatalf("retry=%+v debits=%d", result, wallet.debits)
	}
}
