package searchserver

import (
	"context"
	"log"

	pb "github.com/domino14/word_db_server/rpc/wordsearcher"
)

// Server implements the WordSearcher service
type Server struct{}

// Search implements the search for alphagrams/words
func (s *Server) Search(ctx context.Context, req *pb.SearchRequest) (*pb.SearchResponse, error) {
	log.Println("Got request", req.String(), "getsearchparams returns", req.GetSearchparams())
	return &pb.SearchResponse{
		Alphagrams: []*pb.Alphagram{&pb.Alphagram{
			Alphagram: "foo",
		}},
	}, nil
}
