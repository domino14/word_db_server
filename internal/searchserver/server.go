package searchserver

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"path/filepath"

	// sqlite3 driver is used by this server.
	_ "github.com/mattn/go-sqlite3"

	"github.com/domino14/word_db_server/internal/querygen"
	pb "github.com/domino14/word_db_server/rpc/wordsearcher"
)

var LexiconDir = os.Getenv("LEXICON_PATH")

const (
	// MaxSQLChunkSize is how many parameters we allow to put in a SQLite
	// query. The actual limit is something around 1000 unless we
	// recompile the plugin.
	MaxSQLChunkSize = 950
)

// Server implements the WordSearcher service
type Server struct{}

func createQueryGen(req *pb.SearchRequest, maxChunkSize int) (*querygen.QueryGen, error) {
	log.Println("[DEBUG] Got request", req.String(), "getsearchparams returns", req.GetSearchparams())

	if req.Searchparams[0].Condition != pb.SearchRequest_LEXICON {
		return nil, errors.New("the first condition must be a lexicon")
	}
	lexName := req.Searchparams[0].GetStringvalue().GetValue()
	qgen := querygen.NewQueryGen(lexName, req.Expand, req.Searchparams[1:], maxChunkSize)
	log.Println("[DEBUG] Creating new querygen with lexicon name", lexName,
		"search params", req.Searchparams[1:], "expand", req.Expand)

	err := qgen.Validate()
	if err != nil {
		return nil, err
	}
	return qgen, nil
}

func getDbConnection(lexName string) (*sql.DB, error) {
	// Try to connect to the db.
	return sql.Open("sqlite3", filepath.Join(LexiconDir, "db", lexName+".db"))
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

// Search implements the search for alphagrams/words
func (s *Server) Search(ctx context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {

	qgen, err := createQueryGen(req, MaxSQLChunkSize)
	if err != nil {
		return nil, err
	}

	db, err := getDbConnection(qgen.LexiconName())
	if err != nil {
		return nil, err
	}
	defer db.Close()

	queries, err := qgen.Generate()
	if err != nil {
		return nil, err
	}
	log.Println("[DEBUG] Generated queries", queries)

	alphagrams, err := combineQueryResults(queries, db, req.Expand)
	if err != nil {
		return nil, err
	}
	return &pb.SearchResponse{
		Alphagrams: alphagrams,
		Lexicon:    qgen.LexiconName(),
	}, nil
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

// Expand implements the "expand" rpc command, which takes in a simple
// list of alphagrams with words and returns all the needed expanded info
// (such as definitions, hooks, etc).
func (s *Server) Expand(ctx context.Context, req *pb.SearchResponse) (*pb.SearchResponse, error) {
	// qgen, err := createQueryGen(req, MaxSQLChunkSize)
	// if err != nil {
	// 	return nil, err
	// }

	lexName := req.Lexicon

	qgen := querygen.NewQueryGen(lexName, true, , MaxSQLChunkSize)


	db, err := getDbConnection(qgen.LexiconName())
	if err != nil {
		return nil, err
	}
	defer db.Close()
	// this func shuld be like get_questions_from_alph_dicts in webolith.
}
