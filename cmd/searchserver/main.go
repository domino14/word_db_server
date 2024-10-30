package main

import (
	"context"
	"fmt"
	"io"
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
	"github.com/domino14/word_db_server/internal/auth"
	"github.com/domino14/word_db_server/internal/searchserver"
	"github.com/domino14/word_db_server/internal/stores/models"
	"github.com/domino14/word_db_server/internal/wordvault"
)

const (
	GracefulShutdownTimeout = 10 * time.Second
)

const maxUploadSize = 25 << 20 // 25 MB

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
	mux.Handle("/import-cardbox/", http.HandlerFunc(importCardboxHandler(queries, searchServer, dbPool, []byte(cfg.SecretKey))))

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

func importCardboxHandler(queries *models.Queries, searchServer *searchserver.Server, dbPool *pgxpool.Pool, secretKey []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, err := authenticateJWT(r.Context(), r.Header, secretKey)
		if err != nil {
			http.Error(w, err.Error(), http.StatusUnauthorized)
			return
		}
		u := auth.UserFromContext(ctx)
		if u != nil {
			log.Info().Str("username", u.Username).Msg("authenticated-user-uploading-cardbox")
		} else {
			http.Error(w, "unexpected-empty-user", http.StatusUnauthorized)
			return
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		// Limit the size of the request body to prevent denial of service attacks
		r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)
		if err := r.ParseMultipartForm(maxUploadSize); err != nil {
			http.Error(w, "The uploaded file is too big. Please choose a file that's less than 25MB in size.", http.StatusBadRequest)
			return
		}

		// Read the "lexicon" parameter from the form data
		lexicon := r.FormValue("lexicon")
		if lexicon == "" {
			http.Error(w, "Missing 'lexicon' parameter.", http.StatusBadRequest)
			return
		}

		file, _, err := r.FormFile("file")
		if err != nil {
			http.Error(w, "Unable to read uploaded file.", http.StatusBadRequest)
			return
		}
		defer file.Close()

		// Read the first 16 bytes to check for the SQLite magic header
		buffer := make([]byte, 16)
		if _, err := io.ReadFull(file, buffer); err != nil {
			http.Error(w, "Unable to read uploaded file.", http.StatusInternalServerError)
			return
		}

		// Verify the SQLite magic header
		magicHeader := "SQLite format 3\x00"
		if string(buffer[:len(magicHeader)]) != magicHeader {
			http.Error(w, "Uploaded file is not a valid SQLite file.", http.StatusBadRequest)
			return
		}

		// Create a temporary file to save the uploaded SQLite database
		tempFile, err := os.CreateTemp("", "uploaded-*.sqlite")
		if err != nil {
			http.Error(w, "Unable to create temporary file.", http.StatusInternalServerError)
			return
		}
		defer func() {
			tempFile.Close()
			os.Remove(tempFile.Name()) // Clean up the file after processing
		}()

		// Write the initial 16 bytes (already read) to the temp file
		if _, err := tempFile.Write(buffer); err != nil {
			http.Error(w, "Unable to write to temporary file.", http.StatusInternalServerError)
			return
		}

		// Copy the rest of the file to the temp file
		if _, err := io.Copy(tempFile, file); err != nil {
			http.Error(w, "Unable to write to temporary file.", http.StatusInternalServerError)
			return
		}

		// Process the SQLite database
		if nAdded, invalidAlphas, err := wordvault.LeitnerImport(ctx, searchServer, lexicon, queries, dbPool, tempFile.Name()); err != nil {
			log.Err(err).Msg("error-cardbox-import")
			http.Error(w, "Error processing SQLite database.", http.StatusInternalServerError)
			return
		} else {
			fmt.Fprintf(w, "Imported %d cards into your WordVault. Alphagrams not found: %v", nAdded, invalidAlphas)
			return
		}

	}
}
