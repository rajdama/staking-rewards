// Package api serves indexed staking data as JSON.
package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/ethereum/go-ethereum/common"
	"github.com/rajdama/staking-rewards/indexer/internal/store"
)

// Reader is the data the API serves (the Postgres store in production).
type Reader interface {
	Stats(ctx context.Context) (store.Stats, error)
	Activity(ctx context.Context, limit int, user string) ([]store.Activity, error)
	Leaderboard(ctx context.Context, limit int) ([]store.Leader, error)
	History(ctx context.Context, days int) ([]store.HistoryPoint, error)
}

type Server struct {
	data Reader
	log  *slog.Logger
}

func New(data Reader, log *slog.Logger) *Server {
	return &Server{data: data, log: log}
}

// Handler returns the routes, with CORS enabled so any frontend can read the API.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/stats", s.stats)
	mux.HandleFunc("GET /api/activity", s.activity)
	mux.HandleFunc("GET /api/users/{address}/activity", s.userActivity)
	mux.HandleFunc("GET /api/leaderboard", s.leaderboard)
	mux.HandleFunc("GET /api/history", s.history)
	return cors(mux)
}

type list[T any] struct {
	Items []T `json:"items"`
}

func (s *Server) stats(w http.ResponseWriter, r *http.Request) {
	st, err := s.data.Stats(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) activity(w http.ResponseWriter, r *http.Request) {
	items, err := s.data.Activity(r.Context(), intParam(r, "limit", 20, 1, 100), "")
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list[store.Activity]{Items: nonNil(items)})
}

func (s *Server) userActivity(w http.ResponseWriter, r *http.Request) {
	address := r.PathValue("address")
	if !common.IsHexAddress(address) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid address"})
		return
	}
	items, err := s.data.Activity(r.Context(), intParam(r, "limit", 50, 1, 200), address)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list[store.Activity]{Items: nonNil(items)})
}

func (s *Server) leaderboard(w http.ResponseWriter, r *http.Request) {
	items, err := s.data.Leaderboard(r.Context(), intParam(r, "limit", 10, 1, 100))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list[store.Leader]{Items: nonNil(items)})
}

func (s *Server) history(w http.ResponseWriter, r *http.Request) {
	items, err := s.data.History(r.Context(), intParam(r, "days", 30, 1, 365))
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, list[store.HistoryPoint]{Items: nonNil(items)})
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	s.log.Error("api error", "err", err)
	writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
}

// intParam reads an integer query parameter, clamped to [min, max].
func intParam(r *http.Request, name string, fallback, min, max int) int {
	v, err := strconv.Atoi(r.URL.Query().Get(name))
	if err != nil {
		return fallback
	}
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

func nonNil[T any](items []T) []T {
	if items == nil {
		return []T{}
	}
	return items
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
