package v1

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/gofiber/fiber/v3"

	"gopkg.aoctech.app/api-commons/pushsvc"
	"gopkg.aoctech.app/poker/api/internal/pokerpush"
)

type memReminders struct {
	mu   sync.Mutex
	rows map[string]pushsvc.Reminder
}

func (m *memReminders) Put(_ context.Context, r pushsvc.Reminder) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rows[r.Sub+"/"+r.Kind] = r
	return nil
}

func (m *memReminders) Get(_ context.Context, sub, kind string) (pushsvc.Reminder, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.rows[sub+"/"+kind]
	return r, ok, nil
}

type recordingInvoker struct{ jobs []pushsvc.Job }

func (r *recordingInvoker) Send(_ context.Context, j pushsvc.Job) error {
	r.jobs = append(r.jobs, j)
	return nil
}

func pushApp(t *testing.T, enabled bool, inv *recordingInvoker) *fiber.App {
	t.Helper()
	app := fiber.New()
	auth := func(c fiber.Ctx) error {
		if c.Get("Authorization") == "" {
			return unauthorizedProblem("missing bearer token").Send(c)
		}
		c.Locals(localsUserID, "player-1")
		return c.Next()
	}
	prefs := pokerpush.NewService(&memReminders{rows: map[string]pushsvc.Reminder{}})
	svc := pushsvc.NewService(nil, "")
	if enabled {
		svc = pushsvc.NewService(nil, testVAPIDPublicKey(t))
	}
	RegisterPush(app.Group("/v1.0"), auth, svc, prefs, inv, nil)
	return app
}

func pushDo(t *testing.T, app *fiber.App, method, path, body string, authed bool) *http.Response {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if authed {
		req.Header.Set("Authorization", "Bearer x")
	}
	res, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestPreferencesRequireAuth(t *testing.T) {
	app := pushApp(t, true, &recordingInvoker{})
	if res := pushDo(t, app, "GET", "/v1.0/push/preferences", "", false); res.StatusCode != 401 {
		t.Errorf("status = %d", res.StatusCode)
	}
}

func TestPutPreferencesRoundTrip(t *testing.T) {
	app := pushApp(t, true, &recordingInvoker{})
	body := `{"streak_reminder":{"enabled":true,"time":"20:30"},"come_play":{"enabled":true}}`
	res := pushDo(t, app, "PUT", "/v1.0/push/preferences", body, true)
	if res.StatusCode != 200 {
		t.Fatalf("put = %d", res.StatusCode)
	}
	var got pokerpush.Preferences
	_ = json.NewDecoder(pushDo(t, app, "GET", "/v1.0/push/preferences", "", true).Body).Decode(&got)
	if !got.StreakReminder.Enabled || got.StreakReminder.Time != "20:30" || !got.ComePlay.Enabled {
		t.Errorf("get = %+v", got)
	}
}

func TestPutPreferencesRejectsBadTimeAndUnknownFields(t *testing.T) {
	app := pushApp(t, true, &recordingInvoker{})
	if res := pushDo(t, app, "PUT", "/v1.0/push/preferences", `{"streak_reminder":{"enabled":true,"time":"25:00"},"come_play":{"enabled":true}}`, true); res.StatusCode != 400 {
		t.Errorf("bad time = %d", res.StatusCode)
	}
	if res := pushDo(t, app, "PUT", "/v1.0/push/preferences", `{"admin":true}`, true); res.StatusCode != 400 {
		t.Errorf("unknown field = %d", res.StatusCode)
	}
}

func TestPushTestSendsOnlyToTheCaller(t *testing.T) {
	inv := &recordingInvoker{}
	app := pushApp(t, true, inv)
	if res := pushDo(t, app, "POST", "/v1.0/push/test", "", true); res.StatusCode != 202 {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if len(inv.jobs) != 1 || len(inv.jobs[0].Recipients) != 1 || inv.jobs[0].Recipients[0] != "player-1" || inv.jobs[0].EventID == "" {
		t.Errorf("jobs = %+v", inv.jobs)
	}
}

func TestPushTestWhenPushIsOffIs503(t *testing.T) {
	inv := &recordingInvoker{}
	app := pushApp(t, false, inv)
	if res := pushDo(t, app, "POST", "/v1.0/push/test", "", true); res.StatusCode != 503 || len(inv.jobs) != 0 {
		t.Errorf("status = %d jobs = %d", res.StatusCode, len(inv.jobs))
	}
}
