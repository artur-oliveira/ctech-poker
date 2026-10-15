package responsible

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ExportedUser is one player's state as ctech-wallet exports it (field names are the
// wallet's wire contract: GET /v1.0/internal/wallet/responsible/export).
type ExportedUser struct {
	UserID                  string               `json:"user_id"`
	TermsAddendumVersion    string               `json:"terms_addendum_version,omitempty"`
	TermsAcceptedAt         string               `json:"terms_accepted_at,omitempty"`
	GamblingAddendumVersion string               `json:"gambling_addendum_version,omitempty"`
	GamblingActivatedAt     string               `json:"gambling_activated_at,omitempty"`
	SelfExclusion           *SelfExclusion       `json:"self_exclusion,omitempty"`
	GameLimits              *GameLimits          `json:"game_limits,omitempty"`
	GameDepositCounters     *GameDepositCounters `json:"game_deposit_counters,omitempty"`
}

// ExportPage is one page of the export. Carried exclusions come with every page that
// has any (the wallet keeps them in the same table as its users).
type ExportPage struct {
	Users             []ExportedUser     `json:"users"`
	CarriedExclusions []CarriedExclusion `json:"carried_exclusions"`
	NextCursor        string             `json:"next_cursor"`
}

// Exporter pages the wallet's export.
type Exporter interface {
	ResponsibleExport(ctx context.Context, cursor string) (*ExportPage, error)
}

// ImportCounts is what a run did.
type ImportCounts struct {
	Users, Imported, AlreadyThere, Carried int
}

// Import copies the wallet's responsible-gambling state into this service's store,
// verbatim: consent versions and times, an exclusion's end, a pending limit increase
// mid-cooling-off, the inflow counters. A player already present is left alone, so a
// second run changes nothing. dryRun counts without writing.
func Import(ctx context.Context, from Exporter, store Store, dryRun bool, now time.Time) (ImportCounts, error) {
	var counts ImportCounts
	cursor := ""
	for {
		page, err := from.ResponsibleExport(ctx, cursor)
		if err != nil {
			return counts, fmt.Errorf("responsible: export page: %w", err)
		}
		for _, u := range page.Users {
			counts.Users++
			if dryRun {
				continue
			}
			st := &State{
				UserID:                  u.UserID,
				GamblingAddendumVersion: u.GamblingAddendumVersion,
				GamblingAcceptedAt:      u.GamblingActivatedAt,
				SelfExclusion:           u.SelfExclusion,
				GameLimits:              u.GameLimits,
				GameDepositCounters:     u.GameDepositCounters,
				ImportedAt:              now.UTC().Format(time.RFC3339),
			}
			switch err := store.Put(ctx, st, 0); {
			case err == nil:
				counts.Imported++
				_ = store.Audit(ctx, u.UserID, "imported_from_wallet", u.GamblingAddendumVersion, now)
			case errors.Is(err, ErrConflict):
				counts.AlreadyThere++
			default:
				return counts, fmt.Errorf("responsible: import %s: %w", u.UserID, err)
			}
		}
		for _, c := range page.CarriedExclusions {
			counts.Carried++
			if dryRun {
				continue
			}
			if err := store.PutCarried(ctx, c.CPFHMAC, c.SelfExclusion, time.Unix(c.ExpiresAt, 0)); err != nil {
				return counts, fmt.Errorf("responsible: import carried exclusion: %w", err)
			}
		}
		if page.NextCursor == "" {
			return counts, nil
		}
		cursor = page.NextCursor
	}
}
