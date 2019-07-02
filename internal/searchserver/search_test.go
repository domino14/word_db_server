package searchserver

import (
	"context"
	"os"
	"testing"

	pb "github.com/domino14/word_db_server/rpc/wordsearcher"
	"github.com/stretchr/testify/assert"
)

func alphagrams(sr *pb.SearchResponse) []string {
	return alphsFromPB(sr.Alphagrams)
}

func alphsFromPB(pba []*pb.Alphagram) []string {
	strs := []string{}
	for _, alph := range pba {
		strs = append(strs, alph.Alphagram)
	}
	return strs
}

func searchHelper(req *pb.SearchRequest) (*pb.SearchResponse, error) {
	s := &Server{
		LexiconPath: os.Getenv("LEXICON_PATH"),
	}
	sr, err := s.Search(context.Background(), req)

	if err != nil {
		return nil, err
	}
	return sr, nil
}

// These tests should test functionality more than the actual query
// themselves. We want to know what the query returns after being executed.
func TestProbabilityWordSearchNoExpand(t *testing.T) {
	req := WordSearch([]*pb.SearchRequest_SearchParam{
		SearchDescLexicon("NWL18"),
		SearchDescLength(8, 8),
		SearchDescProbRange(201, 204),
	}, false)

	resp, err := searchHelper(req)
	assert.Nil(t, err)
	assert.Equal(t, []string{"ADEEGORT", "AEEGLNOT", "DEEGIORT", "EEGILNOR"},
		alphagrams(resp))
	assert.Equal(t, false, resp.Alphagrams[0].ExpandedRepr)
}

func TestProbabilityWordSearchExpand(t *testing.T) {
	req := WordSearch([]*pb.SearchRequest_SearchParam{
		SearchDescLexicon("NWL18"),
		SearchDescLength(8, 8),
		SearchDescProbRange(201, 204),
	}, true)

	resp, _ := searchHelper(req)
	assert.Equal(t, resp.Alphagrams[0].Alphagram, "ADEEGORT")
	assert.Equal(t, resp.Alphagrams[0].Words[0].Word, "DEROGATE")
	assert.Equal(t, resp.Alphagrams[0].Words[0].BackHooks, "DS")
}

func TestProbabilityAndPoints(t *testing.T) {
	req := WordSearch([]*pb.SearchRequest_SearchParam{
		SearchDescLexicon("NWL18"),
		SearchDescLength(8, 8),
		SearchDescProbRange(3060, 3080),
		SearchDescPointValue(14, 30),
	}, false)
	resp, _ := searchHelper(req)
	assert.Equal(t, []string{"DEINOPRY", "DEINORVW", "EHILORTY", "EHIORSTW"},
		alphagrams(resp))
}

func TestPoints(t *testing.T) {
	req := WordSearch([]*pb.SearchRequest_SearchParam{
		SearchDescLexicon("NWL18"),
		SearchDescLength(7, 7),
		SearchDescPointValue(40, 100),
	}, false)
	resp, _ := searchHelper(req)
	assert.Equal(t, []string{"AVYYZZZ", "AIPZZZZ"}, alphagrams(resp))
}

func TestNumAnagrams(t *testing.T) {
	req := WordSearch([]*pb.SearchRequest_SearchParam{
		SearchDescLexicon("NWL18"),
		SearchDescLength(7, 7),
		SearchDescNumAnagrams(8, 100),
	}, false)
	resp, _ := searchHelper(req)
	assert.Equal(t, []string{"AEINRST", "AEGINRS", "AEGINST",
		"EORSSTU", "EIPRSST"}, alphagrams(resp))
}

func TestPtsNumAnagrams(t *testing.T) {
	req := WordSearch([]*pb.SearchRequest_SearchParam{
		SearchDescLexicon("NWL18"),
		SearchDescLength(7, 7),
		SearchDescNumAnagrams(8, 100),
		SearchDescPointValue(8, 100),
	}, false)
	resp, _ := searchHelper(req)
	assert.Equal(t, []string{"AEGINRS", "AEGINST", "EIPRSST"}, alphagrams(resp))
}

