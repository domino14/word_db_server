package searchserver

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	// sqlite3 driver is used by this server.
	"github.com/domino14/word_db_server/config"
	_ "github.com/mattn/go-sqlite3"
	"github.com/rs/zerolog/log"
)

const (
	// MaxSQLChunkSize is how many parameters we allow to put in a SQLite
	// query. The actual limit is something around 1000 unless we
	// recompile the plugin.
	MaxSQLChunkSize = 950
)

// Server implements the WordSearcher service
type Server struct {
	Config *config.Config
}

func getDbConnection(cfg *config.Config, lexName string) (*sql.DB, error) {
	// Try to connect to the db.
	if lexName == "" {
		return nil, errors.New("lexicon not specified")
	}

	lexPath := filepath.Join(cfg.DataPath, "lexica")

	fileName := filepath.Join(lexPath, "db", lexName+".db")
	_, err := os.Stat(fileName)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("the lexicon %v is not supported", lexName)
	}
	return sql.Open("sqlite3", fileName)
}

func timeTrack(start time.Time, name string) {
	elapsed := time.Since(start)
	log.Info().Dur("elapsed-ms", elapsed).Str("name", name).Msgf("time-track")
}

func (s *Server) HasAlphagram(alpha, lexicon string) (bool, error) {
	db, err := getDbConnection(s.Config, lexicon)
	if err != nil {
		return false, err
	}
	defer db.Close()
	// Prepare the query
	var count int
	err = db.QueryRow("SELECT count(*) FROM alphagrams WHERE alphagram = ?", alpha).Scan(&count)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, nil // No rows found means no matching alphagram
		}
		return false, fmt.Errorf("failed to execute query: %w", err)
	}
	return count > 0, nil
}
