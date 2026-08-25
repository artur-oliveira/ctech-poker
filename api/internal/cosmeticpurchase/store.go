package cosmeticpurchase

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"gopkg.aoctech.app/api-commons/dynamo"
	"gopkg.aoctech.app/poker/api/internal/cosmetics"
)

const (
	tableEntitlements = "poker_cosmetic_entitlements"
	tablePurchases    = "poker_cosmetic_purchases"

	statusProcessing = "processing"
	statusPending    = "pending"
	statusActive     = "active"
	statusConfirmed  = "confirmed"
	statusRefunding  = "refunding"
	statusRefunded   = "refunded"
	statusFailed     = "failed"
	statusExpired    = "expired"

	methodPIX    = "pix"
	methodFichas = "fichas"
)

// entitlementSK builds the composite sort key for an entitlement row. Deck
// and felt ids don't collide today, but the key is namespaced by kind
// regardless so the table never has to assume that forever.
func entitlementSK(kind cosmetics.Kind, itemID string) string { return string(kind) + "#" + itemID }

// Entitlement is one row per premium cosmetic claim. Pending rows reserve the
// single purchase slot but do not grant ownership; active rows do. Free
// cosmetics never get a row because their ownership is universal.
type Entitlement struct {
	PlayerID       string `dynamodbav:"pk" json:"player_id"`
	Kind           string `dynamodbav:"kind" json:"kind"`
	ItemID         string `dynamodbav:"item_id" json:"item_id"`
	SK             string `dynamodbav:"sk" json:"-"`
	PurchaseMethod string `dynamodbav:"purchase_method" json:"purchase_method"` // "pix" | "fichas"
	PurchaseID     string `dynamodbav:"purchase_id" json:"purchase_id"`
	Status         string `dynamodbav:"status,omitempty" json:"-"` // pending | active; empty means active for legacy rows
	RequestKey     string `dynamodbav:"request_key,omitempty" json:"-"`
	IdemKey        string `dynamodbav:"idem_key,omitempty" json:"-"`
	UsedAt         string `dynamodbav:"used_at,omitempty" json:"used_at,omitempty"`
	CreatedAt      string `dynamodbav:"created_at" json:"created_at"`
}

// Record is purchase history — never TTL'd, mirrors poker_reaction_purchases's shape.
type Record struct {
	PlayerID      string `dynamodbav:"pk" json:"player_id"`
	PurchaseID    string `dynamodbav:"sk" json:"purchase_id"`
	Kind          string `dynamodbav:"kind" json:"kind"`
	ItemID        string `dynamodbav:"item_id" json:"item_id"`
	Method        string `dynamodbav:"method" json:"method"` // "pix" | "fichas"
	PriceCents    int64  `dynamodbav:"price_cents,omitempty" json:"price_cents,omitempty"`
	PriceFichas   int64  `dynamodbav:"price_fichas,omitempty" json:"price_fichas,omitempty"`
	Status        string `dynamodbav:"status" json:"status"` // processing | pending | confirmed | refunding | refunded | failed | expired
	IdemKey       string `dynamodbav:"idem_key,omitempty" json:"-"`
	PixCopiaECola string `dynamodbav:"pix_copia_e_cola,omitempty" json:"pix_copia_e_cola,omitempty"`
	QRCodeBase64  string `dynamodbav:"qr_code_base64,omitempty" json:"qr_code_base64,omitempty"`
	ExpiresAt     string `dynamodbav:"expires_at,omitempty" json:"expires_at,omitempty"`
	CreatedAt     string `dynamodbav:"created_at" json:"created_at"`
	UpdatedAt     string `dynamodbav:"updated_at" json:"updated_at"`
}

type EntitlementStore struct{ base dynamo.Base }

func NewEntitlementStore(db *dynamodb.Client, env string) *EntitlementStore {
	return &EntitlementStore{base: dynamo.NewBase(db, env, tableEntitlements)}
}

