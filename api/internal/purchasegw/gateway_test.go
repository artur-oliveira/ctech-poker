package purchasegw

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"gopkg.aoctech.app/poker/api/internal/accountclient"
	"gopkg.aoctech.app/poker/api/internal/billingclient"
	"gopkg.aoctech.app/poker/api/internal/purchaselifecycle"
	"gopkg.aoctech.app/poker/api/internal/walletclient"
)

var ctx = context.Background()

type fakeBilling struct {
	invoices      map[string]*billingclient.Invoice // by reference
	created       []string
	refunds       []string // "invoice|amount|key"
	customers     []string
	createErr     error
	pixOpened     int
	nextInvoice   int
	payerMessages []string
	lifetimes     []time.Duration
}

func newFakeBilling() *fakeBilling {
	return &fakeBilling{invoices: map[string]*billingclient.Invoice{}}
}

func (f *fakeBilling) EnsureCustomer(_ context.Context, userID, taxID, name, email string) (string, error) {
	f.customers = append(f.customers, userID+"|"+taxID+"|"+name)
	return "cus_" + userID, nil
}

func (f *fakeBilling) CreateInvoice(_ context.Context, customerID, reference, _, payerMessage string, amount int64, lifetime time.Duration, md map[string]string) (*billingclient.Invoice, error) {
	f.payerMessages = append(f.payerMessages, payerMessage)
	f.lifetimes = append(f.lifetimes, lifetime)
	if f.createErr != nil {
		return nil, f.createErr
	}
	if _, ok := f.invoices[reference]; ok {
		return nil, billingclient.ErrConflict
	}
	f.nextInvoice++
	inv := &billingclient.Invoice{ID: "in_" + reference, Reference: reference, Status: billingclient.StatusOpen, AmountCents: amount,
		Metadata: md, CheckoutURL: "https://billing/checkout?token=T"}
	f.invoices[reference] = inv
	f.created = append(f.created, reference)
	return inv, nil
}

func (f *fakeBilling) InvoiceByReference(_ context.Context, reference string) (*billingclient.Invoice, error) {
	inv, ok := f.invoices[reference]
	if !ok {
		return nil, billingclient.ErrNotFound
	}
	cp := *inv
	return &cp, nil
}

func (f *fakeBilling) OpenPix(_ context.Context, inv *billingclient.Invoice) error {
	f.pixOpened++
	inv.PixCopiaECola, inv.ExpiresAt = "00020126pix", time.Date(2026, 10, 15, 20, 0, 0, 0, time.UTC)
	return nil
}

func (f *fakeBilling) RefundInvoice(_ context.Context, invoiceID string, amount int64, _, key string) error {
	f.refunds = append(f.refunds, invoiceID+"|"+itoa(amount)+"|"+key)
	return nil
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for ; n > 0; n /= 10 {
		b = append([]byte{byte('0' + n%10)}, b...)
	}
	return string(b)
}

func (f *fakeBilling) pay(reference string, paid int64) {
	f.invoices[reference].Status = billingclient.StatusPaid
	f.invoices[reference].PaidCents = paid
}

type fakeKYC struct {
	k   *accountclient.KYC
	err error
}

func (f fakeKYC) Get(context.Context, string) (*accountclient.KYC, error) { return f.k, f.err }

// fakeMoves is the wallet's Sandbox ledger with its idempotency: the same key credits once.
type fakeMoves struct {
	credits  map[string]int64 // key -> amount
	debits   []string
	debitErr error
	calls    int
}

func newFakeMoves() *fakeMoves { return &fakeMoves{credits: map[string]int64{}} }

func (m *fakeMoves) Credit(_ context.Context, _ string, amount int64, key, _, _ string) error {
	m.calls++
	m.credits[key] = amount
	return nil
}

func (m *fakeMoves) Debit(_ context.Context, userID string, amount int64, key, _, _ string) error {
	if m.debitErr != nil {
		return m.debitErr
	}
	m.debits = append(m.debits, userID+"|"+itoa(amount)+"|"+key)
	return nil
}

