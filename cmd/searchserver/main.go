package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/domino14/word_db_server/dbmaker"
	"github.com/domino14/word_db_server/internal/anagramserver"
	"github.com/domino14/word_db_server/internal/searchserver"
	"github.com/domino14/word_db_server/rpc/wordsearcher"
)

var LogLevel = os.Getenv("LOG_LEVEL")
var LexiconPath = os.Getenv("LEXICON_PATH")
var InitializeSelf = os.Getenv("INITIALIZE_SELF")
var SupportedLexica = os.Getenv("SUPPORTED_LEXICA")

const (
	GracefulShutdownTimeout = 10 * time.Second
)

func main() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if strings.ToLower(LogLevel) == "debug" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	supportedLexica := []string{}
	for _, sl := range strings.Split(SupportedLexica, ",") {
		supportedLexica = append(supportedLexica,
			strings.ToUpper(strings.TrimSpace(sl)))
	}

	if InitializeSelf == "true" {
		recreateDataStructures(supportedLexica)
	}

	searchServer := &searchserver.Server{
		LexiconPath:     LexiconPath,
		SupportedLexica: supportedLexica,
	}
	// anagramServer doesn't need supportedLexica because it loads all its
	// structures from the dawgs directory.
	anagramServer := &anagramserver.Server{
		LexiconPath: LexiconPath,
	}
	anagramServer.Initialize()
	searchHandler := wordsearcher.NewQuestionSearcherServer(searchServer, nil)
	anagramHandler := wordsearcher.NewAnagrammerServer(anagramServer, nil)

	mux := http.NewServeMux()
	mux.Handle(searchHandler.PathPrefix(), searchHandler)
	mux.Handle(anagramHandler.PathPrefix(), anagramHandler)

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

func recreateDataStructures(supportedLexica []string) {
	// Fetch the lexica files.
	// XXX: assume they are in LEXICON_PATH
	os.MkdirAll(filepath.Join(LexiconPath, "dawg"), os.ModePerm)
	os.MkdirAll(filepath.Join(LexiconPath, "db"), os.ModePerm)
	log.Info().Msg("creating databases...")
	symbols, lexiconMap := dbmaker.LexiconMappings()
	for lexName, info := range lexiconMap {
		if !searchserver.StrInList(lexName, supportedLexica) {
			log.Info().Msgf("%v not in supported lexica list, skipping", lexName)
			continue
		}
		if info.Dawg == nil || info.Dawg.GetAlphabet() == nil {
			log.Info().Msgf("%v info dawg was null", lexName)
			continue
		}
		info.Initialize()
		log.Info().Msgf("Creating database for %v", lexName)
		dbmaker.CreateLexiconDatabase(lexName, info, symbols, lexiconMap,
			filepath.Join(LexiconPath, "db"), true)
	}
}
