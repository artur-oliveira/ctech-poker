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
func (s *apiReportStore) SetStatus(context.Context, string, string, reports.Status, reports.Resolution, string) error { return nil }

type apiReportPlayers struct{}
func (apiReportPlayers) Get(_ context.Context, id string) (*player.PlayerProfile, error) { return &player.PlayerProfile{UserID: id}, nil }

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
