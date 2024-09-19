package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	wglconfig "github.com/domino14/word-golib/config"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/domino14/word_db_server/config"
	"github.com/domino14/word_db_server/internal/anagramserver"
	"github.com/domino14/word_db_server/internal/searchserver"
	"github.com/domino14/word_db_server/rpc/api/wordsearcher/wordsearcherconnect"
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
	mux.Handle("/plainsearch", plainTextHandler(wordSearchServer, anagramServer))

	api := http.NewServeMux()
	api.Handle(wordsearcherconnect.NewAnagrammerHandler(anagramServer))
	api.Handle(wordsearcherconnect.NewQuestionSearcherHandler(searchServer))
	api.Handle(wordsearcherconnect.NewWordSearcherHandler(wordSearchServer))

	mux.Handle("/api/", http.StripPrefix("/api", api))

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
