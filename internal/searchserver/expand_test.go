package searchserver

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"

	pb "github.com/domino14/word_db_server/api/rpc/wordsearcher"
)

func TestExpand(t *testing.T) {

	req := &pb.SearchResponse{
		Alphagrams: []*pb.Alphagram{
			{Alphagram: "EILNORS", Words: []*pb.Word{
				{Word: "NEROLIS"},
			}},
			{Alphagram: "AINORU?", Words: []*pb.Word{
				{Word: "RAINOUT"},
			}},
		},
		Lexicon: "NWL18",
	}

	s := &Server{
		Config: DefaultConfig,
	}
	resp, err := s.Expand(context.Background(), connect.NewRequest(req))
	assert.Nil(t, err)
	assert.Equal(t, []string{
		"EILNORS", "AINORU?",
	}, alphsFromPB(resp.Msg.Alphagrams))
	assert.Equal(t, "atomic fallout occurring in precipitation [n RAINOUTS]",
		resp.Msg.Alphagrams[1].Words[0].Definition,
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
		Config: DefaultConfig,
	}
	expandedResp, err := s.Expand(context.Background(), connect.NewRequest(resp))
	assert.Nil(t, err)
	assert.Equal(t, 3001, len(expandedResp.Msg.Alphagrams))
	assert.True(t, len(expandedResp.Msg.Alphagrams[3000].Words) > 0)
	assert.True(t, len(expandedResp.Msg.Alphagrams[3000].Words[0].Definition) > 0)
	assert.Equal(t, expandedResp.Msg.Alphagrams[3000].Words[0].Alphagram,
		expandedResp.Msg.Alphagrams[3000].Alphagram)

}