func newGateway() (*Gateway, *fakeBilling, *fakeMoves) {
	b, m := newFakeBilling(), newFakeMoves()
	k := fakeKYC{k: &accountclient.KYC{CPF: "12345678909", LegalName: "Ana Souza", Email: "ana@x.com"}}
	return New(m, b, k), b, m
}

func TestAChipPackBecomesAnInvoiceAtTheCataloguePriceWithAPix(t *testing.T) {
	g, b, _ := newGateway()
	p, err := g.PurchaseSandbox(ctx, "u1", "pack_500", "key-1", "Compra de fichas (pack_500)")
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != purchaselifecycle.StatusPending || p.PixCopiaECola != "00020126pix" || p.QRCodeBase64 == "" || p.ExpiresAt == "" {
		t.Fatalf("purchase = %+v", p)
	}
	if !strings.HasPrefix(p.PurchaseID, SandboxPrefix) || p.CreditsGranted != 55000 || p.Amount != 500 {
		t.Fatalf("purchase = %+v", p)
	}
	inv := b.invoices[p.PurchaseID]
	if inv.AmountCents != 500 || inv.Metadata[billingclient.MetadataSKU] != "pack_500" || inv.Metadata[billingclient.MetadataUserID] != "u1" {
		t.Fatalf("invoice = %+v", inv)
	}
	if len(b.customers) != 1 || b.customers[0] != "u1|12345678909|Ana Souza" {
		t.Fatalf("customers = %v", b.customers)
	}

	// The same key is the same sale; another pack under that key is a conflict.
	again, err := g.PurchaseSandbox(ctx, "u1", "pack_500", "key-1", "x")
	if err != nil || again.PurchaseID != p.PurchaseID || len(b.created) != 1 {
		t.Fatalf("replay = %+v err=%v created=%v", again, err, b.created)
	}
	_, err = g.PurchaseSandbox(ctx, "u1", "pack_1000", "key-1", "x")
	var werr *walletclient.Error
	if !errors.As(err, &werr) || werr.Status != http.StatusConflict || !purchaselifecycle.DefinitiveWalletRejection(err) {
		t.Fatalf("another pack under the same key: %v", err)
	}
}

func TestAPaidPackCreditsTheSandboxOnceUnderTheInvoiceId(t *testing.T) {
	g, b, m := newGateway()
	p, _ := g.PurchaseSandbox(ctx, "u1", "pack_100", "key-1", "x")
	if got, _ := g.GetSandboxPurchase(ctx, p.PurchaseID); got.Status != purchaselifecycle.StatusPending || m.calls != 0 {
		t.Fatalf("an unpaid sale credited or moved: %+v calls=%d", got, m.calls)
	}
	b.pay(p.PurchaseID, 100)
	for range 3 { // the webhook, the poll and the sweep all read it
		got, err := g.GetSandboxPurchase(ctx, p.PurchaseID)
		if err != nil || got.Status != purchaselifecycle.StatusConfirmed {
			t.Fatalf("paid read = %+v err=%v", got, err)
		}
	}
	if len(m.credits) != 1 || m.credits["in_"+p.PurchaseID] != 10000 {
		t.Fatalf("credits = %v (one grant, keyed by the invoice id, 10000 credits)", m.credits)
	}
}

func TestAPaidInvoiceThatIsNotTheCataloguePriceIsNeverCredited(t *testing.T) {
	g, b, m := newGateway()
	p, _ := g.PurchaseSandbox(ctx, "u1", "pack_500", "key-1", "x")
	b.pay(p.PurchaseID, 500)
	b.invoices[p.PurchaseID].AmountCents = 100 // an invoice that does not say what was sold
	_, err := g.GetSandboxPurchase(ctx, p.PurchaseID)
	var werr *walletclient.Error
	if !errors.As(err, &werr) || werr.Type != "/problems/amount-mismatch" || !purchaselifecycle.DefinitiveWalletRejection(err) {
		t.Fatalf("err = %v", err)
	}
	if m.calls != 0 {
		t.Fatal("a mismatching invoice was credited")
	}
	// Paid short is the same.
	b.invoices[p.PurchaseID].AmountCents, b.invoices[p.PurchaseID].PaidCents = 500, 300
	if _, err := g.GetSandboxPurchase(ctx, p.PurchaseID); err == nil || m.calls != 0 {
		t.Fatalf("a short payment was credited: %v calls=%d", err, m.calls)
	}
}

