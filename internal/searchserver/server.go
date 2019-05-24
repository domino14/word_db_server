package searchserver

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"path/filepath"

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

// Search implements the search for alphagrams/words
func (s *Server) Search(ctx context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
	log.Println("[DEBUG] Got request", req.String(), "getsearchparams returns", req.GetSearchparams())

	if req.Searchparams[0].Condition != pb.SearchRequest_LEXICON {
		return nil, errors.New("the first condition must be a lexicon")
	}
	lexName := req.Searchparams[0].GetStringvalue().GetValue()
	shouldExpand := req.Expand
	qgen := querygen.NewQueryGen(lexName, shouldExpand, req.Searchparams[1:],
		MaxSQLChunkSize)
	log.Println("[DEBUG] Creating new querygen with lexicon name", lexName,
		"search params", req.Searchparams[1:], "expand", shouldExpand)

	err := qgen.Validate()
	if err != nil {
		return nil, err
	}
	// Try to connect to the db.
	db, err := sql.Open("sqlite3", filepath.Join(LexiconDir, "db", lexName+".db"))
	if err != nil {
		return nil, err
	}
	defer db.Close()

	queries, err := qgen.Generate()
	if err != nil {
		return nil, err
	}
	log.Println("[DEBUG] Generated queries", queries)
	alphagrams := []*pb.Alphagram{}
	// Execute the queries.
	for _, query := range queries {
		rows, err := db.Query(query.Rendered(), query.BindParams()...)

		if err != nil {
			return nil, err
		}

		defer rows.Close()
		alphagrams = append(alphagrams, processQuestionRows(rows)...)

	}

	return &pb.SearchResponse{
		Alphagrams: []*pb.Alphagram{&pb.Alphagram{
			Alphagram: "foo",
		}},
	}, nil
}

func processQuestionRows(rows *sql.Rows) []*pb.Alphagram {
	for rows.Next() {

		alpha := &pb.Alphagram{}
		//rows.Scan(&alpha.Alphagram, //

	}
}

// Expand implements the "expand" rpc command, which takes in a simple
// list of alphagrams with words and returns all the needed expanded info
// (such as definitions, hooks, etc).
func (s *Server) Expand(ctx context.Context, req *pb.SearchResponse) (*pb.SearchResponse, error) {
	return nil, errors.New("not yet implemented")
}
