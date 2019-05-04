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
	Render() (string, []interface{})
}

func whereClauseRender(table string, column string, condition string) string {
	return fmt.Sprintf("%s.%s %s", table, column, condition)
}

type WhereBetweenClause struct {
	conditionParams *wordsearcher.SearchRequest_MinMax
	table           string
	column          string
}

func NewWhereBetweenClause(table string, column string,
	smm *wordsearcher.SearchRequest_MinMax) *WhereBetweenClause {
	return &WhereBetweenClause{
		conditionParams: smm,
		table:           table,
		column:          column,
	}
}

func (w *WhereBetweenClause) Render() (string, []interface{}) {
	var conditionTemplate string
	bindParams := make([]interface{}, 0)
	min := w.conditionParams.GetMin()
	max := w.conditionParams.GetMax()

	if min == max {
		conditionTemplate = `= ?`
		bindParams = append(bindParams, min)
	} else {
		conditionTemplate = `between ? and ?`
		bindParams = append(bindParams, min, max)
	}
	return whereClauseRender(w.table, w.column, conditionTemplate), bindParams
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

func (w *WhereEqualsClause) Render() (string, []interface{}) {
	var conditionTemplate string
	bindParams := make([]interface{}, 0)
	val := w.conditionParams.GetValue()

	conditionTemplate = `= ?`
	bindParams = append(bindParams, val)

	return whereClauseRender(w.table, w.column, conditionTemplate), bindParams
}

type WhereInClause struct {
	conditionParams *wordsearcher.SearchRequest_StringArray
	table           string
	column          string
}

func NewWhereInClause(table string, column string,
	ssa *wordsearcher.SearchRequest_StringArray) *WhereInClause {
	return &WhereInClause{
		conditionParams: ssa,
		table:           table,
		column:          column,
	}
}

func (w *WhereInClause) Render() (string, []interface{}) {
	var conditionTemplate string
	var bindParams []interface{}
	vals := w.conditionParams.GetValues()
	numVals := len(vals)

	if numVals == 1 {
		conditionTemplate = `= ?`
		bindParams = make([]interface{}, 1)
		bindParams[0] = vals[0]
	} else {
		markers := strings.Repeat("?,", numVals)
		// Remove last comma:
		conditionTemplate = `in (` + markers[:len(markers)-1] + ")"
		bindParams = make([]interface{}, numVals)
		for i, v := range vals {
			bindParams[i] = v
		}
	}

	return whereClauseRender(w.table, w.column, conditionTemplate), bindParams
}
