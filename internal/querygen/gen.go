package querygen

import (
	"errors"
	"fmt"
	"strings"

	"github.com/domino14/word_db_server/rpc/wordsearcher"
)

// UnexpandedQuery just selects word and alphagram. We save bandwidth and
// speed by just selecting what we need.
const UnexpandedQuery = `
SELECT word, alphagram FROM (
	SELECT alphagrams.alphagram
	FROM alphagrams
	WHERE %s
	ORDER BY alphagrams.probability
	%s) q
INNER JOIN words w using (alphagram)
`

// FullQuery selects all the words and alphagram details
const FullQuery = `
SELECT lexicon_symbols, definition, front_hooks, back_hooks,
inner_front_hook, inner_back_hook, word, alphagram, probability,
combinations FROM (
	SELECT alphagrams.probability, alphagrams.combinations,
		alphagrams.alphagram
	FROM alphagrams
	WHERE %s
	ORDER BY alphagrams.probability
	%s) q
INNER JOIN words w using (alphagram)
`

type Query struct {
	bindParams []interface{}
	template   string
}

func NewUnexpandedQuery(bp []interface{}) *Query {
	return &Query{
		bindParams: bp,
		template:   UnexpandedQuery,
	}
}

func NewFullQuery(bp []interface{}) *Query {
	return &Query{
		bindParams: bp,
		template:   FullQuery,
	}
}

func (q *Query) Render(whereClauses []string, limitOffsetClause string) string {
	where := strings.Join(whereClauses, " AND ")
	return fmt.Sprintf(q.template, where, limitOffsetClause)
}

type QueryGen struct {
}

func generateWhereClause(sp *wordsearcher.SearchRequest_SearchParam) (Clause, error) {
	condition := sp.GetCondition()
	switch condition {
	case wordsearcher.SearchRequest_LENGTH:
		minmax := sp.GetMinmax()
		if minmax == nil {
			return nil, errors.New("minmax not provided for length request")
		}
		return NewWhereBetweenClause("alphagrams", "length", minmax), nil

	case wordsearcher.SearchRequest_NUMBER_OF_ANAGRAMS:
		minmax := sp.GetMinmax()
		if minmax == nil {
			return nil, errors.New("minmax not provided for num anagrams request")
		}
		return NewWhereBetweenClause("alphagrams", "num_anagrams", minmax), nil

	case wordsearcher.SearchRequest_PROBABILITY_RANGE:
		minmax := sp.GetMinmax()
		if minmax == nil {
			return nil, errors.New("minmax not provided for prob range request")
		}
		return NewWhereBetweenClause("alphagrams", "probability", minmax), nil

	case wordsearcher.SearchRequest_NUMBER_OF_VOWELS:
		minmax := sp.GetMinmax()
		if minmax == nil {
			return nil, errors.New("minmax not provided for num vowels request")
		}
		return NewWhereBetweenClause("alphagrams", "num_vowels", minmax), nil

	case wordsearcher.SearchRequest_POINT_VALUE:
		minmax := sp.GetMinmax()
		if minmax == nil {
			return nil, errors.New("minmax not provided for point value request")
		}
		return NewWhereBetweenClause("alphagrams", "point_value", minmax), nil

	case wordsearcher.SearchRequest_NOT_IN_LEXICON:
		desc := sp.GetStringvalue()
		var column string
		if desc == nil {
			return nil, errors.New("stringvalue not provided for not_in_lexicon request")
		}
		if desc.GetValue() == "other_english" {
			column = "contains_word_uniq_to_lex_split"
		} else if desc.GetValue() == "update" {
			column = "contains_update_to_lex"
		}
		return NewWhereEqualsNumberClause("alphagrams", column, 1), nil
	}
	// Otherwise we are here. It might be one of the list conditions.

}