func TestBillingDownFailsClosedAndLeavesNothingThatBlocksARetry(t *testing.T) {
	g, b, _ := newGateway()
	b.createErr = errors.New("billing 503")
	_, err := g.PurchaseSandbox(ctx, "u1", "pack_100", "key-1", "x")
	if err == nil || purchaselifecycle.DefinitiveWalletRejection(err) {
		t.Fatalf("a Billing outage must be a retryable error, got %v", err)
	}
	if len(b.invoices) != 0 {
		t.Fatal("a failed sale left an invoice behind")
	}
	b.createErr = nil
	if _, err := g.PurchaseSandbox(ctx, "u1", "pack_100", "key-1", "x"); err != nil {
		t.Fatalf("the retry: %v", err)
	}
}

func TestAPlayerWithoutAVerifiedCPFCannotBuy(t *testing.T) {
	g, b, _ := newGateway()
	for _, k := range []fakeKYC{{err: accountclient.ErrNoKYC}, {k: &accountclient.KYC{}}} {
		g.kyc = k
		_, err := g.PurchaseSandbox(ctx, "u1", "pack_100", "key-1", "x")
		var werr *walletclient.Error
		if !errors.As(err, &werr) || werr.Type != "/problems/kyc-required" {
			t.Fatalf("err = %v", err)
		}
	}
	if len(b.invoices) != 0 {
		t.Fatal("an invoice was issued without a CPF")
	}
}

func TestUnknownPackAndMissingKeyAreRefusedBeforeAnyBillingCall(t *testing.T) {
	g, b, _ := newGateway()
	if _, err := g.PurchaseSandbox(ctx, "u1", "pack_free", "k", "x"); !purchaselifecycle.DefinitiveWalletRejection(err) {
		t.Fatalf("unknown pack: %v", err)
	}
	if _, err := g.PurchaseSandbox(ctx, "u1", "pack_100", "", "x"); !purchaselifecycle.DefinitiveWalletRejection(err) {
		t.Fatalf("no key: %v", err)
	}
	if len(b.customers) != 0 || len(b.created) != 0 {
		t.Fatal("Billing was called for a refused sale")
	}
}

func TestRefundTakesTheCreditsBackThenTheMoneyAndNeverTheOtherWayRound(t *testing.T) {
	g, b, m := newGateway()
	p, _ := g.PurchaseSandbox(ctx, "u1", "pack_500", "key-1", "x")
	b.pay(p.PurchaseID, 500)
	if _, err := g.GetSandboxPurchase(ctx, p.PurchaseID); err != nil {
		t.Fatal(err)
	}

	// The player has spent the credits: the wallet refuses the debit, no money moves.
	m.debitErr = &walletclient.Error{Status: 422, Type: "/problems/insufficient-balance", Title: "Insufficient Balance"}
	if _, err := g.RefundSandboxPurchase(ctx, "u1", p.PurchaseID, "r-1"); err == nil || len(b.refunds) != 0 {
		t.Fatalf("refund with spent credits: err=%v refunds=%v", err, b.refunds)
	}

	m.debitErr = nil
	got, err := g.RefundSandboxPurchase(ctx, "u1", p.PurchaseID, "r-1")
	if err != nil || got.Status != purchaselifecycle.StatusRefunded {
		t.Fatalf("refund = %+v err=%v", got, err)
	}
	if len(m.debits) != 1 || m.debits[0] != "u1|55000|refund-credits-in_"+p.PurchaseID {
		t.Fatalf("debits = %v", m.debits)
	}
	if len(b.refunds) != 1 || b.refunds[0] != "in_"+p.PurchaseID+"|500|refund-in_"+p.PurchaseID {
		t.Fatalf("refunds = %v", b.refunds)
	}
	// Another player cannot refund it.
	if _, err := g.RefundSandboxPurchase(ctx, "u2", p.PurchaseID, "r-2"); err == nil {
		t.Fatal("refunded someone else's purchase")
	}
}