func (s *EntitlementStore) Put(ctx context.Context, e Entitlement) error {
	e.SK = entitlementSK(cosmetics.Kind(e.Kind), e.ItemID)
	encoded, err := dynamo.Encode(e)
	if err != nil {
		return fmt.Errorf("cosmeticpurchase: encode entitlement: %w", err)
	}
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{s.base.BuildPutTxItemIfAbsent(encoded)}); err != nil {
		return fmt.Errorf("cosmeticpurchase: put entitlement: %w", err)
	}
	return nil
}

// Reserve creates the single per-item ownership row before any external
// money operation. A different request can therefore never buy an already
// owned or in-flight item; a replay of the same request can safely resume.
func (s *EntitlementStore) Reserve(ctx context.Context, e Entitlement) (*Entitlement, bool, error) {
	e.SK = entitlementSK(cosmetics.Kind(e.Kind), e.ItemID)
	encoded, err := dynamo.Encode(e)
	if err != nil {
		return nil, false, fmt.Errorf("cosmeticpurchase: encode reservation: %w", err)
	}
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{s.base.BuildPutTxItemIfAbsent(encoded)}); err == nil {
		return &e, true, nil
	} else if !dynamo.IsConditionFailed(err) {
		return nil, false, fmt.Errorf("cosmeticpurchase: reserve entitlement: %w", err)
	}
	existing, err := s.Get(ctx, e.PlayerID, cosmetics.Kind(e.Kind), e.ItemID)
	if err != nil {
		return nil, false, err
	}
	return existing, false, nil
}

func (s *EntitlementStore) CancelReservation(ctx context.Context, playerID string, kind cosmetics.Kind, itemID, requestKey string) error {
	item := conditionalDelete(s.base.TableName, playerID, entitlementSK(kind, itemID),
		"#status = :pending AND request_key = :request_key", map[string]string{"#status": "status"},
		map[string]types.AttributeValue{
			":pending":     &types.AttributeValueMemberS{Value: statusPending},
			":request_key": &types.AttributeValueMemberS{Value: requestKey},
		})
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{item}); err != nil {
		return fmt.Errorf("cosmeticpurchase: cancel entitlement reservation: %w", err)
	}
	return nil
}

func (s *EntitlementStore) Get(ctx context.Context, playerID string, kind cosmetics.Kind, itemID string) (*Entitlement, error) {
	item, err := s.base.GetItem(ctx, playerID, entitlementSK(kind, itemID))
	if err != nil {
		return nil, fmt.Errorf("cosmeticpurchase: get entitlement: %w", err)
	}
	if item == nil {
		return nil, nil
	}
	return dynamo.Decode[Entitlement](item)
}

// OwnedIDs returns the ids of kind this player actively owns.
//
// Ownership lives here and nowhere else: a bought-then-refunded item leaves a
// history row behind (status "refunded") but no entitlement row, so anything
// answering "what does this player have" must read entitlements rather than
// replay purchase history. One query is enough — the entitlement table only
// ever holds premium claims, and cosmetics.All(kind) is a fixed handful.
func (s *EntitlementStore) OwnedIDs(ctx context.Context, playerID string, kind cosmetics.Kind) (map[string]bool, error) {
	result, err := s.base.Query(ctx, dynamo.QueryOpts{PK: playerID, SKPrefix: string(kind) + "#", Limit: 100})
	if err != nil {
		return nil, fmt.Errorf("cosmeticpurchase: list entitlements: %w", err)
	}
	owned := make(map[string]bool, len(result.Items))
	for _, item := range result.Items {
		e, err := dynamo.Decode[Entitlement](item)
		if err != nil {
			return nil, fmt.Errorf("cosmeticpurchase: decode entitlement: %w", err)
		}
		if e.active() {
			owned[e.ItemID] = true
		}
	}
	return owned, nil
}

func (s *EntitlementStore) Delete(ctx context.Context, playerID string, kind cosmetics.Kind, itemID string) error {
	if _, err := s.base.DeleteItem(ctx, playerID, entitlementSK(kind, itemID)); err != nil {
		return fmt.Errorf("cosmeticpurchase: delete entitlement: %w", err)
	}
	return nil
}

