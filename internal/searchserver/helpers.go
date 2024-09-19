package searchserver

/* some helper functions for making word searches */
import (
	"github.com/domino14/word_db_server/rpc/api/wordsearcher"
	pb "github.com/domino14/word_db_server/rpc/api/wordsearcher"
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

func SearchDescDifficultyRange(min int, max int) *pb.SearchRequest_SearchParam {
	return &pb.SearchRequest_SearchParam{
		Condition:      pb.SearchRequest_DIFFICULTY_RANGE,
		Conditionparam: minMaxParam(min, max),
	}
}

func SearchDescProbLimit(min int, max int) *pb.SearchRequest_SearchParam {
	return &pb.SearchRequest_SearchParam{
		Condition:      pb.SearchRequest_PROBABILITY_LIMIT,
		Conditionparam: minMaxParam(min, max),
	}
}

func SearchDescPointValue(min int, max int) *pb.SearchRequest_SearchParam {
	return &pb.SearchRequest_SearchParam{
		Condition:      pb.SearchRequest_POINT_VALUE,
		Conditionparam: minMaxParam(min, max),
	}
}

func SearchDescNumAnagrams(min int, max int) *pb.SearchRequest_SearchParam {
	return &pb.SearchRequest_SearchParam{
		Condition:      pb.SearchRequest_NUMBER_OF_ANAGRAMS,
		Conditionparam: minMaxParam(min, max),
	}
}

func SearchDescAlphagramList(alphas []string) *pb.SearchRequest_SearchParam {
	return &pb.SearchRequest_SearchParam{
		Condition:      pb.SearchRequest_ALPHAGRAM_LIST,
		Conditionparam: stringArrayParam(alphas),
	}
}

func SearchDescWordList(words []string) *pb.SearchRequest_SearchParam {
	return &pb.SearchRequest_SearchParam{
		Condition:      pb.SearchRequest_WORD_LIST,
		Conditionparam: stringArrayParam(words),
	}
}

func SearchDescProbabilityList(probs []int32) *pb.SearchRequest_SearchParam {
	return &pb.SearchRequest_SearchParam{
		Condition:      pb.SearchRequest_PROBABILITY_LIST,
		Conditionparam: intArrayParam(probs),
	}
}

func SearchDescNotInLexicon(n pb.SearchRequest_NotInLexCondition) *pb.SearchRequest_SearchParam {
	return &pb.SearchRequest_SearchParam{
		Condition:      pb.SearchRequest_NOT_IN_LEXICON,
		Conditionparam: numberParam(int(n)),
	}
}

func stringArrayParam(sa []string) *pb.SearchRequest_SearchParam_Stringarray {
	return &pb.SearchRequest_SearchParam_Stringarray{
		Stringarray: &pb.SearchRequest_StringArray{
			Values: sa,
		},
	}
}

func intArrayParam(ia []int32) *pb.SearchRequest_SearchParam_Numberarray {
	return &pb.SearchRequest_SearchParam_Numberarray{
		Numberarray: &pb.SearchRequest_NumberArray{
			Values: ia,
		},
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

func numberParam(num int) *pb.SearchRequest_SearchParam_Numbervalue {
	return &pb.SearchRequest_SearchParam_Numbervalue{
		Numbervalue: &pb.SearchRequest_NumberValue{
			Value: int32(num),
		},
	}
}

func WordSearch(params []*pb.SearchRequest_SearchParam, expand bool) *pb.SearchRequest {
	return &wordsearcher.SearchRequest{
		Searchparams: params,
		Expand:       expand,
	}
}
