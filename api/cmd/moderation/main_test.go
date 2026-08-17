package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"gopkg.aoctech.app/poker/api/internal/reports"
)

type fakeModerationStore struct {
	report     reports.Report
	status     reports.Status
	resolution reports.Resolution
}

func (s *fakeModerationStore) Get(context.Context, string, string) (*reports.Report, error) {
	return &s.report, nil
}
func (s *fakeModerationStore) ListByStatus(context.Context, reports.Status, string, int) (reports.Page, error) {
	return reports.Page{Reports: []reports.Report{s.report}}, nil
}
func (s *fakeModerationStore) SetStatus(_ context.Context, _, _ string, status reports.Status, resolution reports.Resolution, _ string) error {
	s.status, s.resolution = status, resolution
	return nil
}

func TestListHidesTextAndShowIsExplicitDisclosure(t *testing.T) {
	store := &fakeModerationStore{report: reports.Report{TargetPlayerID: "target", StorageKey: "key", ReportID: "id", Status: reports.StatusOpen, Details: "private details", EvidenceMessage: "private evidence"}}
	var list bytes.Buffer
	if err := run(context.Background(), []string{"list"}, store, &list); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(list.String(), "private") {
		t.Fatalf("list leaked text: %s", list.String())
	}
	var show bytes.Buffer
	if err := run(context.Background(), []string{"show", "--target", "target", "--key", "key"}, store, &show); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(show.String(), "private details") || !strings.Contains(show.String(), "private evidence") {
		t.Fatalf("show=%s", show.String())
	}
}

func TestResolveRequiresEnumeratedResolution(t *testing.T) {
	store := &fakeModerationStore{}
	err := run(context.Background(), []string{"resolve", "--target", "target", "--key", "key", "--moderator", "ops", "--resolution", "ban_now"}, store, &bytes.Buffer{})
	if err == nil {
		t.Fatal("expected invalid resolution")
	}
	if err := run(context.Background(), []string{"resolve", "--target", "target", "--key", "key", "--moderator", "ops", "--resolution", "no_action"}, store, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if store.status != reports.StatusResolved || store.resolution != reports.ResolutionNoAction {
		t.Fatalf("store=%+v", store)
	}
}
