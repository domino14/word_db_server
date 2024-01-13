package searchserver

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/domino14/word_db_server/config"
	"github.com/domino14/word_db_server/internal/querygen"
	pb "github.com/domino14/word_db_server/rpc/wordsearcher"
	"github.com/rs/zerolog/log"
)

type WordSearchServer struct {
	Config *config.Config
}

func (s *WordSearchServer) WordSearch(ctx context.Context, req *pb.WordSearchRequest) (*pb.WordSearchResponse, error) {
	// Uses a glob to search the database directly.

	db, err := getDbConnection(s.Config, req.Lexicon)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	column := ""
	switch req.AppliesTo {
	case "word":
		column = "word"
	case "definition":
		column = "definition"
	default:
		return nil, errors.New("applies_to must be only word or definition")
	}

	glob := req.Glob
	glob = strings.ReplaceAll(glob, "*", "%")
	glob = strings.ReplaceAll(glob, "?", "_")

	queryTemplate := querygen.WordInfoQuery
	where := fmt.Sprintf("%s LIKE ?", column)
	query := fmt.Sprintf(queryTemplate, where, "")
	log.Debug().Str("query", query).Str("glob", glob).Msg("word-search-query")
	rows, err := db.QueryContext(ctx, query, glob)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	words := []*pb.Word{}
	words = append(words, processWordRows(rows)...)

	return &pb.WordSearchResponse{Words: words}, nil
}

func (s *WordSearchServer) GetWordInformation(ctx context.Context, req *pb.DefineRequest) (*pb.WordSearchResponse, error) {
	db, err := getDbConnection(s.Config, req.Lexicon)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	queryTemplate := querygen.WordInfoQuery
	where := "word = ?"
	query := fmt.Sprintf(queryTemplate, where, "")
	rows, err := db.QueryContext(ctx, query, strings.ToUpper(req.Word))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	words := []*pb.Word{}
	words = append(words, processWordRows(rows)...)

	return &pb.WordSearchResponse{Words: words}, nil
}
