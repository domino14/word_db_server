package searchserver

import (
	"database/sql"
	"errors"
	"path/filepath"

	// sqlite3 driver is used by this server.
	_ "github.com/mattn/go-sqlite3"
)

const (
	// MaxSQLChunkSize is how many parameters we allow to put in a SQLite
	// query. The actual limit is something around 1000 unless we
	// recompile the plugin.
	MaxSQLChunkSize = 950
)

// Server implements the WordSearcher service
type Server struct {
	LexiconPath string
}

func (s *Server) getDbConnection(lexName string) (*sql.DB, error) {
	// Try to connect to the db.
	if lexName == "" {
		return nil, errors.New("lexicon not specified")
	}
	return sql.Open("sqlite3", filepath.Join(s.LexiconPath, "db", lexName+".db"))
}