type Store struct{ base dynamo.Base }

func NewStore(db *dynamodb.Client, env string) *Store {
	return &Store{base: dynamo.NewBase(db, env, tablePurchases)}
}

func encodeRecord(rec Record) (map[string]types.AttributeValue, error) {
	encoded, err := dynamo.Encode(rec)
	if err != nil {
		return nil, fmt.Errorf("cosmeticpurchase: encode record: %w", err)
	}
	return encoded, nil
}

// CreateSandboxReservation atomically persists the recoverable purchase intent
// and the single-item reservation before the wallet debit is attempted.
func (s *Store) CreateSandboxReservation(ctx context.Context, entitlements *EntitlementStore, rec Record, entitlement Entitlement) error {
	recItem, err := encodeRecord(rec)
	if err != nil {
		return err
	}
	entitlement.SK = entitlementSK(cosmetics.Kind(entitlement.Kind), entitlement.ItemID)
	entItem, err := dynamo.Encode(entitlement)
	if err != nil {
		return fmt.Errorf("cosmeticpurchase: encode entitlement reservation: %w", err)
	}
	err = s.base.TransactWrite(ctx, []types.TransactWriteItem{
		s.base.BuildPutTxItemIfAbsent(recItem),
		entitlements.base.BuildPutTxItemIfAbsent(entItem),
	})
	if err != nil {
		return fmt.Errorf("cosmeticpurchase: reserve sandbox purchase: %w", err)
	}
	return nil
}

// ConfirmSandbox atomically makes the debited purchase visible in history and
// activates ownership. Replaying it after an ambiguous DynamoDB response is
// harmless because both conditions have already reached their terminal state.
func (s *Store) ConfirmSandbox(ctx context.Context, entitlements *EntitlementStore, rec Record, requestKey, updatedAt string) error {
	sk := rec.PurchaseID
	recordUpdate := s.base.BuildRawUpdateTxItem(rec.PlayerID, &sk,
		"SET #status = :confirmed, updated_at = :updated",
		"#status = :processing AND idem_key = :idem", map[string]string{"#status": "status"},
		map[string]types.AttributeValue{
			":confirmed":  &types.AttributeValueMemberS{Value: statusConfirmed},
			":processing": &types.AttributeValueMemberS{Value: statusProcessing},
			":updated":    &types.AttributeValueMemberS{Value: updatedAt},
			":idem":       &types.AttributeValueMemberS{Value: rec.IdemKey},
		})
	entSK := entitlementSK(cosmetics.Kind(rec.Kind), rec.ItemID)
	entitlementUpdate := entitlements.base.BuildRawUpdateTxItem(rec.PlayerID, &entSK,
		"SET #status = :active, purchase_id = :purchase_id",
		"#status = :pending AND request_key = :request_key", map[string]string{"#status": "status"},
		map[string]types.AttributeValue{
			":active":      &types.AttributeValueMemberS{Value: statusActive},
			":pending":     &types.AttributeValueMemberS{Value: statusPending},
			":purchase_id": &types.AttributeValueMemberS{Value: rec.PurchaseID},
			":request_key": &types.AttributeValueMemberS{Value: requestKey},
		})
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{recordUpdate, entitlementUpdate}); err != nil {
		if dynamo.IsConditionFailed(err) {
			current, getErr := s.Get(ctx, rec.PlayerID, rec.PurchaseID)
			ent, entErr := entitlements.Get(ctx, rec.PlayerID, cosmetics.Kind(rec.Kind), rec.ItemID)
			if getErr == nil && entErr == nil && current != nil && current.Status == statusConfirmed && ent != nil && ent.active() {
				return nil
			}
		}
		return fmt.Errorf("cosmeticpurchase: confirm sandbox purchase: %w", err)
	}
	return nil
}

