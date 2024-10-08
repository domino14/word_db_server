package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"connectrpc.com/connect"
	wglconfig "github.com/domino14/word-golib/config"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justinas/alice"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/hlog"
	"github.com/rs/zerolog/log"

	"github.com/domino14/word_db_server/api/rpc/wordsearcher/wordsearcherconnect"
	"github.com/domino14/word_db_server/api/rpc/wordvault/wordvaultconnect"
	"github.com/domino14/word_db_server/config"
	"github.com/domino14/word_db_server/internal/anagramserver"
	"github.com/domino14/word_db_server/internal/searchserver"
	"github.com/domino14/word_db_server/internal/stores/models"
	"github.com/domino14/word_db_server/internal/wordvault"
)

const (
	GracefulShutdownTimeout = 10 * time.Second
)

func main() {

	cfg := &config.Config{}
	cfg.Load(os.Args[1:])

	log.Info().Interface("config", cfg).Msg("searchserver-started")

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if strings.ToLower(cfg.LogLevel) == "debug" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	log.Debug().Msg("debug logging is on")

	log.Info().Msg("setting up migration")
	m, err := migrate.New(cfg.DBMigrationsPath, cfg.DBConnUri)
	if err != nil {
		panic(err)
	}
	log.Info().Msg("bringing up migration")
	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		panic(err)
	}
	e1, e2 := m.Close()
	log.Err(e1).Msg("close-source")
	log.Err(e2).Msg("close-database")

	dbPool, err := pgxpool.New(context.Background(), cfg.DBConnUri)
	if err != nil {
		panic(err)
	}
	queries := models.New(dbPool)

	mux := http.NewServeMux()
	// Add connect RPC endpoints.

	searchServer := &searchserver.Server{
		Config: cfg,
	}
	anagramServer := &anagramserver.Server{
		Config: &wglconfig.Config{DataPath: cfg.DataPath},
	}
	wordSearchServer := &searchserver.WordSearchServer{
		Config: cfg,
	}
	wordvaultServer := wordvault.NewServer(cfg, dbPool, queries, searchServer)
	mux.Handle("/plainsearch", plainTextHandler(wordSearchServer, anagramServer))

	api := http.NewServeMux()

	interceptors := connect.WithInterceptors(NewAuthInterceptor([]byte(cfg.SecretKey)))

	api.Handle(wordsearcherconnect.NewAnagrammerHandler(anagramServer))
	api.Handle(wordsearcherconnect.NewQuestionSearcherHandler(searchServer))
	api.Handle(wordsearcherconnect.NewWordSearcherHandler(wordSearchServer))
	// Only this latter service requires user auth:
	api.Handle(wordvaultconnect.NewWordVaultServiceHandler(wordvaultServer, interceptors))

	apichain := alice.New(
		hlog.NewHandler(log.With().Str("service", "word-db-server").Logger()),
		hlog.AccessHandler(func(r *http.Request, status int, size int, d time.Duration) {
			// Extract client IP address
			clientIP := r.RemoteAddr
			if forwardedFor := r.Header.Get("X-Forwarded-For"); forwardedFor != "" {
				// X-Forwarded-For can contain multiple IPs, use the first one
				clientIP = strings.Split(forwardedFor, ",")[0]
			}

			hlog.FromRequest(r).Info().
				Str("path", r.URL.Path).
				Str("clientIP", clientIP).
				Int("status", status).
				Int("size", size).
				Dur("duration", d).Msg("")
		}),
	).Then(api)

	mux.Handle("/api/", http.StripPrefix("/api", apichain))

	srv := &http.Server{
		Addr:    ":8180",
		Handler: mux,
	}
	idleConnsClosed := make(chan struct{})

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
		<-sig
		// We received an interrupt signal, shut down.
		log.Info().Msg("got quit signal...")
		ctx, cancel := context.WithTimeout(context.Background(), GracefulShutdownTimeout)

		if err := srv.Shutdown(ctx); err != nil {
			// Error from closing listeners, or context timeout:
			log.Error().Msgf("HTTP server Shutdown: %v", err)
		}
		cancel()
		close(idleConnsClosed)
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal().Err(err).Msg("")
	}
	<-idleConnsClosed
	log.Info().Msg("server gracefully shutting down")
}
