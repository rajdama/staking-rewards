package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rajdama/staking-rewards/indexer/internal/store"
)

type fakeReader struct {
	fail      bool
	lastLimit int
	lastUser  string
	lastDays  int
}

var errBoom = errors.New("boom")

func (f *fakeReader) Stats(context.Context) (store.Stats, error) {
	if f.fail {
		return store.Stats{}, errBoom
	}
	return store.Stats{TotalStaked: "100", Stakers: 2}, nil
}

func (f *fakeReader) Activity(_ context.Context, limit int, user string) ([]store.Activity, error) {
	f.lastLimit, f.lastUser = limit, user
	return nil, nil
}

func (f *fakeReader) Leaderboard(_ context.Context, limit int) ([]store.Leader, error) {
	f.lastLimit = limit
	return []store.Leader{{User: "0xabc", Staked: "1", Effective: "3", Claimed: "0"}}, nil
}

func (f *fakeReader) History(_ context.Context, days int) ([]store.HistoryPoint, error) {
	f.lastDays = days
	return []store.HistoryPoint{{Timestamp: 1, TotalStaked: "5"}}, nil
}

func serve(t *testing.T, r Reader, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	h := New(r, slog.New(slog.NewTextHandler(io.Discard, nil))).Handler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, nil))
	return rec
}

func TestStats(t *testing.T) {
	rec := serve(t, &fakeReader{}, http.MethodGet, "/api/stats")
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var st store.Stats
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil || st.TotalStaked != "100" || st.Stakers != 2 {
		t.Fatalf("body %s err=%v", rec.Body, err)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Fatal("missing CORS header")
	}
}

func TestStatsError(t *testing.T) {
	rec := serve(t, &fakeReader{fail: true}, http.MethodGet, "/api/stats")
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d", rec.Code)
	}
}

func TestActivityReturnsEmptyListNotNull(t *testing.T) {
	f := &fakeReader{}
	rec := serve(t, f, http.MethodGet, "/api/activity?limit=5")
	if rec.Code != http.StatusOK || rec.Body.String() != "{\"items\":[]}\n" {
		t.Fatalf("status %d body %q", rec.Code, rec.Body)
	}
	if f.lastLimit != 5 || f.lastUser != "" {
		t.Fatalf("limit=%d user=%q", f.lastLimit, f.lastUser)
	}
}

func TestLimitsAreClamped(t *testing.T) {
	f := &fakeReader{}
	serve(t, f, http.MethodGet, "/api/leaderboard?limit=100000")
	if f.lastLimit != 100 {
		t.Fatalf("limit not clamped: %d", f.lastLimit)
	}
	serve(t, f, http.MethodGet, "/api/leaderboard?limit=abc")
	if f.lastLimit != 10 {
		t.Fatalf("bad limit should use default, got %d", f.lastLimit)
	}
	serve(t, f, http.MethodGet, "/api/history?days=0")
	if f.lastDays != 1 {
		t.Fatalf("days not clamped: %d", f.lastDays)
	}
}

func TestUserActivity(t *testing.T) {
	f := &fakeReader{}
	addr := "0x00000000000000000000000000000000000A11CE"
	if rec := serve(t, f, http.MethodGet, "/api/users/"+addr+"/activity"); rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if f.lastUser != addr {
		t.Fatalf("user = %q", f.lastUser)
	}
	if rec := serve(t, f, http.MethodGet, "/api/users/not-an-address/activity"); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid address status %d", rec.Code)
	}
}

func TestPreflightAndHealth(t *testing.T) {
	if rec := serve(t, &fakeReader{}, http.MethodOptions, "/api/stats"); rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status %d", rec.Code)
	}
	if rec := serve(t, &fakeReader{}, http.MethodGet, "/healthz"); rec.Code != http.StatusOK {
		t.Fatalf("health status %d", rec.Code)
	}
}