// CancelSandboxReservation removes both pre-debit rows after a definitive
// wallet rejection. Transport/5xx errors deliberately do not call this: the
// persisted intent is what lets a retry safely recover an ambiguous debit.
func (s *Store) CancelSandboxReservation(ctx context.Context, entitlements *EntitlementStore, rec Record, requestKey string) error {
	recordDelete := conditionalDelete(s.base.TableName, rec.PlayerID, rec.PurchaseID,
		"#status = :processing AND idem_key = :idem", map[string]string{"#status": "status"},
		map[string]types.AttributeValue{
			":processing": &types.AttributeValueMemberS{Value: statusProcessing},
			":idem":       &types.AttributeValueMemberS{Value: rec.IdemKey},
		})
	entitlementDelete := conditionalDelete(entitlements.base.TableName, rec.PlayerID, entitlementSK(cosmetics.Kind(rec.Kind), rec.ItemID),
		"#status = :pending AND request_key = :request_key", map[string]string{"#status": "status"},
		map[string]types.AttributeValue{
			":pending":     &types.AttributeValueMemberS{Value: statusPending},
			":request_key": &types.AttributeValueMemberS{Value: requestKey},
		})
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{recordDelete, entitlementDelete}); err != nil {
		return fmt.Errorf("cosmeticpurchase: cancel sandbox reservation: %w", err)
	}
	return nil
}

// AttachRealPurchase atomically persists the wallet purchase and links the
// pre-charge reservation to its wallet-generated purchase ID.
func (s *Store) AttachRealPurchase(ctx context.Context, entitlements *EntitlementStore, rec Record, requestKey string) error {
	recItem, err := encodeRecord(rec)
	if err != nil {
		return err
	}
	entSK := entitlementSK(cosmetics.Kind(rec.Kind), rec.ItemID)
	entitlementUpdate := entitlements.base.BuildRawUpdateTxItem(rec.PlayerID, &entSK,
		"SET purchase_id = :purchase_id",
		"#status = :pending AND request_key = :request_key", map[string]string{"#status": "status"},
		map[string]types.AttributeValue{
			":pending":     &types.AttributeValueMemberS{Value: statusPending},
			":purchase_id": &types.AttributeValueMemberS{Value: rec.PurchaseID},
			":request_key": &types.AttributeValueMemberS{Value: requestKey},
		})
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{s.base.BuildPutTxItemIfAbsent(recItem), entitlementUpdate}); err != nil {
		if dynamo.IsConditionFailed(err) {
			existing, getErr := s.Get(ctx, rec.PlayerID, rec.PurchaseID)
			if getErr == nil && existing != nil && existing.Kind == rec.Kind && existing.ItemID == rec.ItemID && existing.IdemKey == rec.IdemKey {
				return nil
			}
			ent, entErr := entitlements.Get(ctx, rec.PlayerID, cosmetics.Kind(rec.Kind), rec.ItemID)
			if getErr == nil && entErr == nil && existing != nil && existing.Kind == rec.Kind && existing.ItemID == rec.ItemID &&
				existing.Status == statusConfirmed && ent.active() && ent.PurchaseID == rec.PurchaseID {
				return s.HydratePIXDetails(ctx, rec)
			}
		}
		return fmt.Errorf("cosmeticpurchase: attach real purchase: %w", err)
	}
	return nil
}

// HydratePIXDetails fills payment fields on a record reconstructed by an
// early webhook. It never changes financial status or ownership.
func (s *Store) HydratePIXDetails(ctx context.Context, rec Record) error {
	sk := rec.PurchaseID
	item := s.base.BuildRawUpdateTxItem(rec.PlayerID, &sk,
		"SET idem_key = :idem, pix_copia_e_cola = :pix, qr_code_base64 = :qr, expires_at = :expires, price_cents = :cents, price_fichas = :fichas",
		"kind = :kind AND item_id = :item AND #status = :confirmed", map[string]string{"#status": "status"},
		map[string]types.AttributeValue{
			":idem":      &types.AttributeValueMemberS{Value: rec.IdemKey},
			":pix":       &types.AttributeValueMemberS{Value: rec.PixCopiaECola},
			":qr":        &types.AttributeValueMemberS{Value: rec.QRCodeBase64},
			":expires":   &types.AttributeValueMemberS{Value: rec.ExpiresAt},
			":cents":     &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", rec.PriceCents)},
			":fichas":    &types.AttributeValueMemberN{Value: fmt.Sprintf("%d", rec.PriceFichas)},
			":kind":      &types.AttributeValueMemberS{Value: rec.Kind},
			":item":      &types.AttributeValueMemberS{Value: rec.ItemID},
			":confirmed": &types.AttributeValueMemberS{Value: statusConfirmed},
		})
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{item}); err != nil {
		return fmt.Errorf("cosmeticpurchase: hydrate PIX details: %w", err)
	}
	return nil
}

