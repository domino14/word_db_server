package searchserver

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"

	pb "github.com/domino14/word_db_server/api/rpc/wordsearcher"
	"github.com/domino14/word_db_server/config"
)

func TestRankedAlphagrams(t *testing.T) {
	s := &Server{Config: DefaultConfig}
	ranked, err := s.RankedAlphagrams(context.Background(), "NWL18", []*pb.SearchRequest_SearchParam{
		SearchDescLength(7, 8),
		SearchDescProbRange(201, 204),
	})
	assert.Nil(t, err)
	// By probability, then length.
	assert.Equal(t, []RankedAlphagram{
		{"ADEGILO", 7, 201}, {"ADEEGORT", 8, 201},
		{"AEGILOS", 7, 202}, {"AEEGLNOT", 8, 202},
		{"AEGILOU", 7, 203}, {"DEEGIORT", 8, 203},
		{"AAEELOR", 7, 204}, {"EEGILNOR", 8, 204},
	}, ranked)
}

// The same alphagrams as Search finds, without the words.
func TestRankedAlphagramsMatchesSearch(t *testing.T) {
	s := &Server{Config: DefaultConfig}
	for _, params := range [][]*pb.SearchRequest_SearchParam{
		{SearchDescLength(7, 7), SearchDescProbRange(1, 3000), SearchDescNumAnagrams(2, 100)},
		{SearchDescLength(5, 6), SearchDescPointValue(20, 40)},
		{SearchDescLength(8, 8), SearchDescProbRange(1, 2000), {
			Condition:      pb.SearchRequest_CONTAINS_HOOKS,
			Conditionparam: &pb.SearchRequest_SearchParam_Hooksparam{Hooksparam: &pb.SearchRequest_HooksParam{HookType: pb.SearchRequest_BACK_HOOKS, Hooks: "S"}},
		}},
		{SearchDescAlphagramList([]string{"AEINRST", "ADEINRS", "NOTANALPHAGRAM"})},
	} {
		resp, err := s.Search(context.Background(), connect.NewRequest(WordSearch(
			append([]*pb.SearchRequest_SearchParam{SearchDescLexicon("NWL18")}, params...), true)))
		assert.Nil(t, err)
		want := map[string]int32{}
		for _, a := range resp.Msg.Alphagrams {
			want[a.Alphagram] = a.Probability
		}

		ranked, err := s.RankedAlphagrams(context.Background(), "NWL18", params)
		assert.Nil(t, err)
		got := map[string]int32{}
		for _, a := range ranked {
			got[a.Alphagram] = a.Probability
			assert.Equal(t, int32(len(a.Alphagram)), a.Length)
		}
		assert.NotEmpty(t, got)
		assert.Equal(t, want, got)
	}
}

func TestRankedAlphagramsErrors(t *testing.T) {
	s := &Server{Config: DefaultConfig}
	_, err := s.RankedAlphagrams(context.Background(), "NWL18", []*pb.SearchRequest_SearchParam{
		SearchDescLexicon("NWL18"), SearchDescLength(7, 7)})
	assert.NotNil(t, err)

	_, err = s.RankedAlphagrams(context.Background(), "NWL18", []*pb.SearchRequest_SearchParam{SearchDescDeleted()})
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	_, err = s.RankedAlphagrams(context.Background(), "NOSUCHLEX", []*pb.SearchRequest_SearchParam{SearchDescLength(7, 7)})
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))

	small := &Server{Config: &config.Config{DataPath: DefaultConfig.DataPath, MaxQueryResults: 100}}
	_, err = small.RankedAlphagrams(context.Background(), "NWL18", []*pb.SearchRequest_SearchParam{SearchDescLength(7, 7)})
	assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
}
