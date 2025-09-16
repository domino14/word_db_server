package querygen

import (
	"testing"

	"github.com/domino14/word_db_server/api/rpc/wordsearcher"
	"github.com/stretchr/testify/assert"
)

func TestWhereBetweenClause(t *testing.T) {
	c := NewWhereBetweenClause("test_table", "foo_column",
		&wordsearcher.SearchRequest_MinMax{
			Min: 175,
			Max: 234,
		})
	res, params, _ := c.Render()
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

func TestWhereHooksClause(t *testing.T) {
	// Test front hooks with normal condition
	c := &WhereHooksClause{
		column:       "front_hooks",
		hooks:        "S",
		notCondition: false,
	}
	res, params, _ := c.Render()
	assert.Equal(t, "(front_hooks LIKE ?)", res)
	assert.Equal(t, []interface{}{"%S%"}, params)

	// Test back hooks with NOT condition
	c = &WhereHooksClause{
		column:       "back_hooks",
		hooks:        "S",
		notCondition: true,
	}
	res, params, _ = c.Render()
	assert.Equal(t, "(back_hooks NOT LIKE ?)", res)
	assert.Equal(t, []interface{}{"%S%"}, params)

	// Test multiple hook letters
	c = &WhereHooksClause{
		column:       "front_hooks",
		hooks:        "ST",
		notCondition: false,
	}
	res, params, _ = c.Render()
	assert.Equal(t, "(front_hooks LIKE ? OR front_hooks LIKE ?)", res)
	assert.Equal(t, []interface{}{"%S%", "%T%"}, params)
}

func TestWhereInnerHooksClause(t *testing.T) {
	// Test has inner hooks
	c := &WhereInnerHooksClause{hasInnerHooks: true}
	res, params, _ := c.Render()
	assert.Equal(t, "(inner_front_hook = 1 OR inner_back_hook = 1)", res)
	assert.Equal(t, []interface{}{}, params)

	// Test does NOT have inner hooks
	c = &WhereInnerHooksClause{hasInnerHooks: false}
	res, params, _ = c.Render()
	assert.Equal(t, "(inner_front_hook = 0 AND inner_back_hook = 0)", res)
	assert.Equal(t, []interface{}{}, params)
}

func TestWhereDefinitionContainsClause(t *testing.T) {
	c := &WhereDefinitionContainsClause{searchTerm: "animal"}
	res, params, _ := c.Render()
	assert.Equal(t, "definition LIKE ? COLLATE NOCASE", res)
	assert.Equal(t, []interface{}{"%animal%"}, params)
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

func TestEmptyWhereClause(t *testing.T) {
	// Test that empty WHERE clauses are handled properly by adding "1=1"
	q := NewQuery([]interface{}{}, AlphagramsAndWords)
	
	// Render with empty where clauses (simulates LEXICON-only search)
	q.Render([]string{}, "")
	
	// The rendered query should contain "WHERE 1=1" instead of empty WHERE
	assert.Contains(t, q.Rendered(), "WHERE 1=1")
	assert.NotContains(t, q.Rendered(), "WHERE ORDER")
}