// RecoverPendingPIX reconstructs the durable local half of a wallet purchase
// after a crash or webhook/create race. The wallet is authoritative for the
// user and SKU; this method only claims a missing/pending entitlement.
func (s *Store) RecoverPendingPIX(ctx context.Context, entitlements *EntitlementStore, rec Record) error {
	recItem, err := encodeRecord(rec)
	if err != nil {
		return err
	}
	ent, err := entitlements.Get(ctx, rec.PlayerID, cosmetics.Kind(rec.Kind), rec.ItemID)
	if err != nil {
		return err
	}
	items := []types.TransactWriteItem{s.base.BuildPutTxItemIfAbsent(recItem)}
	if ent == nil {
		entItem, encodeErr := dynamo.Encode(Entitlement{
			PlayerID: rec.PlayerID, Kind: rec.Kind, ItemID: rec.ItemID, SK: entitlementSK(cosmetics.Kind(rec.Kind), rec.ItemID),
			PurchaseMethod: methodPIX, PurchaseID: rec.PurchaseID, Status: statusPending, IdemKey: rec.IdemKey, CreatedAt: rec.CreatedAt,
		})
		if encodeErr != nil {
			return fmt.Errorf("cosmeticpurchase: encode recovered entitlement: %w", encodeErr)
		}
		items = append(items, entitlements.base.BuildPutTxItemIfAbsent(entItem))
	} else {
		entSK := entitlementSK(cosmetics.Kind(rec.Kind), rec.ItemID)
		items = append(items, entitlements.base.BuildRawUpdateTxItem(rec.PlayerID, &entSK,
			"SET purchase_id = :purchase_id",
			"#status = :pending AND purchase_method = :method AND (attribute_not_exists(purchase_id) OR purchase_id = :empty OR purchase_id = :purchase_id)",
			map[string]string{"#status": "status"}, map[string]types.AttributeValue{
				":pending":     &types.AttributeValueMemberS{Value: statusPending},
				":method":      &types.AttributeValueMemberS{Value: methodPIX},
				":empty":       &types.AttributeValueMemberS{Value: ""},
				":purchase_id": &types.AttributeValueMemberS{Value: rec.PurchaseID},
			}))
	}
	if err := s.base.TransactWrite(ctx, items); err != nil {
		if dynamo.IsConditionFailed(err) {
			existing, getErr := s.Get(ctx, rec.PlayerID, rec.PurchaseID)
			if getErr == nil && existing != nil && existing.Kind == rec.Kind && existing.ItemID == rec.ItemID {
				return nil
			}
		}
		return fmt.Errorf("cosmeticpurchase: recover pending PIX purchase: %w", err)
	}
	return nil
}

func (e *Entitlement) active() bool { return e != nil && (e.Status == "" || e.Status == statusActive) }

func conditionalDelete(tableName, pk, sk, condition string, names map[string]string, values map[string]types.AttributeValue) types.TransactWriteItem {
	return types.TransactWriteItem{Delete: &types.Delete{
		TableName:                 aws.String(tableName),
		Key:                       map[string]types.AttributeValue{"pk": &types.AttributeValueMemberS{Value: pk}, "sk": &types.AttributeValueMemberS{Value: sk}},
		ConditionExpression:       aws.String(condition),
		ExpressionAttributeNames:  names,
		ExpressionAttributeValues: values,
	}}
}

