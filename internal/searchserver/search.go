package searchserver

import (
	"context"
	"database/sql"
	"errors"

	"github.com/rs/zerolog/log"

	"github.com/domino14/word_db_server/internal/querygen"
	pb "github.com/domino14/word_db_server/rpc/wordsearcher"
)

// Search implements the search for alphagrams/words
func (s *Server) Search(ctx context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {

	qgen, err := createQueryGen(req, MaxSQLChunkSize)
	if err != nil {
		return nil, err
	}

	db, err := s.getDbConnection(qgen.LexiconName())
	if err != nil {
		return nil, err
	}
	defer db.Close()

	queries, err := qgen.Generate()
	if err != nil {
		return nil, err
	}
	log.Debug().Msgf("Generated queries %v", queries)

	alphagrams, err := combineQueryResults(queries, db, req.Expand)
	if err != nil {
		return nil, err
	}
	return &pb.SearchResponse{
		Alphagrams: alphagrams,
		Lexicon:    qgen.LexiconName(),
	}, nil
}
func createQueryGen(req *pb.SearchRequest, maxChunkSize int) (*querygen.QueryGen, error) {
	log.Debug().Msgf("Got request %v, getsearchparams returns %v", req.String(), req.GetSearchparams())

	if req.Searchparams[0].Condition != pb.SearchRequest_LEXICON {
		return nil, errors.New("the first condition must be a lexicon")
	}
	lexName := req.Searchparams[0].GetStringvalue().GetValue()

	var queryType querygen.QueryType
	if req.Expand {
		queryType = querygen.FullExpanded
	} else {
		queryType = querygen.AlphagramsAndWords
	}
	qgen := querygen.NewQueryGen(lexName, queryType, req.Searchparams[1:], maxChunkSize)
	log.Debug().Msgf("Creating new querygen with lexicon name %v, search params %v, expand %v",
		lexName, req.Searchparams[1:], req.Expand)

	err := qgen.Validate()
	if err != nil {
		return nil, err
	}
	return qgen, nil
}

func combineQueryResults(queries []*querygen.Query, db *sql.DB, expand bool) ([]*pb.Alphagram, error) {
	alphagrams := []*pb.Alphagram{}
	// Execute the queries.
	for _, query := range queries {
		rows, err := db.Query(query.Rendered(), query.BindParams()...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		alphagrams = append(alphagrams, processQuestionRows(rows, expand)...)
	}
	return alphagrams, nil
}

func processQuestionRows(rows *sql.Rows, expanded bool) []*pb.Alphagram {
	alphagrams := []*pb.Alphagram{}
	var lastAlphagram *pb.Alphagram
	curWords := []*pb.Word{}
	for rows.Next() {
		var word, alphagram string
		var lexSymbols, definition, frontHooks, backHooks string
		var probability int32
		var combinations int64
		var innerFrontHook, innerBackHook bool
		if !expanded {
			rows.Scan(&word, &alphagram)
		} else {
			rows.Scan(&lexSymbols, &definition, &frontHooks, &backHooks,
				&innerFrontHook, &innerBackHook, &word, &alphagram, &probability,
				&combinations)
		}
		alpha := &pb.Alphagram{
			Alphagram:    alphagram,
			Probability:  probability,
			Combinations: combinations,
			Length:       int32(len(alphagram)),
			ExpandedRepr: expanded,
		}
		if lastAlphagram != nil && alpha.Alphagram != lastAlphagram.Alphagram {
			lastAlphagram.Words = curWords
			alphagrams = append(alphagrams, lastAlphagram)
			curWords = []*pb.Word{}
		}
		if !expanded {
			// Don't bother with the extra bandwidth for including the
			// alphagram for every word.
			alphagram = ""
		}
		curWords = append(curWords, &pb.Word{
			Word:      word,
			Alphagram: alphagram,
			// These fields will all have default values if not expanded.
			Definition:     definition,
			FrontHooks:     frontHooks,
			BackHooks:      backHooks,
			LexiconSymbols: lexSymbols,
			InnerFrontHook: innerFrontHook,
			InnerBackHook:  innerBackHook,
		})
		lastAlphagram = alpha
	}
	if lastAlphagram != nil {
		lastAlphagram.Words = curWords
		alphagrams = append(alphagrams, lastAlphagram)
	}
	return alphagrams
}