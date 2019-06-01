package querygen

import (
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/domino14/macondo/alphabet"

	"github.com/domino14/macondo/anagrammer"
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

// AlphagramOnlyQuery is used to select only alphagrams with their info
const AlphagramOnlyQuery = `
SELECT alphagram, probability, combinations FROM alphagrams
WHERE %s
`

// WordInfoQuery is used to select words with their info
const WordInfoQuery = `
SELECT lexicon_symbols, definition, front_hooks,
	back_hooks, inner_front_hook, inner_back_hook, alphagram,
	word
FROM words WHERE %s
ORDER BY word
`

// Query is a struct that encapsulates a set of bind parameters and a template.
type Query struct {
	bindParams   []interface{}
	template     string
	rendered     string
	expandedForm bool
}

func (q *Query) String() string {
	return fmt.Sprintf("<Query: %s, Params: %v>", q.rendered, q.bindParams)
}

// NewQuery creates a new query, setting the template according to the
// expand parameter.
func NewQuery(bp []interface{}, expand bool) *Query {
	if expand {
		return &Query{
			bindParams:   bp,
			template:     FullQuery,
			expandedForm: true,
		}
	}
	return &Query{
		bindParams:   bp,
		template:     UnexpandedQuery,
		expandedForm: false,
	}
}

// Rendered returns the full rendered query string.
func (q *Query) Rendered() string {
	return q.rendered
}

// BindParams returns the bound parameters needed for the query to actually
// execute.
func (q *Query) BindParams() []interface{} {
	return q.bindParams
}

// Expanded returns whether the query uses the expanded (full) or unexpanded
// query template.
func (q *Query) Expanded() bool {
	return q.expandedForm
}

func alphasFromWordList(words []string, dist *alphabet.LetterDistribution) []string {
	alphaSet := map[string]bool{}
	for _, word := range words {
		w := alphabet.Word{
			Word: word,
			Dist: dist,
		}
		alphaSet[w.MakeAlphagram()] = true
	}
	vals := []string{}
	for a := range alphaSet {
		vals = append(vals, a)
	}
	return vals
}

// Render renders a list of whereClauses and a limitOffsetClause into the
// query template.
func (q *Query) Render(whereClauses []string, limitOffsetClause string) {
	where := strings.Join(whereClauses, " AND ")
	q.rendered = fmt.Sprintf(q.template, where, limitOffsetClause)
}

// QueryGen is a query generator.
type QueryGen struct {
	lexiconName  string
	expanded     bool
	searchParams []*wordsearcher.SearchRequest_SearchParam
	maxChunkSize int
}

// NewQueryGen generates a new query generator with the given parameters.
func NewQueryGen(lexiconName string, expanded bool,
	searchParams []*wordsearcher.SearchRequest_SearchParam,
	maxChunkSize int) *QueryGen {

	return &QueryGen{lexiconName, expanded, searchParams, maxChunkSize}
}

func (qg *QueryGen) generateWhereClause(sp *wordsearcher.SearchRequest_SearchParam) (Clause, error) {
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
		desc := sp.GetNumbervalue()
		var column string
		if desc == nil {
			return nil, errors.New("numbervalue not provided for not_in_lexicon request")
		}
		if desc.GetValue() == int32(wordsearcher.SearchRequest_OTHER_ENGLISH) {
			column = "contains_word_uniq_to_lex_split"
		} else if desc.GetValue() == int32(wordsearcher.SearchRequest_PREVIOUS_VERSION) {
			column = "contains_update_to_lex"
		}
		return NewWhereEqualsNumberClause("alphagrams", column, 1), nil

	case wordsearcher.SearchRequest_MATCHING_ANAGRAM:
		desc := sp.GetStringvalue()
		if desc == nil {
			return nil, errors.New("stringvalue not provided for not_in_lexicon request")
		}
		letters := desc.GetValue()
		dawgInfo := anagrammer.Dawgs[qg.lexiconName]

		words := anagrammer.Anagram(letters, dawgInfo.GetDawg(), anagrammer.ModeExact)
		if len(words) == 0 {
			return nil, errors.New("no words matched this anagram search")
		}
		alphas := alphasFromWordList(words, dawgInfo.GetDist())
		newSp := &wordsearcher.SearchRequest_SearchParam{
			Conditionparam: &wordsearcher.SearchRequest_SearchParam_Stringarray{
				Stringarray: &wordsearcher.SearchRequest_StringArray{
					Values: alphas}}}

		return NewWhereInClause("alphagrams", "alphagram", newSp), nil

	case wordsearcher.SearchRequest_PROBABILITY_LIST:
		return NewWhereInClause("alphagrams", "probability", sp), nil

	case wordsearcher.SearchRequest_ALPHAGRAM_LIST:
		return NewWhereInClause("alphagrams", "alphagram", sp), nil

	case wordsearcher.SearchRequest_PROBABILITY_LIMIT:
		// This is handled by a limit offset clause, which is handled specially.
		// Don't do anything here.
		return nil, nil

		// HAS_TAGS can be implemented in the caller, basically, just generate
		// the list of alphagrams and use ALPHAGRAM_LIST.

	default:
		return nil, fmt.Errorf("unhandled search request condition: %v", condition)

	}
	return nil, nil
}

