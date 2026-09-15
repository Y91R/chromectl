// Package debugserver — служебный порт: пробы и pprof. Отдельно от основного,
// чтобы его не выставлять наружу вместе с публичным API и статикой.
package debugserver

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/pprof"
	"time"
)

// Checker сообщает, готов ли сервис принимать нагрузку.
type Checker func(ctx context.Context) error

type Server struct {
	http  *http.Server
	log   *slog.Logger
	ready Checker
}

func New(addr string, ready Checker, log *slog.Logger) *Server {
	s := &Server{log: log, ready: ready}

	mux := http.NewServeMux()
	// livez отвечает всегда: процесс жив. readyz проверяет зависимости —
	// смешивать их нельзя, иначе оркестратор перезапустит живой сервис
	// из-за недоступной на секунду базы.
	mux.HandleFunc("/livez", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", s.handleReady)
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)

	s.http = &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	return s
}

func (s *Server) handleReady(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()

	if err := s.ready(ctx); err != nil {
		// Причина остаётся в логе: текст ошибки драйвера несёт хост, пользователя
		// и имя базы, а проба читается снаружи процесса.
		s.log.Warn("сервис не готов", slog.Any("error", err))
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("not ready"))
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (s *Server) Run() error {
	s.log.Info("служебный сервер запущен", slog.String("addr", s.http.Addr))
	if err := s.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (s *Server) Shutdown(ctx context.Context) error { return s.http.Shutdown(ctx) }