func TestAlphagramList(t *testing.T) {
	req := WordSearch([]*pb.SearchRequest_SearchParam{
		SearchDescLexicon("NWL18"),
		SearchDescAlphagramList([]string{"DEGORU", "AAAIMNORT", "DGOS"}),
	}, false)
	resp, _ := searchHelper(req)
	assert.Equal(t, []string{"DGOS", "DEGORU", "AAAIMNORT"}, alphagrams(resp))
}

func TestProbabilityList(t *testing.T) {
	req := WordSearch([]*pb.SearchRequest_SearchParam{
		SearchDescLexicon("NWL18"),
		SearchDescLength(7, 7),
		SearchDescProbabilityList([]int32{92, 73, 85, 61}),
	}, false)
	resp, _ := searchHelper(req)
	assert.Equal(t, []string{"AINORTU", "ADELNOR", "AENORSU", "EILNORS"},
		alphagrams(resp))
}

func TestNotEnoughParams(t *testing.T) {
	req := WordSearch([]*pb.SearchRequest_SearchParam{
		SearchDescLength(7, 7),
	}, false)
	resp, err := searchHelper(req)
	assert.Nil(t, resp)
	assert.EqualError(t, err, "the first condition must be a lexicon")
}

func TestNoLexicon(t *testing.T) {
	req := WordSearch([]*pb.SearchRequest_SearchParam{
		SearchDescLength(7, 7),
		SearchDescNumAnagrams(5, 8),
	}, false)
	resp, err := searchHelper(req)
	assert.Nil(t, resp)
	assert.EqualError(t, err, "the first condition must be a lexicon")
}

func TestProbabilityLimitUnallowed(t *testing.T) {
	req := WordSearch([]*pb.SearchRequest_SearchParam{
		SearchDescLexicon("NWL18"),
		SearchDescLength(7, 7),
		SearchDescProbLimit(1, 3),
		SearchDescProbabilityList([]int32{5, 6, 10, 40}),
	}, false)
	resp, err := searchHelper(req)
	assert.Nil(t, resp)
	assert.EqualError(t, err, "mutually exclusive search conditions not allowed")
}

func TestProbabilityLimitSecond(t *testing.T) {
	req := WordSearch([]*pb.SearchRequest_SearchParam{
		SearchDescLexicon("NWL18"),
		SearchDescLength(7, 7),
		SearchDescPointValue(40, 100),
		SearchDescProbLimit(2, 2),
	}, false)
	resp, err := searchHelper(req)
	assert.Nil(t, err)
	assert.Equal(t, []string{"AIPZZZZ"}, alphagrams(resp))
}

func TestProbabilityLimitFirst(t *testing.T) {
	req := WordSearch([]*pb.SearchRequest_SearchParam{
		SearchDescLexicon("NWL18"),
		SearchDescLength(7, 7),
		SearchDescPointValue(40, 100),
		SearchDescProbLimit(1, 1),
	}, false)
	resp, err := searchHelper(req)
	assert.Nil(t, err)
	assert.Equal(t, []string{"AVYYZZZ"}, alphagrams(resp))
}

func TestProbabilityLimitMany(t *testing.T) {
	req := WordSearch([]*pb.SearchRequest_SearchParam{
		SearchDescLexicon("NWL18"),
		SearchDescLength(7, 7),
		SearchDescPointValue(40, 100),
		SearchDescProbLimit(1, 50),
	}, false)
	resp, err := searchHelper(req)
	assert.Nil(t, err)
	assert.Equal(t, []string{"AVYYZZZ", "AIPZZZZ"}, alphagrams(resp))
}

