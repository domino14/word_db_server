package searchserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"connectrpc.com/connect"
	"github.com/rs/zerolog/log"

	pb "github.com/domino14/word_db_server/api/rpc/wordsearcher"
	"github.com/domino14/word_db_server/config"
	"github.com/domino14/word_db_server/internal/querygen"
)

// Search implements the search for alphagrams/words
func (s *Server) Search(ctx context.Context, req *connect.Request[pb.SearchRequest]) (
	*connect.Response[pb.SearchResponse], error) {

	defer timeTrack(time.Now(), "search")
	log.Info().Str("desc", searchReqDescription(req.Msg)).Msg("searchRequest")

	qgen, err := createQueryGen(req.Msg, s.Config, MaxSQLChunkSize)
	if err != nil {
		return nil, err
	}

	db, err := getDbConnection(s.Config, qgen.LexiconName())
	if err != nil {
		return nil, err
	}
	defer db.Close()

	queries, err := qgen.Generate()
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	log.Debug().Msgf("Generated queries %v", queries)

	alphagrams, err := combineQueryResults(queries, db, req.Msg.Expand, qgen.Type(), s.Config)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&pb.SearchResponse{
		Alphagrams: alphagrams,
		Lexicon:    qgen.LexiconName(),
	}), nil
}

func createQueryGen(req *pb.SearchRequest, cfg *config.Config, maxChunkSize int) (*querygen.QueryGen, error) {
	log.Debug().Msgf("Creating query gen for request %v", req)
	if req.Searchparams == nil || len(req.Searchparams) < 1 {
		return nil, errors.New("no search params provided")
	}
	if req.Searchparams[0].Condition != pb.SearchRequest_LEXICON {
		return nil, errors.New("the first condition must be a lexicon")
	}
	lexName := req.Searchparams[0].GetStringvalue().GetValue()

	var queryType querygen.QueryType
	needsWordFiltering := false
	needsAlphagramAccess := false
	
	// Check if any search params require word-level filtering or alphagram access
	for _, p := range req.Searchparams {
		if p.Condition == pb.SearchRequest_DELETED_WORD {
			queryType = querygen.DeletedWords
			break
		}
		if p.Condition == pb.SearchRequest_CONTAINS_HOOKS || 
		   p.Condition == pb.SearchRequest_DEFINITION_CONTAINS {
			needsWordFiltering = true
		}
		// Check if condition needs alphagram table columns
		switch p.Condition {
		case pb.SearchRequest_LENGTH,
			pb.SearchRequest_NUMBER_OF_ANAGRAMS,
			pb.SearchRequest_PROBABILITY_RANGE,
			pb.SearchRequest_DIFFICULTY_RANGE,
			pb.SearchRequest_NUMBER_OF_VOWELS,
			pb.SearchRequest_POINT_VALUE,
			pb.SearchRequest_NOT_IN_LEXICON,
			pb.SearchRequest_PROBABILITY_LIST,
			pb.SearchRequest_ALPHAGRAM_LIST:
			needsAlphagramAccess = true
		}
	}
	
	// Set query type based on filtering needs and expand parameter
	if queryType != querygen.DeletedWords {
		if needsWordFiltering {
			if needsAlphagramAccess {
				// Need both word filtering and alphagram access
				if req.Expand {
					queryType = querygen.WordFilteredExpandedWithAlphagrams
				} else {
					queryType = querygen.WordFilteredUnexpandedWithAlphagrams
				}
			} else {
				// Only need word filtering
				if req.Expand {
					queryType = querygen.WordFilteredExpanded
				} else {
					queryType = querygen.WordFilteredUnexpanded
				}
			}
		} else {
			if req.Expand {
				queryType = querygen.FullExpanded
			} else {
				queryType = querygen.AlphagramsAndWords
			}
		}
	}

	qgen := querygen.NewQueryGen(lexName, queryType, req.Searchparams[1:], maxChunkSize, cfg)
	log.Debug().Msgf("Creating new querygen with lexicon name %v, search params %v, expand %v",
		lexName, req.Searchparams[1:], req.Expand)

	err := qgen.Validate()
	if err != nil {
		return nil, err
	}
	return qgen, nil
}

func combineQueryResults(queries []*querygen.Query, db *sql.DB, expand bool, qtype querygen.QueryType, cfg *config.Config) (
	[]*pb.Alphagram, error) {

	alphagrams := []*pb.Alphagram{}
	// Execute the queries.
	for _, query := range queries {
		rows, err := db.Query(query.Rendered(), query.BindParams()...)
		if err != nil {
			return nil, err
		}
		results, err := processQuestionRows(rows, expand, qtype, cfg)
		if err != nil {
			rows.Close()
			return nil, err
		}
		alphagrams = append(alphagrams, results...)

		rows.Close()
	}

	return alphagrams, nil
}

func processQuestionRows(rows *sql.Rows, expanded bool, qtype querygen.QueryType, cfg *config.Config) ([]*pb.Alphagram, error) {
	alphagrams := []*pb.Alphagram{}
	start := time.Now()

	var lastAlphagram *pb.Alphagram
	curWords := []*pb.Word{}
	var rawBuffer []sql.RawBytes
	var numColumns int
	if expanded {
		numColumns = 11
	} else {
		numColumns = 2
	}
	// Ignore expand if we're dealing with DeletedWords.
	// DeletedWords come from a special table, have no alphagrams, definitions, etc.
	if qtype == querygen.DeletedWords {
		numColumns = 1
	}
	// We are using raw bytes here because scanning is slow otherwise.
	rawBuffer = make([]sql.RawBytes, numColumns)
	scanCallArgs := make([]interface{}, len(rawBuffer))
	for i := range rawBuffer {
		scanCallArgs[i] = &rawBuffer[i]
	}

	rowCtr := 0
	log.Info().Msgf("before rows.Next() took %s", time.Since(start))

	for rows.Next() {
		rowCtr++
		// Check if we've exceeded the maximum number of results
		if rowCtr > cfg.MaxQueryResults {
			return nil, fmt.Errorf("query exceeded maximum results limit of %d. Please refine your search criteria", cfg.MaxQueryResults)
		}
		
		var word, alphagram string
		var lexSymbols, definition, frontHooks, backHooks string
		var probability, difficulty int32
		var combinations int64
		var innerFrontHook, innerBackHook bool
		err := rows.Scan(scanCallArgs...)
		if err != nil {
			log.Error().Err(err).Msg("error while scanning")
			continue
		}
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
			case 8:
				probability = toint32(col)
			case 9:
				combinations = toint64(col)
			case 10:
				difficulty = toint32(col)
			}
		}
		if qtype == querygen.DeletedWords {
			alphagram = word
		}

		alpha := &pb.Alphagram{
			Alphagram:    alphagram,
			Probability:  probability,
			Combinations: combinations,
			Length:       int32(len([]rune(alphagram))),
			ExpandedRepr: expanded,
			Difficulty:   difficulty,
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
	log.Debug().Msgf("Scanned %v rows", rowCtr)
	return alphagrams, nil
}