func isMutexCondition(condition wordsearcher.SearchRequest_Condition) bool {
	// a "list condition" is a condition that requires the query generator
	// to generate a "where ... in (?, .., ?)" query. We can't have more than
	// one of these per query otherwise it gets really complicated.
	// Note that probability_limit is not a list condition, but we return
	// true anyway because we can't combine this condition with any list conditions.
	switch condition {
	case wordsearcher.SearchRequest_PROBABILITY_LIST,
		wordsearcher.SearchRequest_ALPHAGRAM_LIST,
		wordsearcher.SearchRequest_PROBABILITY_LIMIT,
		wordsearcher.SearchRequest_MATCHING_ANAGRAM:

		return true

	}
	return false
}

// Validate returns an error if the query is invalid.
func (qg *QueryGen) Validate() error {
	numMutexDescriptions := 0
	conditionOrderProblem := false
	for idx, param := range qg.searchParams {
		if isMutexCondition(param.Condition) {
			if idx != len(qg.searchParams)-1 {
				conditionOrderProblem = true
			}
			numMutexDescriptions++
		}
	}
	if numMutexDescriptions > 1 {
		return errors.New("mutually exclusive search conditions not allowed")
	}
	if conditionOrderProblem {
		return errors.New("any condition with a list of alphagrams or " +
			"probabilities must be last in the list")
	}
	return nil
}

// Generate returns a list of *Query objects. Each query must be individually
// executed.
func (qg *QueryGen) Generate() ([]*Query, error) {
	clauses := []Clause{}
	queries := []*Query{}
	bindParams := []interface{}{}
	var loffClause Clause
	for _, param := range qg.searchParams {
		clause, err := qg.generateWhereClause(param)
		log.Println("[DEBUG] For param", param, "generated clause:", clause, err)
		if err != nil {
			return nil, err
		}
		if clause != nil {
			clauses = append(clauses, clause)
		}
		// Try to obtain limit/offset params
		if param.Condition == wordsearcher.SearchRequest_PROBABILITY_LIMIT {
			loffClause = NewLimitOffsetClause(param.GetMinmax())
		}
	}
	// Now render.
	renderedWhereClauses := []string{}
	queriesAlreadyGenerated := false
	log.Println("[DEBUG] where clauses", clauses)
	log.Println("[DEBUG] limit offset", loffClause)

	for _, clause := range clauses {
		if isListClause(clause) {
			lc := clause.(*WhereInClause)
			if lc.numItems == 0 {
				return nil, errors.New("query returns no results")
			}
			idx := 0
			for idx < lc.numItems {
				newWhereClause := NewWhereInClause(lc.table, lc.column,
					lc.conditionSubRange(idx, idx+qg.maxChunkSize))

				r, bp, err := newWhereClause.Render()
				if err != nil {
					return nil, err
				}
				newRenderedWhereClauses := append(renderedWhereClauses, r)
				query := NewQuery(append(bindParams, bp...), qg.expanded)
				query.Render(newRenderedWhereClauses, "")
				queries = append(queries, query)
				queriesAlreadyGenerated = true
				idx += qg.maxChunkSize
			}
		} else {
			r, bp, err := clause.Render()
			if err != nil {
				return nil, err
			}
			log.Println("[DEBUG] clause is not a listclause, render returns",
				r, bp)
			renderedWhereClauses = append(renderedWhereClauses, r)
			bindParams = append(bindParams, bp...)
		}
	}
	if queriesAlreadyGenerated {
		if loffClause != nil {
			return nil, errors.New("incompatible query arguments; please try " +
				"a simpler query (remove probability limit)")
		}
	} else {
		var renderedLOClause string
		var err error
		var bp []interface{}
		if loffClause != nil {
			renderedLOClause, bp, err = loffClause.Render()
			if err != nil {
				return nil, err
			}
			bindParams = append(bindParams, bp...)
		} else {
			renderedLOClause = ""
		}
		query := NewQuery(bindParams, qg.expanded)
		query.Render(renderedWhereClauses, renderedLOClause)
		queries = append(queries, query)

	}
	log.Println("[DEBUG] Returning queries")

	return queries, nil
}

func (qg *QueryGen) LexiconName() string {
	return qg.lexiconName
}
