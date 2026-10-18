package searchserver

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"

	pb "github.com/domino14/word_db_server/api/rpc/wordsearcher"
	"github.com/domino14/word_db_server/internal/querygen"
)

// RankedAlphagram is an alphagram's place in its lexicon, without its words.
// Rank is its probability or playability rank within its length, whichever
// was asked for.
type RankedAlphagram struct {
	Alphagram string
	Length    int32
	Rank      int32
}

// RankedAlphagrams runs a search in the given lexicon and returns the
// matching alphagrams with their rank in the given column, ordered by
// probability and then length. params must
// not include a lexicon condition. It's lighter than Search, which returns a
// row per word, so it suits searches that match tens of thousands of
// alphagrams.
func (s *Server) RankedAlphagrams(ctx context.Context, lexicon string,
	params []*pb.SearchRequest_SearchParam, rankBy querygen.RankColumn) ([]RankedAlphagram, error) {

	queryType := querygen.RankedAlphagrams
	for _, p := range params {
		switch p.Condition {
		case pb.SearchRequest_LEXICON:
			return nil, errors.New("lexicon must not be a search condition here")
		case pb.SearchRequest_DELETED_WORD:
			return nil, connect.NewError(connect.CodeInvalidArgument,
				errors.New("cannot search for deleted words here"))
		case pb.SearchRequest_CONTAINS_HOOKS, pb.SearchRequest_DEFINITION_CONTAINS:
			queryType = querygen.WordFilteredRankedAlphagrams
		}
	}
	qgen := querygen.NewQueryGen(lexicon, queryType, params, MaxSQLChunkSize, s.Config)
	qgen.SetRankColumn(rankBy)
	if err := qgen.Validate(); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	queries, err := qgen.Generate()
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	db, err := getDbConnection(s.Config, lexicon)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	defer db.Close()

	var out []RankedAlphagram
	for _, q := range queries {
		rows, err := db.QueryContext(ctx, q.Rendered(), q.BindParams()...)
		if err != nil {
			// A database made before version 7 has no playability column.
			if strings.Contains(err.Error(), "no such column") {
				return nil, noRankError(lexicon, rankBy)
			}
			return nil, err
		}
		for rows.Next() {
			var a RankedAlphagram
			var rank sql.NullInt32
			if err := rows.Scan(&a.Alphagram, &a.Length, &rank); err != nil {
				rows.Close()
				return nil, err
			}
			if !rank.Valid {
				rows.Close()
				return nil, noRankError(lexicon, rankBy)
			}
			a.Rank = rank.Int32
			out = append(out, a)
			if len(out) > s.Config.MaxQueryResults {
				rows.Close()
				return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf(
					"search matched more than %d alphagrams; please narrow it down", s.Config.MaxQueryResults))
			}
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

func noRankError(lexicon string, rankBy querygen.RankColumn) error {
	return connect.NewError(connect.CodeInvalidArgument,
		fmt.Errorf("%s has no %s data", lexicon, rankBy))
}
