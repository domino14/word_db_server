package querygen

import (
	"fmt"
	"strings"

	"github.com/domino14/word_db_server/api/rpc/wordsearcher"
)

// Clause is a statement in a SQL query.
type Clause interface {
	// Render returns a string with `?` markers, and an array of items
	// to interpolate into those `?` markers.
	Render() (string, []interface{}, error)
}

func whereClauseRender(table string, column string, condition string) string {
	return fmt.Sprintf("%s.%s %s", table, column, condition)
}

// XXX Consider this file knowing nothing about protobufs. That might be ok,
// but there's also a case to be made for the way it is now. Since this
// is all in an "internal" package it's probably ok to leave it. It just
// makes this file less reusable.

// WhereBetweenClause is a "between" clause in SQL.
type WhereBetweenClause struct {
	conditionParams *wordsearcher.SearchRequest_MinMax
	table           string
	column          string
}

// NewWhereBetweenClause creates a WhereBetweenClause with a given table,
// column, and search request. The search request must be of the MinMax
// type.
func NewWhereBetweenClause(table string, column string,
	smm *wordsearcher.SearchRequest_MinMax) *WhereBetweenClause {
	return &WhereBetweenClause{
		conditionParams: smm,
		table:           table,
		column:          column,
	}
}

// Render implements the Clause.Render function basically. If only one
// parameter is passed in, the between turns into a '= ?'.
func (w *WhereBetweenClause) Render() (string, []interface{}, error) {
	var conditionTemplate string
	bindParams := make([]interface{}, 0)
	min := w.conditionParams.GetMin()
	max := w.conditionParams.GetMax()

	if min == max {
		conditionTemplate = `= ?`
		bindParams = append(bindParams, min)
	} else {
		conditionTemplate = `BETWEEN ? and ?`
		bindParams = append(bindParams, min, max)
	}
	return whereClauseRender(w.table, w.column, conditionTemplate), bindParams, nil
}

type WhereEqualsClause struct {
	conditionParams *wordsearcher.SearchRequest_StringValue
	table           string
	column          string
}

func NewWhereEqualsClause(table string, column string,
	ssv *wordsearcher.SearchRequest_StringValue) *WhereEqualsClause {
	return &WhereEqualsClause{
		conditionParams: ssv,
		table:           table,
		column:          column,
	}
}

func (w *WhereEqualsClause) Render() (string, []interface{}, error) {
	var conditionTemplate string
	bindParams := make([]interface{}, 0)
	val := w.conditionParams.GetValue()

	conditionTemplate = `= ?`
	bindParams = append(bindParams, val)

	return whereClauseRender(w.table, w.column, conditionTemplate), bindParams, nil
}

// WhereEqualsNumberClause is a special case of a WhereEqualsClause. This
// one does not use any special protobuf classes.
type WhereEqualsNumberClause struct {
	num    int
	table  string
	column string
}

func NewWhereEqualsNumberClause(table string, column string, num int) *WhereEqualsNumberClause {
	return &WhereEqualsNumberClause{
		num:    num,
		table:  table,
		column: column,
	}
}

func (w *WhereEqualsNumberClause) Render() (string, []interface{}, error) {
	var conditionTemplate string
	bindParams := []interface{}{w.num}
	conditionTemplate = `= ?`

	return whereClauseRender(w.table, w.column, conditionTemplate), bindParams, nil
}

// WhereInClause can represent a clause with a string array or a number array.
type WhereInClause struct {
	conditionParams *wordsearcher.SearchRequest_SearchParam
	table           string
	column          string
	numItems        int
}

func NewWhereInClause(table string, column string,
	sr *wordsearcher.SearchRequest_SearchParam) *WhereInClause {

	numItems := 0
	switch sr.Conditionparam.(type) {
	case *wordsearcher.SearchRequest_SearchParam_Numberarray:
		numItems = len(sr.GetNumberarray().GetValues())
	case *wordsearcher.SearchRequest_SearchParam_Stringarray:
		numItems = len(sr.GetStringarray().GetValues())
	}

	return &WhereInClause{
		conditionParams: sr,
		table:           table,
		column:          column,
		numItems:        numItems,
	}
}

func (w *WhereInClause) Render() (string, []interface{}, error) {
	var conditionTemplate string
	var bindParams []interface{}

	switch t := w.conditionParams.Conditionparam.(type) {
	// This is literally the only time I've ever thought "I wish Go had generics"

	case *wordsearcher.SearchRequest_SearchParam_Numberarray:
		numarr := w.conditionParams.GetNumberarray()
		vals := numarr.GetValues()
		bindParams = make([]interface{}, w.numItems)
		for i, v := range vals {
			bindParams[i] = v
		}

	case *wordsearcher.SearchRequest_SearchParam_Stringarray:
		strarr := w.conditionParams.GetStringarray()
		vals := strarr.GetValues()
		bindParams = make([]interface{}, w.numItems)
		for i, v := range vals {
			bindParams[i] = v
		}

	default:
		return "", nil, fmt.Errorf("error rendering, %t is not a supported type for WhereInClause",
			t)
	}

	if w.numItems == 1 {
		conditionTemplate = `= ?`
	} else {
		markers := strings.Repeat("?,", w.numItems)
		// Remove last comma:
		conditionTemplate = `IN (` + markers[:len(markers)-1] + ")"
	}

	return whereClauseRender(w.table, w.column, conditionTemplate), bindParams, nil
}