// ClosePIXTerminal atomically records a wallet terminal state and releases a
// still-pending reservation, or revokes an active entitlement when wallet is
// already authoritative that a purchase was refunded.
func (s *Store) ClosePIXTerminal(ctx context.Context, entitlements *EntitlementStore, rec Record, status, updatedAt string) error {
	if status != statusFailed && status != statusExpired && status != statusRefunded {
		return fmt.Errorf("cosmeticpurchase: invalid terminal status %q", status)
	}
	sk := rec.PurchaseID
	recordUpdate := s.base.BuildRawUpdateTxItem(rec.PlayerID, &sk,
		"SET #status = :terminal, updated_at = :updated",
		"#status = :expected", map[string]string{"#status": "status"},
		map[string]types.AttributeValue{
			":terminal": &types.AttributeValueMemberS{Value: status},
			":expected": &types.AttributeValueMemberS{Value: rec.Status},
			":updated":  &types.AttributeValueMemberS{Value: updatedAt},
		})
	entitlementCondition := "#status = :pending AND purchase_id = :purchase_id"
	entitlementValues := map[string]types.AttributeValue{
		":pending":     &types.AttributeValueMemberS{Value: statusPending},
		":purchase_id": &types.AttributeValueMemberS{Value: rec.PurchaseID},
	}
	if rec.Status == statusConfirmed {
		entitlementCondition = "(attribute_not_exists(#status) OR #status = :active) AND purchase_id = :purchase_id"
		entitlementValues = map[string]types.AttributeValue{
			":active":      &types.AttributeValueMemberS{Value: statusActive},
			":purchase_id": &types.AttributeValueMemberS{Value: rec.PurchaseID},
		}
	}
	entitlementDelete := conditionalDelete(entitlements.base.TableName, rec.PlayerID, entitlementSK(cosmetics.Kind(rec.Kind), rec.ItemID),
		entitlementCondition, map[string]string{"#status": "status"}, entitlementValues)
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{recordUpdate, entitlementDelete}); err != nil {
		if dynamo.IsConditionFailed(err) {
			latest, getErr := s.Get(ctx, rec.PlayerID, rec.PurchaseID)
			if getErr == nil && latest != nil && latest.Status == status {
				return nil
			}
		}
		return fmt.Errorf("cosmeticpurchase: close terminal PIX purchase: %w", err)
	}
	return nil
}

