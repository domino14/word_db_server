package searchserver

import (
	"context"
	"os"
	"testing"

	pb "github.com/domino14/word_db_server/rpc/wordsearcher"
	"github.com/stretchr/testify/assert"
)

var LexiconDir = os.Getenv("LEXICON_PATH")

func alphagrams(sr *pb.SearchResponse) []string {
	strs := []string{}
	for _, alph := range sr.Alphagrams {
		strs = append(strs, alph.Alphagram)
	}
	return strs
}

func searchHelper(req *pb.SearchRequest) []string {
	s := &Server{}
	sr, err := s.Search(context.Background(), req)

	if err != nil {
		panic(err)
	}
	return alphagrams(sr)
}

// These tests should test functionality more than the actual query
// themselves. We want to know what the query returns after being executed.
func TestProbabilityWordSearch(t *testing.T) {
	req := WordSearch([]*pb.SearchRequest_SearchParam{
		SearchDescLexicon("America"),
		SearchDescLength(8, 8),
		SearchDescProbRange(200, 203),
	})

	assert.Equal(t, []string{"ADEEGNOR", "EEGILNOR", "ADEEGORT", "AEEGLNOT"},
		searchHelper(req))
}
