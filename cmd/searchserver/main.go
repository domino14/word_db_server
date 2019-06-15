package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"

	"github.com/domino14/macondo/anagrammer"
	"github.com/domino14/word_db_server/internal/searchserver"
	"github.com/domino14/word_db_server/rpc/wordsearcher"
)

var LogLevel = os.Getenv("LOG_LEVEL")
var LexiconPath = os.Getenv("LEXICON_PATH")

func main() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if strings.ToLower(LogLevel) == "debug" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	server := &searchserver.Server{
		LexiconPath: LexiconPath,
	}
	twirpHandler := wordsearcher.NewQuestionSearcherServer(server, nil)

	// Initialize the Macondo anagrammer, which is needed by this search
	// server for various conditions.
	anagrammer.LoadDawgs(filepath.Join(LexiconPath, "dawg"))

	http.ListenAndServe(":8180", twirpHandler)
}