// conditionSubRange returns a contiguous subset of this clause's
// list of values and formats it as a new search param. This function
// is used to split up a possibly gigantic list of "where .. in" values.
// Note: the `max` value passed in here is NOT inclusive, but `min` is.
// this is in keeping with Go/Python/etc semantics for range.
func (w *WhereInClause) conditionSubRange(min int, max int) *wordsearcher.SearchRequest_SearchParam {
	switch w.conditionParams.Conditionparam.(type) {
	case *wordsearcher.SearchRequest_SearchParam_Numberarray:
		numarr := w.conditionParams.GetNumberarray()
		vals := numarr.GetValues()
		if max >= len(vals) {
			max = len(vals)
		}
		if min >= len(vals) {
			min = len(vals)
		}
		return &wordsearcher.SearchRequest_SearchParam{
			Conditionparam: &wordsearcher.SearchRequest_SearchParam_Numberarray{
				&wordsearcher.SearchRequest_NumberArray{
					Values: vals[min:max]}}}

	case *wordsearcher.SearchRequest_SearchParam_Stringarray:
		strarr := w.conditionParams.GetStringarray()
		vals := strarr.GetValues()
		if max >= len(vals) {
			max = len(vals)
		}
		if min >= len(vals) {
			min = len(vals)
		}
		return &wordsearcher.SearchRequest_SearchParam{
			Conditionparam: &wordsearcher.SearchRequest_SearchParam_Stringarray{
				&wordsearcher.SearchRequest_StringArray{
					Values: vals[min:max]}}}

	}
	return nil
}

// LimitOffsetClause represents a limit/offset SQL statement.
type LimitOffsetClause struct {
	conditionParams *wordsearcher.SearchRequest_MinMax
}

// NewLimitOffsetClause creates a new LimitOffsetClause
func NewLimitOffsetClause(smm *wordsearcher.SearchRequest_MinMax) *LimitOffsetClause {
	return &LimitOffsetClause{
		conditionParams: smm,
	}
}

// Render renders the limit/offset clause. Note that there is a calculation
// done here. The MinMax passed in to the NewLimitOffsetClause is assumed
// to begin counting at 1, for example, as a probability. The limit and offset
// must take this into account.
func (lc *LimitOffsetClause) Render() (string, []interface{}, error) {
	limit := lc.conditionParams.Max - lc.conditionParams.Min + 1
	offset := lc.conditionParams.Min - 1
	return "LIMIT ? OFFSET ?", []interface{}{limit, offset}, nil
}

// WhereHooksClause handles front_hooks and back_hooks searches
type WhereHooksClause struct {
	column       string
	hooks        string
	notCondition bool
}

func (w *WhereHooksClause) Render() (string, []interface{}, error) {
	var clauses []string
	var bindParams []interface{}
	
	// Special handling for empty hooks
	if w.hooks == "" {
		if w.notCondition {
			// Search for words that have some hooks (non-empty)
			return fmt.Sprintf("%s != ?", w.column), []interface{}{""}, nil
		} else {
			// Search for words that have no hooks (empty)
			return fmt.Sprintf("%s = ?", w.column), []interface{}{""}, nil
		}
	}
	
	if w.notCondition {
		// Search for words that do NOT contain any of the specified hook letters
		for _, letter := range w.hooks {
			clauses = append(clauses, fmt.Sprintf("%s NOT LIKE ?", w.column))
			bindParams = append(bindParams, fmt.Sprintf("%%%c%%", letter))
		}
		condition := "(" + strings.Join(clauses, " AND ") + ")"
		return condition, bindParams, nil
	} else {
		// Search for words that contain at least one of the specified hook letters
		for _, letter := range w.hooks {
			clauses = append(clauses, fmt.Sprintf("%s LIKE ?", w.column))
			bindParams = append(bindParams, fmt.Sprintf("%%%c%%", letter))
		}
		condition := "(" + strings.Join(clauses, " OR ") + ")"
		return condition, bindParams, nil
	}
}

// WhereInnerHooksClause handles inner_hooks searches
type WhereInnerHooksClause struct {
	hasInnerHooks bool
}

func (w *WhereInnerHooksClause) Render() (string, []interface{}, error) {
	if w.hasInnerHooks {
		return "(inner_front_hook = 1 OR inner_back_hook = 1)", []interface{}{}, nil
	} else {
		return "(inner_front_hook = 0 AND inner_back_hook = 0)", []interface{}{}, nil
	}
}

// WhereDefinitionContainsClause handles definition searches
type WhereDefinitionContainsClause struct {
	searchTerm string
}

func (w *WhereDefinitionContainsClause) Render() (string, []interface{}, error) {
	condition := "definition LIKE ? COLLATE NOCASE"
	bindParams := []interface{}{"%" + w.searchTerm + "%"}
	return condition, bindParams, nil
}

func isListClause(clause Clause) bool {
	// try to cast to a WhereIn clause.
	_, ok := clause.(*WhereInClause)
	return ok
}