// GrantConfirmed converges a wallet-confirmed PIX purchase into a confirmed
// history row plus active entitlement in one DynamoDB transaction. It can also
// reconstruct a record when the webhook beat CreateReal's local write.
func (s *Store) GrantConfirmed(ctx context.Context, entitlements *EntitlementStore, rec Record) (Record, bool, error) {
	current, err := s.Get(ctx, rec.PlayerID, rec.PurchaseID)
	if err != nil {
		return Record{}, false, err
	}
	ent, err := entitlements.Get(ctx, rec.PlayerID, cosmetics.Kind(rec.Kind), rec.ItemID)
	if err != nil {
		return Record{}, false, err
	}
	if current != nil && current.Status == statusConfirmed && ent.active() && ent.PurchaseID == rec.PurchaseID {
		return *current, false, nil
	}

	items := make([]types.TransactWriteItem, 0, 2)
	if current == nil {
		item, err := encodeRecord(rec)
		if err != nil {
			return Record{}, false, err
		}
		items = append(items, s.base.BuildPutTxItemIfAbsent(item))
	} else if current.Status != statusConfirmed {
		sk := rec.PurchaseID
		items = append(items, s.base.BuildRawUpdateTxItem(rec.PlayerID, &sk,
			"SET #status = :confirmed, updated_at = :updated",
			"#status = :expected", map[string]string{"#status": "status"},
			map[string]types.AttributeValue{
				":confirmed": &types.AttributeValueMemberS{Value: statusConfirmed},
				":expected":  &types.AttributeValueMemberS{Value: current.Status},
				":updated":   &types.AttributeValueMemberS{Value: rec.UpdatedAt},
			}))
		rec.CreatedAt = current.CreatedAt
	}

	if ent == nil {
		ent = &Entitlement{
			PlayerID: rec.PlayerID, Kind: rec.Kind, ItemID: rec.ItemID, SK: entitlementSK(cosmetics.Kind(rec.Kind), rec.ItemID),
			PurchaseMethod: methodPIX, PurchaseID: rec.PurchaseID, Status: statusActive, CreatedAt: rec.UpdatedAt,
		}
		item, err := dynamo.Encode(*ent)
		if err != nil {
			return Record{}, false, fmt.Errorf("cosmeticpurchase: encode confirmed entitlement: %w", err)
		}
		items = append(items, entitlements.base.BuildPutTxItemIfAbsent(item))
	} else if !ent.active() || ent.PurchaseID != rec.PurchaseID {
		entSK := entitlementSK(cosmetics.Kind(rec.Kind), rec.ItemID)
		items = append(items, entitlements.base.BuildRawUpdateTxItem(rec.PlayerID, &entSK,
			"SET #status = :active, purchase_method = :method, purchase_id = :purchase_id",
			"attribute_exists(pk) AND (attribute_not_exists(#status) OR #status = :pending)", map[string]string{"#status": "status"},
			map[string]types.AttributeValue{
				":active":      &types.AttributeValueMemberS{Value: statusActive},
				":pending":     &types.AttributeValueMemberS{Value: statusPending},
				":method":      &types.AttributeValueMemberS{Value: methodPIX},
				":purchase_id": &types.AttributeValueMemberS{Value: rec.PurchaseID},
			}))
	}

	if len(items) == 0 {
		return rec, false, nil
	}
	if err := s.base.TransactWrite(ctx, items); err != nil {
		if dynamo.IsConditionFailed(err) {
			latest, getErr := s.Get(ctx, rec.PlayerID, rec.PurchaseID)
			latestEnt, entErr := entitlements.Get(ctx, rec.PlayerID, cosmetics.Kind(rec.Kind), rec.ItemID)
			if getErr == nil && entErr == nil && latest != nil && latest.Status == statusConfirmed && latestEnt.active() && latestEnt.PurchaseID == rec.PurchaseID {
				return *latest, false, nil
			}
		}
		return Record{}, false, fmt.Errorf("cosmeticpurchase: grant confirmed purchase: %w", err)
	}
	return rec, true, nil
}

// BeginRefund atomically closes ownership and moves the record to refunding,
// conditioned on the item never having been used (i.e. never selected as the
// active deck/felt — see cosmeticpurchase.Service.Refund).
func (s *Store) BeginRefund(ctx context.Context, entitlements *EntitlementStore, rec Record, updatedAt string) error {
	sk := rec.PurchaseID
	recordUpdate := s.base.BuildRawUpdateTxItem(rec.PlayerID, &sk,
		"SET #status = :refunding, updated_at = :updated",
		"#status = :confirmed", map[string]string{"#status": "status"},
		map[string]types.AttributeValue{
			":refunding": &types.AttributeValueMemberS{Value: statusRefunding},
			":confirmed": &types.AttributeValueMemberS{Value: statusConfirmed},
			":updated":   &types.AttributeValueMemberS{Value: updatedAt},
		})
	deleteEntitlement := types.TransactWriteItem{Delete: &types.Delete{
		TableName: aws.String(entitlements.base.TableName),
		Key: map[string]types.AttributeValue{
			"pk": &types.AttributeValueMemberS{Value: rec.PlayerID},
			"sk": &types.AttributeValueMemberS{Value: entitlementSK(cosmetics.Kind(rec.Kind), rec.ItemID)},
		},
		ConditionExpression: aws.String("purchase_id = :purchase_id"),
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":purchase_id": &types.AttributeValueMemberS{Value: rec.PurchaseID},
		},
	}}
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{recordUpdate, deleteEntitlement}); err != nil {
		return fmt.Errorf("cosmeticpurchase: begin refund: %w", err)
	}
	return nil
}

