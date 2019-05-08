package querygen

import (
	"fmt"
	"strings"

	"github.com/domino14/word_db_server/rpc/wordsearcher"
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
}

func NewWhereInClause(table string, column string,
	sr *wordsearcher.SearchRequest_SearchParam) *WhereInClause {
	return &WhereInClause{
		conditionParams: sr,
		table:           table,
		column:          column,
	}
}

func (w *WhereInClause) Render() (string, []interface{}, error) {
	var conditionTemplate string
	var bindParams []interface{}
	var numVals int

	switch t := w.conditionParams.Conditionparam.(type) {
	// This is literally the only time I've ever thought "I wish Go had generics"

	case *wordsearcher.SearchRequest_SearchParam_Numberarray:
		numarr := w.conditionParams.GetNumberarray()
		vals := numarr.GetValues()
		numVals = len(vals)
		bindParams = make([]interface{}, numVals)
		for i, v := range vals {
			bindParams[i] = v
		}

	case *wordsearcher.SearchRequest_SearchParam_Stringarray:
		strarr := w.conditionParams.GetStringarray()
		vals := strarr.GetValues()
		numVals = len(vals)
		bindParams = make([]interface{}, numVals)
		for i, v := range vals {
			bindParams[i] = v
		}

	default:
		return "", nil, fmt.Errorf("error rendering, %t is not a supported type for WhereInClause",
			t)
	}

	if numVals == 1 {
		conditionTemplate = `= ?`
	} else {
		markers := strings.Repeat("?,", numVals)
		// Remove last comma:
		conditionTemplate = `IN (` + markers[:len(markers)-1] + ")"
	}

	return whereClauseRender(w.table, w.column, conditionTemplate), bindParams, nil
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
