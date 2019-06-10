package searchserver

import (
	"context"
	"os"
	"testing"

	pb "github.com/domino14/word_db_server/rpc/wordsearcher"
	"github.com/stretchr/testify/assert"
)

func TestExpand(t *testing.T) {

	req := &pb.SearchResponse{
		Alphagrams: []*pb.Alphagram{
			&pb.Alphagram{Alphagram: "EILNORS", Words: []*pb.Word{
				&pb.Word{Word: "NEROLIS"},
			}},
			&pb.Alphagram{Alphagram: "AINORU?", Words: []*pb.Word{
				&pb.Word{Word: "RAINOUT"},
			}},
		},
		Lexicon: "NWL18",
	}

	s := &Server{
		LexiconPath: os.Getenv("LEXICON_PATH"),
	}
	resp, err := s.Expand(context.Background(), req)
	assert.Nil(t, err)
	assert.Equal(t, []string{
		"EILNORS", "AINORU?",
	}, alphsFromPB(resp.Alphagrams))
	assert.Equal(t, "atomic fallout occurring in precipitation [n RAINOUTS]",
		resp.Alphagrams[1].Words[0].Definition,
	)
}
