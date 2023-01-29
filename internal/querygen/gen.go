package querygen

import (
	"errors"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"

	"github.com/domino14/macondo/alphabet"
	mcconfig "github.com/domino14/macondo/config"
	"github.com/domino14/macondo/gaddag"

	anagrammer "github.com/domino14/word_db_server/internal/anagramserver/legacyanagrammer"
	"github.com/domino14/word_db_server/internal/dawg"
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
SELECT word, alphagram, lexicon_symbols, definition, front_hooks, back_hooks,
inner_front_hook, inner_back_hook, probability,
combinations, difficulty FROM (
	SELECT alphagrams.probability, alphagrams.combinations,
		alphagrams.alphagram, alphagrams.difficulty
	FROM alphagrams
	WHERE %s
	ORDER BY alphagrams.probability
	%s) q
INNER JOIN words w using (alphagram)
`

// AlphagramOnlyQuery is used to select only alphagrams with their info
const AlphagramOnlyQuery = `
SELECT alphagram, probability, combinations, difficulty FROM alphagrams
WHERE %s
%s
`

// WordInfoQuery is used to select words with their info
const WordInfoQuery = `
SELECT word, alphagram, lexicon_symbols, definition, front_hooks,
	back_hooks, inner_front_hook, inner_back_hook
FROM words WHERE %s
%s
ORDER BY word
`

const DeletedWordQuery = `
SELECT word
FROM deletedwords WHERE %s
%s
ORDER BY word
`

type QueryType uint8

const (
	FullExpanded QueryType = iota
	AlphagramsOnly
	WordsOnly
	AlphagramsAndWords
	DeletedWords
)

// Query is a struct that encapsulates a set of bind parameters and a template.
type Query struct {
	bindParams   []interface{}
	template     string
	rendered     string
	expandedForm bool
	qtype        QueryType
}

func (q *Query) String() string {
	return fmt.Sprintf("<Query: %s, Params: %v>", q.rendered, q.bindParams)
}

// NewQuery creates a new query, setting the template according to the
// expand parameter.
func NewQuery(bp []interface{}, qt QueryType) *Query {

	var template string
	var expandedForm bool
	switch qt {
	case FullExpanded:
		template = FullQuery
		expandedForm = true
	case AlphagramsOnly:
		template = AlphagramOnlyQuery
	case AlphagramsAndWords:
		template = UnexpandedQuery
	case WordsOnly:
		template = WordInfoQuery
	case DeletedWords:
		template = DeletedWordQuery
	}

	return &Query{
		bindParams:   bp,
		template:     template,
		expandedForm: expandedForm,
		qtype:        qt,
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
	if where == "" && q.template == DeletedWordQuery {
		// This should only happen for deleted words.
		where = "1=1"
	}
	q.rendered = fmt.Sprintf(q.template, where, limitOffsetClause)
}

// QueryGen is a query generator.
type QueryGen struct {
	lexiconName   string
	queryType     QueryType
	searchParams  []*wordsearcher.SearchRequest_SearchParam
	maxChunkSize  int
	macondoConfig *mcconfig.Config
}

// NewQueryGen generates a new query generator with the given parameters.
func NewQueryGen(lexiconName string, queryType QueryType,
	searchParams []*wordsearcher.SearchRequest_SearchParam,
	maxChunkSize int, cfg *mcconfig.Config) *QueryGen {

	return &QueryGen{lexiconName, queryType, searchParams, maxChunkSize, cfg}
}

func (qg *QueryGen) generateWhereClause(sp *wordsearcher.SearchRequest_SearchParam) (Clause, error) {
	condition := sp.GetCondition()
	switch condition {
	case wordsearcher.SearchRequest_LENGTH:
		minmax := sp.GetMinmax()
		if minmax == nil {
			return nil, errors.New("minmax not provided for length request")
		}
		if qg.queryType != DeletedWords {
			return NewWhereBetweenClause("alphagrams", "length", minmax), nil
		}
		return NewWhereBetweenClause("deletedwords", "length", minmax), nil

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

	case wordsearcher.SearchRequest_DIFFICULTY_RANGE:
		minmax := sp.GetMinmax()
		if minmax == nil {
			return nil, errors.New("minmax not provided for difficulty range request")
		}
		return NewWhereBetweenClause("alphagrams", "difficulty", minmax), nil

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

		dawgInfo, err := dawg.GetDawgInfo(qg.macondoConfig, qg.lexiconName)
		if err != nil {
			return nil, err
		}
		thisdawg := dawgInfo.GetDawg()
		alph := thisdawg.GetAlphabet()

		var words []string
		if strings.Contains(letters, "[") {
			// defer to the legacy anagrammer. This is a "range" query.
			words = anagrammer.Anagram(letters, thisdawg, anagrammer.ModeExact)
		} else {
			da := dawg.DaPool.Get().(*gaddag.DawgAnagrammer)
			defer dawg.DaPool.Put(da)
			err = da.InitForString(thisdawg, letters)
			if err != nil {
				return nil, err
			}
			da.Anagram(thisdawg, func(word alphabet.MachineWord) error {
				words = append(words, word.UserVisible(alph))
				return nil
			})
		}
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
	case wordsearcher.SearchRequest_WORD_LIST:
		// NOTE: this is not meant to be used for a SearchServer Search request.
		// It will break. It is only used by the "expand" query.
		return NewWhereInClause("words", "word", sp), nil

	case wordsearcher.SearchRequest_DELETED_WORD:
		// handled elsewhere
		return nil, nil

	default:
		return nil, fmt.Errorf("unhandled search request condition: %v", condition)

	}
}

func isMutexCondition(condition wordsearcher.SearchRequest_Condition) bool {
	// a "mutex condition" is a condition that requires the query generator
	// to generate a "where ... in (?, .., ?)" query. We can't have more than
	// one of these per query otherwise it gets really complicated.
	// Note that probability_limit is not a mutex condition, but we return
	// true anyway because we can't combine this condition with any mutex conditions.
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
	deletedWordCondition := false
	lengthCondition := false
	for idx, param := range qg.searchParams {
		if isMutexCondition(param.Condition) {
			if idx != len(qg.searchParams)-1 {
				conditionOrderProblem = true
			}
			numMutexDescriptions++
		}
		if param.Condition == wordsearcher.SearchRequest_DELETED_WORD {
			deletedWordCondition = true
		}
		if param.Condition == wordsearcher.SearchRequest_LENGTH {
			lengthCondition = true
		}
	}
	if deletedWordCondition {
		// deleted_word, and at most one other condition, and it must be length
		if len(qg.searchParams) > 2 {
			return errors.New("deleted word condition cannot be combined with anything other than length")
		} else if len(qg.searchParams) == 2 {
			if !lengthCondition {
				return errors.New("you can only use deleted word conditions with length conditions")
			}
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

func (qg *QueryGen) maybeChunk(clauses []Clause) (bool, []string,
	[]interface{}, []*Query, error) {

	multipleQueriesGenerated := false
	renderedWhereClauses := []string{}
	bindParams := []interface{}{}
	queries := []*Query{}

	for _, clause := range clauses {
		if isListClause(clause) {
			lc := clause.(*WhereInClause)
			if lc.numItems == 0 {
				return false, nil, nil, nil, errors.New("query returns no results")
			}
			idx := 0
			for idx < lc.numItems {
				newWhereClause := NewWhereInClause(lc.table, lc.column,
					lc.conditionSubRange(idx, idx+qg.maxChunkSize))

				r, bp, err := newWhereClause.Render()
				if err != nil {
					return false, nil, nil, nil, err
				}
				newRenderedWhereClauses := append(renderedWhereClauses, r)
				query := NewQuery(append(bindParams, bp...), qg.queryType)
				query.Render(newRenderedWhereClauses, "")
				queries = append(queries, query)
				multipleQueriesGenerated = true
				idx += qg.maxChunkSize
			}
		} else {
			r, bp, err := clause.Render()
			if err != nil {
				return false, nil, nil, nil, err
			}
			log.Debug().Msgf("clause is not a listclause, render returns %v %v",
				r, bp)
			renderedWhereClauses = append(renderedWhereClauses, r)
			bindParams = append(bindParams, bp...)
		}
	}
	return multipleQueriesGenerated, renderedWhereClauses, bindParams, queries, nil
}

// Generate returns a list of *Query objects. Each query must be individually
// executed.
func (qg *QueryGen) Generate() ([]*Query, error) {
	clauses := []Clause{}

	var loffClause Clause
	for _, param := range qg.searchParams {
		clause, err := qg.generateWhereClause(param)
		log.Debug().Msgf("For param %v generated clause %v (err %v)", param, clause, err)
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
	log.Debug().Msgf("where clauses: %v", clauses)
	log.Debug().Msgf("limit offset: %v", loffClause)

	multipleQueriesGenerated, rwc, bindParams, queries, err := qg.maybeChunk(clauses)

	if err != nil {
		return nil, err
	}

	if multipleQueriesGenerated {
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
		log.Debug().Interface("bindParams", bindParams).Interface("rwc", rwc).Interface("renderedLOClause", renderedLOClause).
			Msg("bd")
		query := NewQuery(bindParams, qg.queryType)
		query.Render(rwc, renderedLOClause)
		queries = append(queries, query)

	}

	return queries, nil
}

func (qg *QueryGen) LexiconName() string {
	return qg.lexiconName
}

func (qg *QueryGen) Type() QueryType {
	return qg.queryType
}
