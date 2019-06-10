package searchserver

import (
	"context"
	"database/sql"

	"github.com/rs/zerolog/log"

	"github.com/domino14/word_db_server/internal/querygen"
	pb "github.com/domino14/word_db_server/rpc/wordsearcher"
)

// Expand implements the "expand" rpc command, which takes in a simple
// list of alphagrams with words and returns all the needed expanded info
// (such as definitions, hooks, etc).
func (s *Server) Expand(ctx context.Context, req *pb.SearchResponse) (*pb.SearchResponse, error) {
	lexName := req.Lexicon
	// Get all the alphagrams from the search request.
	db, err := s.getDbConnection(lexName)
	if err != nil {
		return nil, err
	}

	alphStrToObjs, err := getInputAlphagramInfo(req, db)
	if err != nil {
		return nil, err
	}

	outputAlphas, err := mergeInputWordInfo(req, alphStrToObjs, db)
	if err != nil {
		return nil, err
	}

	return &pb.SearchResponse{
		Alphagrams: outputAlphas,
		Lexicon:    lexName,
	}, nil
}

func getInputAlphagramInfo(req *pb.SearchResponse, db *sql.DB) (map[string]*pb.Alphagram, error) {
	inputAlphas := alphasFromSearchResponse(req)
	alphaQgen := querygen.NewQueryGen(req.Lexicon, querygen.AlphagramsOnly,
		[]*pb.SearchRequest_SearchParam{SearchDescAlphagramList(inputAlphas)},
		MaxSQLChunkSize)

	queries, err := alphaQgen.Generate()
	if err != nil {
		return nil, err
	}
	log.Debug().Msgf("alphaQgen generated queries %v", queries)

	alphagrams, err := combineAlphaQueryResults(queries, db)
	if err != nil {
		return nil, err
	}
	// Now we have a bunch of alphagrams with their info. Create a map
	// for fast access.
	alphStrToObjs := map[string]*pb.Alphagram{}
	for _, a := range alphagrams {
		alphStrToObjs[a.Alphagram] = a
	}
	return alphStrToObjs, nil
}

func mergeInputWordInfo(req *pb.SearchResponse, alphStrToObjs map[string]*pb.Alphagram,
	db *sql.DB) ([]*pb.Alphagram, error) {
	outputAlphas := []*pb.Alphagram{}

	wordToAlphagramDict := map[string]*pb.Alphagram{}
	for _, a := range req.Alphagrams {
		var thisa *pb.Alphagram
		var ok bool
		if thisa, ok = alphStrToObjs[a.Alphagram]; !ok {
			thisa = &pb.Alphagram{
				Alphagram: a.Alphagram,
				Length:    int32(len(a.Alphagram))}
		}
		for _, w := range a.Words {
			wordToAlphagramDict[w.Word] = thisa
		}
		outputAlphas = append(outputAlphas, thisa)
	}
	listOfWords := []string{}
	for k := range wordToAlphagramDict {
		listOfWords = append(listOfWords, k)
	}
	// Now query all of the words.
	wordsQGen := querygen.NewQueryGen(req.Lexicon, querygen.WordsOnly,
		[]*pb.SearchRequest_SearchParam{SearchDescWordList(listOfWords)},
		MaxSQLChunkSize)
	queries, err := wordsQGen.Generate()

	if err != nil {
		return nil, err
	}
	log.Debug().Msgf("Generated word queries %v", queries)
	words, err := combineWordQueryResults(queries, db)
	// Take all the words and match them with the input alphagrams.
	for _, word := range words {
		q := wordToAlphagramDict[word.Word]
		q.Words = append(q.Words, word)
	}
	return outputAlphas, nil
}

func alphasFromSearchResponse(req *pb.SearchResponse) []string {
	astrs := []string{}
	for _, a := range req.Alphagrams {
		astrs = append(astrs, a.Alphagram)
	}
	return astrs
}

func combineAlphaQueryResults(queries []*querygen.Query, db *sql.DB) ([]*pb.Alphagram, error) {
	alphagrams := []*pb.Alphagram{}
	// Execute the queries.
	for _, query := range queries {
		rows, err := db.Query(query.Rendered(), query.BindParams()...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		alphagrams = append(alphagrams, processAlphagramRows(rows)...)
	}
	return alphagrams, nil
}

func combineWordQueryResults(queries []*querygen.Query, db *sql.DB) ([]*pb.Word, error) {
	words := []*pb.Word{}
	for _, query := range queries {
		rows, err := db.Query(query.Rendered(), query.BindParams()...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		words = append(words, processWordRows(rows)...)
	}
	return words, nil
}

func processAlphagramRows(rows *sql.Rows) []*pb.Alphagram {
	alphagrams := []*pb.Alphagram{}

	for rows.Next() {
		var alphagram string
		var probability int32
		var combinations int64
		rows.Scan(&alphagram, &probability, &combinations)
		alpha := &pb.Alphagram{
			Alphagram:    alphagram,
			Probability:  probability,
			Combinations: combinations,
			Length:       int32(len(alphagram)),
		}
		alphagrams = append(alphagrams, alpha)
	}
	return alphagrams
}

func processWordRows(rows *sql.Rows) []*pb.Word {
	words := []*pb.Word{}

	for rows.Next() {
		var lexSymbols, definition, frontHooks, backHooks, alphagram, word string
		var innerFrontHook, innerBackHook bool
		rows.Scan(&lexSymbols, &definition, &frontHooks, &backHooks,
			&innerFrontHook, &innerBackHook, &alphagram, &word)
		pbWord := &pb.Word{
			LexiconSymbols: lexSymbols,
			Definition:     definition,
			FrontHooks:     frontHooks,
			BackHooks:      backHooks,
			InnerFrontHook: innerFrontHook,
			InnerBackHook:  innerBackHook,
			Alphagram:      alphagram,
			Word:           word,
		}
		words = append(words, pbWord)
	}
	return words
}