func (s *Store) CompleteRefund(ctx context.Context, playerID, purchaseID, updatedAt string) error {
	sk := purchaseID
	item := s.base.BuildRawUpdateTxItem(playerID, &sk,
		"SET #status = :refunded, updated_at = :updated",
		"#status = :refunding", map[string]string{"#status": "status"},
		map[string]types.AttributeValue{
			":refunded":  &types.AttributeValueMemberS{Value: statusRefunded},
			":refunding": &types.AttributeValueMemberS{Value: statusRefunding},
			":updated":   &types.AttributeValueMemberS{Value: updatedAt},
		})
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{item}); err != nil {
		if dynamo.IsConditionFailed(err) {
			current, getErr := s.Get(ctx, playerID, purchaseID)
			if getErr == nil && current != nil && current.Status == statusRefunded {
				return nil
			}
		}
		return fmt.Errorf("cosmeticpurchase: complete refund: %w", err)
	}
	return nil
}

// Create persists rec, or returns the existing row unchanged on a retried
// request — mirrors reactionpurchase.Store.Create's conditional-put-then-reget idiom.
func (s *Store) Create(ctx context.Context, rec Record) (Record, error) {
	encoded, err := dynamo.Encode(rec)
	if err != nil {
		return Record{}, fmt.Errorf("cosmeticpurchase: encode record: %w", err)
	}
	if err := s.base.TransactWrite(ctx, []types.TransactWriteItem{s.base.BuildPutTxItemIfAbsent(encoded)}); err == nil {
		return rec, nil
	} else if !dynamo.IsConditionFailed(err) {
		return Record{}, fmt.Errorf("cosmeticpurchase: persist record: %w", err)
	}
	existing, err := s.base.GetItem(ctx, rec.PlayerID, rec.PurchaseID)
	if err != nil {
		return Record{}, fmt.Errorf("cosmeticpurchase: load existing record: %w", err)
	}
	if existing == nil {
		return Record{}, fmt.Errorf("cosmeticpurchase: record disappeared")
	}
	decoded, err := dynamo.Decode[Record](existing)
	if err != nil {
		return Record{}, fmt.Errorf("cosmeticpurchase: decode existing record: %w", err)
	}
	return *decoded, nil
}

func (s *Store) Get(ctx context.Context, playerID, purchaseID string) (*Record, error) {
	item, err := s.base.GetItem(ctx, playerID, purchaseID)
	if err != nil {
		return nil, fmt.Errorf("cosmeticpurchase: get record: %w", err)
	}
	if item == nil {
		return nil, nil
	}
	return dynamo.Decode[Record](item)
}

func (s *Store) UpdateStatus(ctx context.Context, playerID, purchaseID, status, updatedAt string) (bool, error) {
	sk := purchaseID
	return s.base.UpdateItem(ctx, playerID, &sk, map[string]any{"status": status, "updated_at": updatedAt})
}

// List returns one page of this player's purchase history for kind, newest
// page first, plus the DynamoDB key to resume from.
//
// The kind filter is not optional: deck and felt purchases share one
// (pk=player, sk=purchase) table, so an unfiltered query hands the deck
// endpoint every felt purchase too — which is how "8 de 6 liberados" showed
// up in the store. It is a FilterExpression rather than a key condition
// because the sort key is the purchase id, so a page can come back short
// while LastEvaluatedKey is still set; that is ordinary DynamoDB filtering
// and the has_next/next_cursor envelope already expresses it.
func (s *Store) List(ctx context.Context, playerID string, kind cosmetics.Kind, limit int, startKey map[string]types.AttributeValue) ([]Record, map[string]types.AttributeValue, error) {
	result, err := s.base.Query(ctx, dynamo.QueryOpts{
		PK: playerID, Limit: limit, ExclusiveStartKey: startKey,
		FilterField: "kind", FilterValue: string(kind),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("cosmeticpurchase: list records: %w", err)
	}
	out := make([]Record, 0, len(result.Items))
	for _, item := range result.Items {
		rec, err := dynamo.Decode[Record](item)
		if err != nil {
			return nil, nil, fmt.Errorf("cosmeticpurchase: decode record: %w", err)
		}
		out = append(out, *rec)
	}
	return out, result.LastEvaluatedKey, nil
}
