package main

import (
	"net/http"

	"github.com/domino14/word_db_server/internal/searchserver"
	"github.com/domino14/word_db_server/rpc/wordsearcher"
)

func main() {
	server := &searchserver.Server{}
	twirpHandler := wordsearcher.NewQuestionSearcherServer(server, nil)

	http.ListenAndServe(":8180", twirpHandler)
}
