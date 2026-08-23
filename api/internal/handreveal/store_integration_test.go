//go:build integration

package handreveal

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

func handRevealTestClient(t *testing.T) *dynamodb.Client {
	t.Helper()
	cfg, err := config.LoadDefaultConfig(
		context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("dummy", "dummy", "")),
	)
	if err != nil {
		t.Fatal(err)
	}
	return dynamodb.NewFromConfig(cfg, func(o *dynamodb.Options) {
		o.BaseEndpoint = aws.String("http://localhost:8555")
	})
}

func createHandRevealsTable(t *testing.T, db *dynamodb.Client, env string) {
	t.Helper()
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(env + "_" + tableHandReveals), BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func createHandRevealPaymentsTable(t *testing.T, db *dynamodb.Client, env string) {
	t.Helper()
	_, err := db.CreateTable(context.Background(), &dynamodb.CreateTableInput{
		TableName: aws.String(env + "_" + tableHandRevealPayments), BillingMode: types.BillingModePayPerRequest,
		AttributeDefinitions: []types.AttributeDefinition{
			{AttributeName: aws.String("pk"), AttributeType: types.ScalarAttributeTypeS},
		},
		KeySchema: []types.KeySchemaElement{
			{AttributeName: aws.String("pk"), KeyType: types.KeyTypeHash},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStorePutThenGetRoundTrips(t *testing.T) {
	db := handRevealTestClient(t)
	env := fmt.Sprintf("handreveal_test_%d", time.Now().UnixNano())
	createHandRevealsTable(t, db, env)
	store := NewStore(db, env)

	record := HandRecord{
		HandID: "hand-1", TableID: "table-1", BigBlind: 200, WinnerID: "winner",
		PlayerHands: map[string]PlayerHandCode{
			"winner": {Cards: [2]string{"Ah", "Kd"}},
			"loser":  {Cards: [2]string{"2c", "7s"}},
		},
		EndedAt: 1000,
	}
	if err := store.Put(context.Background(), record); err != nil {
		t.Fatal(err)
	}

	got, err := store.Get(context.Background(), "hand-1")
	if err != nil || got == nil {
		t.Fatalf("get: %+v err=%v", got, err)
	}
	if got.WinnerID != "winner" || got.BigBlind != 200 || got.WinnerShown {
		t.Fatalf("unexpected record: %+v", got)
	}
	if got.PlayerHands["winner"].Cards != [2]string{"Ah", "Kd"} {
		t.Fatalf("winner cards not round-tripped: %+v", got.PlayerHands)
	}
}

func TestStoreGetMissingHandReturnsNilNil(t *testing.T) {
	db := handRevealTestClient(t)
	env := fmt.Sprintf("handreveal_test_%d", time.Now().UnixNano())
	createHandRevealsTable(t, db, env)
	store := NewStore(db, env)

	got, err := store.Get(context.Background(), "no-such-hand")
	if err != nil || got != nil {
		t.Fatalf("expected nil, nil for a missing hand, got %+v err=%v", got, err)
	}
}

func TestStorePutTwiceUpdatesWinnerShown(t *testing.T) {
	db := handRevealTestClient(t)
	env := fmt.Sprintf("handreveal_test_%d", time.Now().UnixNano())
	createHandRevealsTable(t, db, env)
	store := NewStore(db, env)

	base := HandRecord{
		HandID: "hand-2", TableID: "table-1", BigBlind: 200, WinnerID: "winner",
		PlayerHands: map[string]PlayerHandCode{"winner": {Cards: [2]string{"Ah", "Kd"}}},
	}
	if err := store.Put(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	base.WinnerShown = true
	if err := store.Put(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(context.Background(), "hand-2")
	if err != nil || got == nil || !got.WinnerShown {
		t.Fatalf("expected WinnerShown=true after the second Put, got %+v err=%v", got, err)
	}
}

func TestClaimPaymentIsIdempotent(t *testing.T) {
	db := handRevealTestClient(t)
	env := fmt.Sprintf("handreveal_test_%d", time.Now().UnixNano())
	createHandRevealPaymentsTable(t, db, env)
	store := NewPaymentStore(db, env)

	first, err := store.ClaimPayment(context.Background(), "hand-1", "buyer")
	if err != nil || first != StatusPending {
		t.Fatalf("first claim: %q err=%v", first, err)
	}
	second, err := store.ClaimPayment(context.Background(), "hand-1", "buyer")
	if err != nil || second != StatusPending {
		t.Fatalf("second claim should return the existing pending row: %q err=%v", second, err)
	}
}

func TestCompletePaymentThenHasPaid(t *testing.T) {
	db := handRevealTestClient(t)
	env := fmt.Sprintf("handreveal_test_%d", time.Now().UnixNano())
	createHandRevealPaymentsTable(t, db, env)
	store := NewPaymentStore(db, env)

	if _, err := store.ClaimPayment(context.Background(), "hand-1", "buyer"); err != nil {
		t.Fatal(err)
	}
	paid, err := store.HasPaid(context.Background(), "hand-1", "buyer")
	if err != nil || paid {
		t.Fatalf("expected not yet paid, got paid=%v err=%v", paid, err)
	}
	if err := store.CompletePayment(context.Background(), "hand-1", "buyer"); err != nil {
		t.Fatal(err)
	}
	paid, err = store.HasPaid(context.Background(), "hand-1", "buyer")
	if err != nil || !paid {
		t.Fatalf("expected paid after CompletePayment, got paid=%v err=%v", paid, err)
	}
}

func TestHasPaidFalseForUnknownBuyer(t *testing.T) {
	db := handRevealTestClient(t)
	env := fmt.Sprintf("handreveal_test_%d", time.Now().UnixNano())
	createHandRevealPaymentsTable(t, db, env)
	store := NewPaymentStore(db, env)

	paid, err := store.HasPaid(context.Background(), "hand-1", "nobody")
	if err != nil || paid {
		t.Fatalf("expected false for an unclaimed payment, got paid=%v err=%v", paid, err)
	}
}

type fakeWallet struct {
	debits, credits       []int64
	debitKeys, creditKeys []string
	failDebit, failCredit bool
}

func (f *fakeWallet) Debit(_ context.Context, _ string, amount int64, key, _ string) error {
	f.debits = append(f.debits, amount)
	f.debitKeys = append(f.debitKeys, key)
	if f.failDebit {
		return fmt.Errorf("wallet: insufficient funds")
	}
	return nil
}

func (f *fakeWallet) Credit(_ context.Context, _ string, amount int64, key, _ string) error {
	f.credits = append(f.credits, amount)
	f.creditKeys = append(f.creditKeys, key)
	if f.failCredit {
		return fmt.Errorf("wallet: unavailable")
	}
	return nil
}

func TestServicePayForRevealDebitsFullFeeCreditsHalfAndCompletes(t *testing.T) {
	db := handRevealTestClient(t)
	env := fmt.Sprintf("handreveal_test_%d", time.Now().UnixNano())
	createHandRevealPaymentsTable(t, db, env)
	payments := NewPaymentStore(db, env)
	wallet := &fakeWallet{}
	svc := NewService(wallet, payments)

	if err := svc.PayForReveal(context.Background(), "buyer", "winner", "hand-1", 200); err != nil {
		t.Fatal(err)
	}
	if len(wallet.debits) != 1 || wallet.debits[0] != 200 {
		t.Fatalf("expected one 200 debit, got %+v", wallet.debits)
	}
	if len(wallet.credits) != 1 || wallet.credits[0] != 100 {
		t.Fatalf("expected one 100 credit (fee/2), got %+v", wallet.credits)
	}
	paid, err := payments.HasPaid(context.Background(), "hand-1", "buyer")
	if err != nil || !paid {
		t.Fatalf("expected HasPaid=true after PayForReveal, got %v err=%v", paid, err)
	}

	// A second call must be a no-op: already completed, no new wallet calls.
	if err := svc.PayForReveal(context.Background(), "buyer", "winner", "hand-1", 200); err != nil {
		t.Fatal(err)
	}
	if len(wallet.debits) != 1 || len(wallet.credits) != 1 {
		t.Fatalf("expected no additional wallet calls on retry, got debits=%+v credits=%+v", wallet.debits, wallet.credits)
	}
}

func TestServicePayForRevealLeavesPendingOnDebitFailure(t *testing.T) {
	db := handRevealTestClient(t)
	env := fmt.Sprintf("handreveal_test_%d", time.Now().UnixNano())
	createHandRevealPaymentsTable(t, db, env)
	payments := NewPaymentStore(db, env)
	wallet := &fakeWallet{failDebit: true}
	svc := NewService(wallet, payments)

	if err := svc.PayForReveal(context.Background(), "buyer", "winner", "hand-1", 200); err == nil {
		t.Fatal("expected the debit failure to propagate")
	}
	paid, err := payments.HasPaid(context.Background(), "hand-1", "buyer")
	if err != nil || paid {
		t.Fatalf("a failed debit must leave the payment row pending, not paid: %v err=%v", paid, err)
	}
}
