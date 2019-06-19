package searchserver

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/domino14/word_db_server/internal/querygen"
	pb "github.com/domino14/word_db_server/rpc/wordsearcher"
)

// Search implements the search for alphagrams/words
func (s *Server) Search(ctx context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
	defer timeTrack(time.Now(), "search")
	t := time.Now()
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

	alphagrams, err := combineQueryResults(queries, db, req.Expand, t)
	if err != nil {
		return nil, err
	}

	return &pb.SearchResponse{
		Alphagrams: alphagrams,
		Lexicon:    qgen.LexiconName(),
	}, nil
}

func createQueryGen(req *pb.SearchRequest, maxChunkSize int) (*querygen.QueryGen, error) {
	log.Info().Msgf("Creating query gen for request %v", req)

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

func combineQueryResults(queries []*querygen.Query, db *sql.DB, expand bool,
	t time.Time) ([]*pb.Alphagram, error) {
	alphagrams := []*pb.Alphagram{}
	// Execute the queries.
	for _, query := range queries {
		rows, err := db.Query(query.Rendered(), query.BindParams()...)
		if err != nil {
			return nil, err
		}
		log.Debug().Msgf("elapsed after query: %v", time.Since(t))
		alphagrams = append(alphagrams, processQuestionRows(rows, expand)...)
		log.Debug().Msgf("elapsed after processing rows: %v", time.Since(t))
		rows.Close()
	}
	return alphagrams, nil
}

func processQuestionRows(rows *sql.Rows, expanded bool) []*pb.Alphagram {
	alphagrams := []*pb.Alphagram{}
	var lastAlphagram *pb.Alphagram
	curWords := []*pb.Word{}
	var rawBuffer []sql.RawBytes
	var numColumns int
	if expanded {
		numColumns = 10
	} else {
		numColumns = 2
	}
	// We are using raw bytes here because scanning is slow otherwise.
	rawBuffer = make([]sql.RawBytes, numColumns)
	scanCallArgs := make([]interface{}, len(rawBuffer))
	for i := range rawBuffer {
		scanCallArgs[i] = &rawBuffer[i]
	}

	for rows.Next() {
		var word, alphagram string
		var lexSymbols, definition, frontHooks, backHooks string
		var probability int32
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
			}
		}

		alpha := &pb.Alphagram{
			Alphagram:    alphagram,
			Probability:  probability,
			Combinations: combinations,
			Length:       int32(len([]rune(alphagram))),
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
