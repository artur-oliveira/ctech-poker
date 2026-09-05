package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v3"
	"gopkg.aoctech.app/poker/api/internal/config"
	"gopkg.aoctech.app/poker/api/internal/player"
	"gopkg.aoctech.app/poker/api/internal/reports"
)

type apiReportStore struct{ report reports.Report }
func (s *apiReportStore) Create(_ context.Context, report reports.Report, _ string) (*reports.Report, bool, error) { report.ReportID = "report-id"; s.report = report; return &report, true, nil }
func (s *apiReportStore) Get(context.Context, string, string) (*reports.Report, error) { return &s.report, nil }
func (s *apiReportStore) ListByStatus(context.Context, reports.Status, string, int) (reports.Page, error) { return reports.Page{}, nil }
func (s *apiReportStore) ListByReporter(context.Context, string, string, int) (reports.Page, error) { return reports.Page{}, nil }
func (s *apiReportStore) SetStatus(context.Context, string, string, reports.Status, reports.Resolution, string) error { return nil }

type apiReportPlayers struct{}
func (apiReportPlayers) Get(_ context.Context, id string) (*player.PlayerProfile, error) { return &player.PlayerProfile{UserID: id}, nil }

// filteringReportStore is a minimal in-memory reports.Store that actually
// honors reporter/target separation, so tests exercise the same IDOR
// boundary the real gsi_reporter query relies on rather than a stub that
// always returns everything.
type filteringReportStore struct{ reports []reports.Report }

func (s *filteringReportStore) Create(_ context.Context, report reports.Report, _ string) (*reports.Report, bool, error) {
	report.ReportID = "report-" + report.TargetPlayerID + "-" + report.ReporterPlayerID
	s.reports = append(s.reports, report)
	return &report, true, nil
}
func (s *filteringReportStore) Get(context.Context, string, string) (*reports.Report, error) { return nil, reports.ErrNotFound }
func (s *filteringReportStore) ListByStatus(context.Context, reports.Status, string, int) (reports.Page, error) {
	return reports.Page{}, nil
}
func (s *filteringReportStore) ListByReporter(_ context.Context, reporterID, _ string, _ int) (reports.Page, error) {
	page := reports.Page{}
	for _, r := range s.reports {
		if r.ReporterPlayerID == reporterID {
			page.Reports = append(page.Reports, r)
		}
	}
	return page, nil
}
func (s *filteringReportStore) SetStatus(context.Context, string, string, reports.Status, reports.Resolution, string) error {
	return nil
}

func TestMyReportsOnlyReturnsCallersOwnReportsNeverReportsAgainstThem(t *testing.T) {
	store := &filteringReportStore{}
	svc := reports.NewService(store, nil, apiReportPlayers{})
	// "victim" filed a report against "reporter" — must never show up in
	// reporter's own list even though reporter is the target.
	if _, err := svc.Create(context.Background(), "victim", "idem-1", reports.CreateInput{TargetPlayerID: "reporter", Category: reports.CategoryOther, Surface: reports.SurfaceProfile}); err != nil {
		t.Fatalf("seed victim report: %v", err)
	}
	if _, err := svc.Create(context.Background(), "reporter", "idem-2", reports.CreateInput{TargetPlayerID: "someone-else", Category: reports.CategoryHarassment, Surface: reports.SurfaceProfile}); err != nil {
		t.Fatalf("seed reporter's own report: %v", err)
	}
	h := &playerHandlers{reports: svc}
	app := fiber.New()
	app.Get("/players/me/reports", func(c fiber.Ctx) error {
		c.Locals(localsUserID, "reporter")
		return c.Next()
	}, h.myReports)
	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/players/me/reports", nil))
	if err != nil || resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status=%d err=%v", resp.StatusCode, err)
	}
	raw, _ := io.ReadAll(resp.Body)
	var out struct {
		Reports []reports.PlayerReportView `json:"reports"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode: %v body=%s", err, raw)
	}
	if len(out.Reports) != 1 || out.Reports[0].TableID != "" || strings.Contains(string(raw), "someone-else") == false {
		t.Fatalf("expected exactly the caller's own report, got %s", raw)
	}
	if strings.Contains(string(raw), "victim") {
		t.Fatalf("leaked a report filed against the caller: %s", raw)
	}
	if strings.Contains(string(raw), "reviewed_by") || strings.Contains(string(raw), "evidence_message") || strings.Contains(string(raw), "details") {
		t.Fatalf("leaked sensitive fields: %s", raw)
	}
}

func TestCreateReportReturnsAcceptedWithoutEchoingFreeText(t *testing.T) {
	store := &apiReportStore{}
	svc := reports.NewService(store, nil, apiReportPlayers{})
	app := fiber.New()
	auth := func(c fiber.Ctx) error { c.Locals(localsUserID, "reporter"); c.Locals(localsFirstParty, true); return c.Next() }
	RegisterReports(app.Group("/v1.0"), auth, svc, &config.Config{Env: "test"}, ReportLimiters{})
	body, _ := json.Marshal(reportRequest{TargetPlayerID: "target", Category: reports.CategoryOther, Surface: reports.SurfaceProfile, Details: "private details"})
	req := httptest.NewRequest(fiber.MethodPost, "/v1.0/social/reports", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(idempotencyKeyHeader, "idem")
	resp, err := app.Test(req)
	if err != nil || resp.StatusCode != fiber.StatusAccepted { t.Fatalf("status=%d err=%v", resp.StatusCode, err) }
	raw, _ := io.ReadAll(resp.Body)
	if strings.Contains(string(raw), "private") || !strings.Contains(string(raw), "report-id") { t.Fatalf("body=%s", raw) }
	if store.report.Details != "private details" { t.Fatalf("stored=%+v", store.report) }
}
