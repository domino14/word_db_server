package querygen

import (
	"errors"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"

	wglconfig "github.com/domino14/word-golib/config"
	"github.com/domino14/word-golib/kwg"
	"github.com/domino14/word-golib/tilemapping"

	"github.com/domino14/word_db_server/api/rpc/wordsearcher"
	"github.com/domino14/word_db_server/config"
	anagrammer "github.com/domino14/word_db_server/internal/anagramserver/legacyanagrammer"
	"github.com/domino14/word_db_server/internal/common"
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

// WordFilteredUnexpandedQuery finds alphagrams with matching words, then returns all words for those alphagrams
const WordFilteredUnexpandedQuery = `
SELECT w.word, w.alphagram FROM words w
WHERE w.alphagram IN (
	SELECT DISTINCT w2.alphagram FROM words w2 WHERE %s
)
ORDER BY w.alphagram
%s
`

// WordFilteredFullQuery finds alphagrams with matching words, then returns all words with full info
const WordFilteredFullQuery = `
SELECT w.word, w.alphagram, w.lexicon_symbols, w.definition, w.front_hooks, w.back_hooks,
w.inner_front_hook, w.inner_back_hook, a.probability, a.combinations, a.difficulty
FROM words w
INNER JOIN alphagrams a ON w.alphagram = a.alphagram
WHERE w.alphagram IN (
	SELECT DISTINCT w2.alphagram FROM words w2 WHERE %s
)
ORDER BY a.probability, w.alphagram
%s
`

// WordFilteredUnexpandedQueryWithAlphagrams finds alphagrams with matching words (with alphagram table access)
const WordFilteredUnexpandedQueryWithAlphagrams = `
SELECT w.word, w.alphagram FROM words w
WHERE w.alphagram IN (
	SELECT DISTINCT w2.alphagram
	FROM words w2
	INNER JOIN alphagrams a2 ON w2.alphagram = a2.alphagram
	WHERE %s
)
ORDER BY w.alphagram
%s
`

// WordFilteredFullQueryWithAlphagrams finds alphagrams with matching words (with alphagram table access)
const WordFilteredFullQueryWithAlphagrams = `
SELECT w.word, w.alphagram, w.lexicon_symbols, w.definition, w.front_hooks, w.back_hooks,
w.inner_front_hook, w.inner_back_hook, a.probability, a.combinations, a.difficulty
FROM words w
INNER JOIN alphagrams a ON w.alphagram = a.alphagram
WHERE w.alphagram IN (
	SELECT DISTINCT w2.alphagram
	FROM words w2
	INNER JOIN alphagrams a2 ON w2.alphagram = a2.alphagram
	WHERE %s
)
ORDER BY a.probability, w.alphagram
%s
`

type QueryType uint8

const (
	FullExpanded QueryType = iota
	AlphagramsOnly
	WordsOnly
	AlphagramsAndWords
	DeletedWords
	WordFilteredExpanded
	WordFilteredUnexpanded
	WordFilteredExpandedWithAlphagrams
	WordFilteredUnexpandedWithAlphagrams
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
	case WordFilteredExpanded:
		template = WordFilteredFullQuery
		expandedForm = true
	case WordFilteredUnexpanded:
		template = WordFilteredUnexpandedQuery
	case WordFilteredExpandedWithAlphagrams:
		template = WordFilteredFullQueryWithAlphagrams
		expandedForm = true
	case WordFilteredUnexpandedWithAlphagrams:
		template = WordFilteredUnexpandedQueryWithAlphagrams
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

func alphasFromWordList(words []string, dist *tilemapping.LetterDistribution) []string {
	alphaSet := map[string]bool{}
	for _, word := range words {
		w := common.InitializeWord(word, dist)
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
	if where == "" {
		// Handle empty WHERE clause for all query types
		// This can happen when only LEXICON condition is provided (which doesn't generate SQL)
		where = "1=1"
	}
	q.rendered = fmt.Sprintf(q.template, where, limitOffsetClause)
}

// QueryGen is a query generator.
type QueryGen struct {
	lexiconName  string
	queryType    QueryType
	searchParams []*wordsearcher.SearchRequest_SearchParam
	maxChunkSize int
	config       *wglconfig.Config
}

// NewQueryGen generates a new query generator with the given parameters.
// XXX: Stop allocating so much, why not re-use these?
func NewQueryGen(lexiconName string, queryType QueryType,
	searchParams []*wordsearcher.SearchRequest_SearchParam,
	maxChunkSize int, cfg *config.Config) *QueryGen {

	qgenConfig := &wglconfig.Config{
		DataPath: cfg.DataPath,
	}

	return &QueryGen{lexiconName, queryType, searchParams, maxChunkSize, qgenConfig}
}

func (qg *QueryGen) generateWhereClause(sp *wordsearcher.SearchRequest_SearchParam) (Clause, error) {
	condition := sp.GetCondition()

	// Determine the correct table alias for alphagrams based on query type
	alphagramsTable := "alphagrams"
	if qg.queryType == WordFilteredExpandedWithAlphagrams || qg.queryType == WordFilteredUnexpandedWithAlphagrams {
		alphagramsTable = "a2"
	}

	switch condition {
	case wordsearcher.SearchRequest_LENGTH:
		minmax := sp.GetMinmax()
		if minmax == nil {
			return nil, errors.New("minmax not provided for length request")
		}
		if qg.queryType != DeletedWords {
			return NewWhereBetweenClause(alphagramsTable, "length", minmax), nil
		}
		return NewWhereBetweenClause("deletedwords", "length", minmax), nil

	case wordsearcher.SearchRequest_NUMBER_OF_ANAGRAMS:
		minmax := sp.GetMinmax()
		if minmax == nil {
			return nil, errors.New("minmax not provided for num anagrams request")
		}
		return NewWhereBetweenClause(alphagramsTable, "num_anagrams", minmax), nil

	case wordsearcher.SearchRequest_PROBABILITY_RANGE:
		minmax := sp.GetMinmax()
		if minmax == nil {
			return nil, errors.New("minmax not provided for prob range request")
		}
		return NewWhereBetweenClause(alphagramsTable, "probability", minmax), nil

	case wordsearcher.SearchRequest_DIFFICULTY_RANGE:
		minmax := sp.GetMinmax()
		if minmax == nil {
			return nil, errors.New("minmax not provided for difficulty range request")
		}
		return NewWhereBetweenClause(alphagramsTable, "difficulty", minmax), nil

	case wordsearcher.SearchRequest_NUMBER_OF_VOWELS:
		minmax := sp.GetMinmax()
		if minmax == nil {
			return nil, errors.New("minmax not provided for num vowels request")
		}
		return NewWhereBetweenClause(alphagramsTable, "num_vowels", minmax), nil

	case wordsearcher.SearchRequest_POINT_VALUE:
		minmax := sp.GetMinmax()
		if minmax == nil {
			return nil, errors.New("minmax not provided for point value request")
		}
		return NewWhereBetweenClause(alphagramsTable, "point_value", minmax), nil

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
		return NewWhereEqualsNumberClause(alphagramsTable, column, 1), nil

	case wordsearcher.SearchRequest_MATCHING_ANAGRAM:
		desc := sp.GetStringvalue()
		if desc == nil {
			return nil, errors.New("stringvalue not provided for not_in_lexicon request")
		}
		letters := strings.TrimSpace(strings.ToUpper(desc.GetValue()))

		dawg, err := kwg.GetKWG(qg.config, qg.lexiconName)
		if err != nil {
			return nil, err
		}
		dist, err := tilemapping.ProbableLetterDistribution(qg.config, qg.lexiconName)
		if err != nil {
			return nil, err
		}
		alph := dawg.GetAlphabet()

		var words []string
		if strings.Contains(letters, "(") {
			// defer to the legacy anagrammer. This is a "range" query.
			words = anagrammer.Anagram(letters, dawg, anagrammer.ModeExact)
		} else {
			da := kwg.DaPool.Get().(*kwg.KWGAnagrammer)
			defer kwg.DaPool.Put(da)
			err = da.InitForString(dawg, letters)
			if err != nil {
				return nil, err
			}
			da.Anagram(dawg, func(word tilemapping.MachineWord) error {
				words = append(words, word.UserVisible(alph))
				return nil
			})
		}
		if len(words) == 0 {
			return nil, errors.New("no words matched this anagram search")
		}
		alphas := alphasFromWordList(words, dist)
		newSp := &wordsearcher.SearchRequest_SearchParam{
			Conditionparam: &wordsearcher.SearchRequest_SearchParam_Stringarray{
				Stringarray: &wordsearcher.SearchRequest_StringArray{
					Values: alphas}}}

		return NewWhereInClause(alphagramsTable, "alphagram", newSp), nil

	case wordsearcher.SearchRequest_UPLOADED_WORD_OR_ALPHAGRAM_LIST:
		words := sp.GetStringarray()
		if words == nil || len(words.Values) == 0 {
			return nil, errors.New("stringarray not provided for uploaded list request")
		}
		dist, err := tilemapping.ProbableLetterDistribution(qg.config, qg.lexiconName)
		if err != nil {
			return nil, err
		}
		alphas := alphasFromWordList(words.Values, dist)
		newSp := &wordsearcher.SearchRequest_SearchParam{
			Conditionparam: &wordsearcher.SearchRequest_SearchParam_Stringarray{
				Stringarray: &wordsearcher.SearchRequest_StringArray{
					Values: alphas}}}

		return NewWhereInClause(alphagramsTable, "alphagram", newSp), nil

	case wordsearcher.SearchRequest_PROBABILITY_LIST:
		return NewWhereInClause(alphagramsTable, "probability", sp), nil

	case wordsearcher.SearchRequest_ALPHAGRAM_LIST:
		return NewWhereInClause(alphagramsTable, "alphagram", sp), nil

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

	case wordsearcher.SearchRequest_CONTAINS_HOOKS:
		hooksParam := sp.GetHooksparam()
		if hooksParam == nil {
			return nil, errors.New("hooksparam not provided for contains hooks request")
		}
		return qg.generateHooksClause(hooksParam)

	case wordsearcher.SearchRequest_DEFINITION_CONTAINS:
		stringValue := sp.GetStringvalue()
		if stringValue == nil {
			return nil, errors.New("stringvalue not provided for definition contains request")
		}
		return qg.generateDefinitionContainsClause(stringValue.GetValue())

	default:
		return nil, fmt.Errorf("unhandled search request condition: %v", condition)

	}
}

// generateHooksClause creates a clause for searching words by hooks
func (qg *QueryGen) generateHooksClause(hooksParam *wordsearcher.SearchRequest_HooksParam) (Clause, error) {
	hookType := hooksParam.GetHookType()
	hooks := strings.TrimSpace(strings.ToUpper(hooksParam.GetHooks()))
	notCondition := hooksParam.GetNotCondition()

	switch hookType {
	case wordsearcher.SearchRequest_INNER_HOOKS:
		// For inner hooks, check the boolean fields
		if notCondition {
			return &WhereInnerHooksClause{hasInnerHooks: false}, nil
		} else {
			return &WhereInnerHooksClause{hasInnerHooks: true}, nil
		}
	case wordsearcher.SearchRequest_FRONT_HOOKS:
		return &WhereHooksClause{
			column:       "front_hooks",
			hooks:        hooks,
			notCondition: notCondition,
		}, nil
	case wordsearcher.SearchRequest_BACK_HOOKS:
		return &WhereHooksClause{
			column:       "back_hooks",
			hooks:        hooks,
			notCondition: notCondition,
		}, nil
	default:
		return nil, fmt.Errorf("unsupported hook type: %v", hookType)
	}
}

// generateDefinitionContainsClause creates a clause for searching words by definition content
func (qg *QueryGen) generateDefinitionContainsClause(searchTerm string) (Clause, error) {
	searchTerm = strings.TrimSpace(searchTerm)
	if searchTerm == "" {
		return nil, errors.New("definition search term cannot be empty")
	}

	return &WhereDefinitionContainsClause{searchTerm: searchTerm}, nil
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
