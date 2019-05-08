package querygen

import (
	"testing"

	"github.com/domino14/word_db_server/rpc/wordsearcher"
	"github.com/stretchr/testify/assert"
)

func TestWhereBetweenClause(t *testing.T) {
	c := NewWhereBetweenClause("test_table", "foo_column",
		&wordsearcher.SearchRequest_MinMax{
			Min: 175,
			Max: 234,
		})
	res, params := c.Render()
	assert.Equal(t, "test_table.foo_column BETWEEN ? and ?", res)
	assert.Equal(t, []interface{}{int32(175), int32(234)}, params)
}

func TestWhereBetweenClauseEqual(t *testing.T) {
	c := NewWhereBetweenClause("test_table", "foo_column",
		&wordsearcher.SearchRequest_MinMax{
			Min: 175,
			Max: 175,
		})
	res, params, _ := c.Render()
	assert.Equal(t, "test_table.foo_column = ?", res)
	assert.Equal(t, []interface{}{int32(175)}, params)
}

func TestWhereEqualsClause(t *testing.T) {
	c := NewWhereEqualsClause("test_table", "foo_column",
		&wordsearcher.SearchRequest_StringValue{
			Value: "dogs",
		})
	res, params, _ := c.Render()
	assert.Equal(t, "test_table.foo_column = ?", res)
	assert.Equal(t, []interface{}{"dogs"}, params)
}

func TestWhereInClause(t *testing.T) {
	sp := &wordsearcher.SearchRequest_SearchParam{
		Conditionparam: &wordsearcher.SearchRequest_SearchParam_Stringarray{
			Stringarray: &wordsearcher.SearchRequest_StringArray{
				Values: []string{"abc", "easy", "as", "123"},
			}}}

	c := NewWhereInClause("test_table", "foo_column", sp)
	res, params, _ := c.Render()
	assert.Equal(t, "test_table.foo_column IN (?,?,?,?)", res)
	assert.Equal(t, []interface{}{"abc", "easy", "as", "123"}, params)
}

func TestWhereInClauseSingleItem(t *testing.T) {
	sp := &wordsearcher.SearchRequest_SearchParam{
		Conditionparam: &wordsearcher.SearchRequest_SearchParam_Stringarray{
			Stringarray: &wordsearcher.SearchRequest_StringArray{
				Values: []string{"abc"},
			}}}

	c := NewWhereInClause("test_table", "foo_column", sp)
	res, params, _ := c.Render()
	assert.Equal(t, "test_table.foo_column = ?", res)
	assert.Equal(t, []interface{}{"abc"}, params)
}

func TestWhereInNumbers(t *testing.T) {
	// Ugh, this is ugly.
	sp := &wordsearcher.SearchRequest_SearchParam{
		Conditionparam: &wordsearcher.SearchRequest_SearchParam_Numberarray{
			Numberarray: &wordsearcher.SearchRequest_NumberArray{
				Values: []int32{35, 87, 88, 14},
			}}}

	c := NewWhereInClause("test_table", "foo_column", sp)
	res, params, _ := c.Render()
	assert.Equal(t, "test_table.foo_column IN (?,?,?,?)", res)
	assert.Equal(t, []interface{}{int32(35), int32(87), int32(88), int32(14)}, params)
}

func TestLimitOffsetClause(t *testing.T) {
	lc := NewLimitOffsetClause(&wordsearcher.SearchRequest_MinMax{
		Min: 201,
		Max: 300,
	})
	res, params, _ := lc.Render()
	assert.Equal(t, "LIMIT ? OFFSET ?", res)
	assert.Equal(t, []interface{}{int32(100), int32(200)}, params)
}
