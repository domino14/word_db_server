package searchserver

/* some helper functions for making word searches */
import (
	"github.com/domino14/word_db_server/rpc/wordsearcher"
	pb "github.com/domino14/word_db_server/rpc/wordsearcher"
)

type searchDescription struct{}

func SearchDescLexicon(lexicon string) *pb.SearchRequest_SearchParam {
	return &pb.SearchRequest_SearchParam{
		Condition:      pb.SearchRequest_LEXICON,
		Conditionparam: stringParam(lexicon),
	}
}

func SearchDescLength(min int, max int) *pb.SearchRequest_SearchParam {
	return &pb.SearchRequest_SearchParam{
		Condition:      pb.SearchRequest_LENGTH,
		Conditionparam: minMaxParam(min, max),
	}
}

func SearchDescProbRange(min int, max int) *pb.SearchRequest_SearchParam {
	return &pb.SearchRequest_SearchParam{
		Condition:      pb.SearchRequest_PROBABILITY_RANGE,
		Conditionparam: minMaxParam(min, max),
	}
}

func stringParam(str string) *pb.SearchRequest_SearchParam_Stringvalue {
	return &pb.SearchRequest_SearchParam_Stringvalue{
		Stringvalue: &pb.SearchRequest_StringValue{
			Value: str,
		},
	}
}

func minMaxParam(min int, max int) *pb.SearchRequest_SearchParam_Minmax {
	return &pb.SearchRequest_SearchParam_Minmax{
		Minmax: &pb.SearchRequest_MinMax{
			Min: int32(min),
			Max: int32(max),
		},
	}
}

func WordSearch(params []*pb.SearchRequest_SearchParam) *pb.SearchRequest {
	return &wordsearcher.SearchRequest{
		Searchparams: params,
	}
}
