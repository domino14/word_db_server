package searchserver

import (
	"context"
	"errors"
	"log"

	"github.com/domino14/word_db_server/internal/querygen"
	pb "github.com/domino14/word_db_server/rpc/wordsearcher"
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
	qgen := querygen.NewQueryGen(lexName, shouldExpand, req.Searchparams[1:])
	log.Println("[DEBUG] Creating new querygen with lexicon name", lexName,
		"search params", req.Searchparams[1:], "expand", shouldExpand)

	err := qgen.Validate()
	if err != nil {
		return nil, err
	}

	queries, err := qgen.Generate()
	if err != nil {
		return nil, err
	}
	log.Println("[DEBUG] Generated queries", queries)

	return &pb.SearchResponse{
		Alphagrams: []*pb.Alphagram{&pb.Alphagram{
			Alphagram: "foo",
		}},
	}, nil
}

// Expand implements the "expand" rpc command, which takes in a simple
// list of alphagrams with words and returns all the needed expanded info
// (such as definitions, hooks, etc).
func (s *Server) Expand(ctx context.Context, req *pb.SearchResponse) (*pb.SearchResponse, error) {
	return nil, errors.New("not yet implemented")
}
