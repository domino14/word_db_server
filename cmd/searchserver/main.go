package main

import (
	"net/http"
	"os"
	"strings"

	"github.com/rs/zerolog"

	"github.com/domino14/word_db_server/internal/anagramserver"
	"github.com/domino14/word_db_server/internal/searchserver"
	"github.com/domino14/word_db_server/rpc/anagrammer"
	"github.com/domino14/word_db_server/rpc/wordsearcher"
)

var LogLevel = os.Getenv("LOG_LEVEL")
var LexiconPath = os.Getenv("LEXICON_PATH")

func main() {
	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if strings.ToLower(LogLevel) == "debug" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	searchServer := &searchserver.Server{
		LexiconPath: LexiconPath,
	}
	anagramServer := &anagramserver.Server{
		LexiconPath: LexiconPath,
	}
	anagramServer.Initialize()
	searchHandler := wordsearcher.NewQuestionSearcherServer(searchServer, nil)
	anagramHandler := anagrammer.NewAnagrammerServer(anagramServer, nil)

	mux := http.NewServeMux()
	mux.Handle(searchHandler.PathPrefix(), searchHandler)
	mux.Handle(anagramHandler.PathPrefix(), anagramHandler)

	http.ListenAndServe(":8180", mux)
}