func TestAGoodIsConfirmedWhenPaidAndMovesNoLedger(t *testing.T) {
	g, b, m := newGateway()
	p, err := g.PurchaseProduct(ctx, "u1", "poker_deck_golden", "key-1", "Baralho")
	if err != nil || !strings.HasPrefix(p.PurchaseID, ProductPrefix) || p.Amount != 500 || p.Status != purchaselifecycle.StatusPending {
		t.Fatalf("purchase = %+v err=%v", p, err)
	}
	b.pay(p.PurchaseID, 500)
	got, err := g.GetProductPurchase(ctx, p.PurchaseID)
	if err != nil || got.Status != purchaselifecycle.StatusConfirmed || got.SKU != "poker_deck_golden" {
		t.Fatalf("paid good = %+v err=%v", got, err)
	}
	if m.calls != 0 || len(m.debits) != 0 {
		t.Fatal("selling a good touched the ledger")
	}
	if r, err := g.RefundProductPurchase(ctx, "u1", p.PurchaseID, "r"); err != nil || r.Status != purchaselifecycle.StatusRefunded || len(b.refunds) != 1 {
		t.Fatalf("refund = %+v err=%v refunds=%v", r, err, b.refunds)
	}
}

func TestEachKindServesOnlyItsOwnIds(t *testing.T) {
	g, _, _ := newGateway()
	p, _ := g.PurchaseSandbox(ctx, "u1", "pack_100", "key-1", "x")
	if _, err := g.GetProductPurchase(ctx, p.PurchaseID); err == nil {
		t.Fatal("a chip-pack id was served as a good")
	}
	if _, err := g.GetSandboxPurchase(ctx, "snd_doesnotexist"); err == nil {
		t.Fatal("an unknown id was served")
	}
}

// The gateway has no door to the wallet's real money: Moves is the Sandbox ledger only (spec
// §7, no Real → Sandbox purchase path), so nothing here can reach DebitReal or the holds.
func TestTheGatewayHasNoRealMoneyDoor(t *testing.T) {
	typ := reflect.TypeOf(New(newFakeMoves(), newFakeBilling(), fakeKYC{}))
	for _, name := range []string{"DebitReal", "HoldGame", "ReleaseHold", "CashoutGame", "Balances"} {
		if _, ok := typ.MethodByName(name); ok {
			t.Errorf("Gateway exposes %s", name)
		}
	}
	if _, ok := typ.MethodByName("Credit"); !ok {
		t.Error("the Sandbox credit the services spend fichas through is missing")
	}
}

// The payer reads what was bought on the Pix, and the Pix lives an hour, not a month.
func TestThePixNamesWhatWasBoughtAndLivesAnHour(t *testing.T) {
	g, b, _ := newGateway()
	if _, err := g.PurchaseSandbox(ctx, "u1", "pack_500", "k1", "x"); err != nil {
		t.Fatal(err)
	}
	if _, err := g.PurchaseProduct(ctx, "u1", "poker_deck_golden", "k2", "x"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(b.payerMessages, " | "); got != "Pacote de 55.000 fichas | Poker: baralho golden" {
		t.Fatalf("messages = %q", got)
	}
	for _, l := range b.lifetimes {
		if l != time.Hour {
			t.Fatalf("lifetime = %v", l)
		}
	}
}

func TestGroupThousandsWritesChipsTheWayPlayersRead(t *testing.T) {
	for in, want := range map[int64]string{0: "0", 999: "999", 1000: "1.000", 55000: "55.000", 2000000: "2.000.000"} {
		if got := groupThousands(in); got != want {
			t.Errorf("%d = %q, want %q", in, got, want)
		}
	}
}
