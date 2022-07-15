package main

import (
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/domino14/word_db_server/internal/anagramserver"
	"github.com/domino14/word_db_server/internal/searchserver"
	"github.com/domino14/word_db_server/rpc/wordsearcher"
)

// Useful for moo.bot

const (
	txtLimit = 375
)

func writeError(w http.ResponseWriter, err string) {
	w.WriteHeader(400)
	w.Write([]byte(err))
}

func plainTextHandler(wordSearchServer *searchserver.WordSearchServer, anagramserver *anagramserver.Server) http.Handler {

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method, ok := r.URL.Query()["method"]
		if !ok || len(method[0]) < 1 {
			writeError(w, "method required")
			return
		}
		switch method[0] {
		case "anagram":
			anagram(anagramserver, w, r)
		case "define":
			define(wordSearchServer, w, r)
		case "pattern":
			patternSearch(wordSearchServer, w, r)
		case "related":
			definitionSearch(wordSearchServer, w, r)
		default:
			writeError(w, "method not found")
		}

	})
}

func writeWords(w http.ResponseWriter, words []*wordsearcher.Word) {

	sort.Slice(words, func(i, j int) bool {
		return words[i].Word < words[j].Word
	})

	var s strings.Builder

	if len(words) == 0 {
		s.WriteString("no words found")
		return
	}
	plural := ""
	if len(words) > 1 {
		plural = "s"
	}

	s.WriteString(fmt.Sprintf("%d word%s found: ", len(words), plural))
	for _, w := range words {
		s.WriteString(w.Word)
		s.WriteString(" ")
		if s.Len() > txtLimit {
			s.WriteString(" (...truncated)")
			break
		}
	}
	w.Write([]byte(s.String()))
}

func anagram(anagramserver *anagramserver.Server, w http.ResponseWriter, r *http.Request) {
	letters, ok := r.URL.Query()["letters"]
	if !ok || len(letters[0]) < 1 {
		writeError(w, "letters required")
		return
	}
	lexicon, ok := r.URL.Query()["lexicon"]
	if !ok || len(lexicon[0]) < 1 {
		lexicon = []string{"CSW21"}
	}
	res, err := anagramserver.Anagram(r.Context(), &wordsearcher.AnagramRequest{
		Lexicon: lexicon[0],
		Letters: letters[0]})
	if err != nil {
		writeError(w, err.Error())
		return
	}

	writeWords(w, res.Words)
}

func define(wsServer *searchserver.WordSearchServer, w http.ResponseWriter, r *http.Request) {
	word, ok := r.URL.Query()["word"]
	if !ok || len(word[0]) < 1 {
		writeError(w, "word required")
		return
	}
	lexicon, ok := r.URL.Query()["lexicon"]
	if !ok || len(lexicon[0]) < 1 {
		lexicon = []string{"CSW21"}
	}
	res, err := wsServer.GetWordInformation(r.Context(), &wordsearcher.DefineRequest{
		Lexicon: lexicon[0], Word: word[0],
	})
	if err != nil {
		writeError(w, err.Error())
		return
	}
	if len(res.Words) == 0 {
		w.Write([]byte("word " + word[0] + " not found."))
		return
	}
	w.Write([]byte(res.Words[0].Definition))
}

func patternSearch(wsServer *searchserver.WordSearchServer, w http.ResponseWriter, r *http.Request) {
	pattern, ok := r.URL.Query()["pattern"]
	if !ok || len(pattern[0]) < 1 {
		writeError(w, "pattern required")
		return
	}
	lexicon, ok := r.URL.Query()["lexicon"]
	if !ok || len(lexicon[0]) < 1 {
		lexicon = []string{"CSW21"}
	}
	res, err := wsServer.WordSearch(r.Context(), &wordsearcher.WordSearchRequest{
		Lexicon: lexicon[0], Glob: pattern[0], AppliesTo: "word",
	})
	if err != nil {
		writeError(w, err.Error())
		return
	}
	if len(res.Words) == 0 {
		w.Write([]byte("no words match this pattern."))
		return
	}
	writeWords(w, res.Words)
}

func definitionSearch(wsServer *searchserver.WordSearchServer, w http.ResponseWriter, r *http.Request) {
	pattern, ok := r.URL.Query()["pattern"]
	if !ok || len(pattern[0]) < 1 {
		writeError(w, "pattern required")
		return
	}
	lexicon, ok := r.URL.Query()["lexicon"]
	if !ok || len(lexicon[0]) < 1 {
		lexicon = []string{"CSW21"}
	}
	res, err := wsServer.WordSearch(r.Context(), &wordsearcher.WordSearchRequest{
		Lexicon: lexicon[0], Glob: "*" + pattern[0] + "*", AppliesTo: "definition",
	})
	if err != nil {
		writeError(w, err.Error())
		return
	}
	if len(res.Words) == 0 {
		w.Write([]byte("no related words."))
		return
	}
	writeWords(w, res.Words)
}
