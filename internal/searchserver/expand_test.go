package searchserver

import (
	"context"
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
		Config: &DefaultConfig,
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

// Test that the chunking will work ok.
func TestExpandHuge(t *testing.T) {
	// query a few thousand words with expand false
	req := WordSearch([]*pb.SearchRequest_SearchParam{
		SearchDescLexicon("NWL18"),
		SearchDescLength(8, 8),
		SearchDescProbRange(3060, 6060),
	}, false)
	resp, err := searchHelper(req)
	assert.Nil(t, err)
	s := &Server{
		Config: &DefaultConfig,
	}
	expandedResp, err := s.Expand(context.Background(), resp)
	assert.Nil(t, err)
	assert.Equal(t, 3001, len(expandedResp.Alphagrams))
	assert.True(t, len(expandedResp.Alphagrams[3000].Words) > 0)
	assert.True(t, len(expandedResp.Alphagrams[3000].Words[0].Definition) > 0)
	assert.Equal(t, expandedResp.Alphagrams[3000].Words[0].Alphagram,
		expandedResp.Alphagrams[3000].Alphagram)

}