func TestProbabilityLimitAnother(t *testing.T) {
	req := WordSearch([]*pb.SearchRequest_SearchParam{
		SearchDescLexicon("NWL18"),
		SearchDescLength(7, 7),
		SearchDescNumAnagrams(8, 100),
		SearchDescProbLimit(3, 4),
	}, false)
	resp, err := searchHelper(req)
	assert.Nil(t, err)
	assert.Equal(t, []string{"AEGINST", "EORSSTU"}, alphagrams(resp))
}

func TestProbabilityLimitOutsideOfRange(t *testing.T) {
	req := WordSearch([]*pb.SearchRequest_SearchParam{
		SearchDescLexicon("NWL18"),
		SearchDescLength(7, 7),
		SearchDescNumAnagrams(8, 100),
		SearchDescProbLimit(10, 20),
	}, false)
	resp, err := searchHelper(req)
	assert.Nil(t, err)
	assert.Equal(t, []string{}, alphagrams(resp))
}

func TestNotInLexicon(t *testing.T) {
	req := WordSearch([]*pb.SearchRequest_SearchParam{
		SearchDescLexicon("NWL18"),
		SearchDescLength(2, 4),
		SearchDescNotInLexicon(pb.SearchRequest_PREVIOUS_VERSION),
	}, false)
	resp, err := searchHelper(req)
	assert.Nil(t, err)
	assert.Equal(t, []string{"EW", "KO", "EIOW", "ENZ", "INNO", "AEPV", "EKUY"},
		alphagrams(resp))
}

func TestProbabilityListMultipleQueries(t *testing.T) {
	expand := true
	req := WordSearch([]*pb.SearchRequest_SearchParam{
		SearchDescLexicon("NWL18"),
		SearchDescLength(7, 7),
		SearchDescProbabilityList([]int32{92, 73, 85, 61, 185, 43, 33, 99, 25}),
	}, expand)

	maxChunkSize := 2
	qgen, err := createQueryGen(req, []string{"NWL18"}, maxChunkSize)
	assert.Nil(t, err)
	s := &Server{
		LexiconPath: os.Getenv("LEXICON_PATH"),
	}
	db, err := s.getDbConnection(qgen.LexiconName())
	assert.Nil(t, err)
	defer db.Close()
	queries, err := qgen.Generate()
	assert.Nil(t, err)
	// There should be 5 queries (max chunk size is 2 and we have 9 elements in list)
	assert.Equal(t, 5, len(queries))
	pbAlphas, err := combineQueryResults(queries, db, expand)
	assert.Nil(t, err)
	assert.Equal(t, []string{
		"ADELNOR", "EILNORS", // 73, 92
		"AINORTU", "AENORSU", // 61, 85
		"AEGINOS", "CEINORT", // 43, 185
		"AAEEINT", "AEINNRT", // 33, 99
		"AEEILNT", // 25
	}, alphsFromPB(pbAlphas))
}

func TestProbabilityListMultipleQueriesOther(t *testing.T) {
	expand := true
	req := WordSearch([]*pb.SearchRequest_SearchParam{
		SearchDescLexicon("NWL18"),
		SearchDescLength(7, 7),
		SearchDescProbabilityList([]int32{92, 73, 85, 61, 185, 43, 33, 99, 25}),
	}, expand)

	maxChunkSize := 3
	qgen, _ := createQueryGen(req, []string{"NWL18"}, maxChunkSize)
	s := &Server{
		LexiconPath: os.Getenv("LEXICON_PATH"),
	}
	db, _ := s.getDbConnection(qgen.LexiconName())
	defer db.Close()
	queries, _ := qgen.Generate()
	// There should be 3 queries (max chunk size is 2 and we have 9 elements in list)
	assert.Equal(t, 3, len(queries))
	pbAlphas, _ := combineQueryResults(queries, db, expand)
	assert.Equal(t, []string{
		"ADELNOR", "AENORSU", "EILNORS", // 73, 85, 92
		"AEGINOS", "AINORTU", "CEINORT", // 43, 61, 185
		"AEEILNT", "AAEEINT", "AEINNRT", // 25, 33, 99
	}, alphsFromPB(pbAlphas))
}
