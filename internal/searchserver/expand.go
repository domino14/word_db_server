package searchserver

import (
	"context"
	"database/sql"
	"time"

	"github.com/rs/zerolog/log"

	mcconfig "github.com/domino14/macondo/config"
	"github.com/domino14/word_db_server/internal/querygen"
	pb "github.com/domino14/word_db_server/rpc/wordsearcher"
)

// Expand implements the "expand" rpc command, which takes in a simple
// list of alphagrams with words and returns all the needed expanded info
// (such as definitions, hooks, etc).
func (s *Server) Expand(ctx context.Context, req *pb.SearchResponse) (*pb.SearchResponse, error) {
	defer timeTrack(time.Now(), "expand")
	lexName := req.Lexicon
	// Get all the alphagrams from the search request.
	db, err := s.getDbConnection(lexName)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	alphStrToObjs, err := getInputAlphagramInfo(req, s.Config, db)
	if err != nil {
		return nil, err
	}

	outputAlphas, err := mergeInputWordInfo(req, s.Config, alphStrToObjs, db)
	if err != nil {
		return nil, err
	}

	return &pb.SearchResponse{
		Alphagrams: outputAlphas,
		Lexicon:    lexName,
	}, nil
}

func getInputAlphagramInfo(req *pb.SearchResponse, cfg *mcconfig.Config, db *sql.DB) (map[string]*pb.Alphagram, error) {
	inputAlphas := alphasFromSearchResponse(req)
	alphaQgen := querygen.NewQueryGen(req.Lexicon, querygen.AlphagramsOnly,
		[]*pb.SearchRequest_SearchParam{SearchDescAlphagramList(inputAlphas)},
		MaxSQLChunkSize, cfg)

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

func mergeInputWordInfo(req *pb.SearchResponse, cfg *mcconfig.Config,
	alphStrToObjs map[string]*pb.Alphagram, db *sql.DB) ([]*pb.Alphagram, error) {
	outputAlphas := []*pb.Alphagram{}

	wordToAlphagramDict := map[string]*pb.Alphagram{}
	for _, a := range req.Alphagrams {
		var thisa *pb.Alphagram
		var ok bool
		if thisa, ok = alphStrToObjs[a.Alphagram]; !ok {
			// The alphagram might not have been found in the DB if for
			// example it contained a blank.
			thisa = &pb.Alphagram{
				Alphagram: a.Alphagram,
				Length:    int32(len([]rune(a.Alphagram)))}
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
		MaxSQLChunkSize, cfg)
	queries, err := wordsQGen.Generate()

	if err != nil {
		return nil, err
	}
	log.Debug().Msgf("Generated word queries %v", queries)
	words, err := combineWordQueryResults(queries, db)
	if err != nil {
		return nil, err
	}
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
		alphagrams = append(alphagrams, processAlphagramRows(rows)...)
		rows.Close()
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
		words = append(words, processWordRows(rows)...)
		rows.Close()
	}
	return words, nil
}

func processAlphagramRows(rows *sql.Rows) []*pb.Alphagram {
	alphagrams := []*pb.Alphagram{}
	var rawBuffer []sql.RawBytes
	rawBuffer = make([]sql.RawBytes, 4)
	scanCallArgs := make([]interface{}, len(rawBuffer))
	for i := range rawBuffer {
		scanCallArgs[i] = &rawBuffer[i]
	}

	for rows.Next() {
		var alphagram string
		var probability, difficulty int32
		var combinations int64

		rows.Scan(scanCallArgs...)
		for i, col := range rawBuffer {
			switch i {
			case 0:
				alphagram = string(col)
			case 1:
				probability = toint32(col)
			case 2:
				combinations = toint64(col)
			case 3:
				difficulty = toint32(col)
			}
		}

		alpha := &pb.Alphagram{
			Alphagram:    alphagram,
			Probability:  probability,
			Combinations: combinations,
			Difficulty:   difficulty,
			Length:       int32(len([]rune(alphagram))),
		}
		alphagrams = append(alphagrams, alpha)
	}
	return alphagrams
}

func processWordRows(rows *sql.Rows) []*pb.Word {
	words := []*pb.Word{}
	var rawBuffer []sql.RawBytes
	rawBuffer = make([]sql.RawBytes, 8)
	scanCallArgs := make([]interface{}, len(rawBuffer))
	for i := range rawBuffer {
		scanCallArgs[i] = &rawBuffer[i]
	}

	for rows.Next() {
		var lexSymbols, definition, frontHooks, backHooks, alphagram, word string
		var innerFrontHook, innerBackHook bool
		rows.Scan(scanCallArgs...)
		for i, col := range rawBuffer {
			switch i {
			case 0:
				word = string(col)
			case 1:
				alphagram = string(col)
			case 2:
				lexSymbols = string(col)
			case 3:
				definition = string(col)
			case 4:
				frontHooks = string(col)
			case 5:
				backHooks = string(col)
			case 6:
				innerFrontHook = tobool(col)
			case 7:
				innerBackHook = tobool(col)
			}
		}

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
