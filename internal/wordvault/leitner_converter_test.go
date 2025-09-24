package wordvault

import (
	"database/sql"
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/matryer/is"
	"github.com/open-spaced-repetition/go-fsrs/v3"
)

func TestConvertToFSRS(t *testing.T) {
	is := is.New(t)
	// Convert a card we've never missed.
	now := time.Date(2024, 11, 14, 3, 4, 5, 0, time.UTC)
	card, revLog, _ := convertLeitnerToFsrs(21, 0, 21, int64(1712380948),
		int64(1721117219),
		sql.NullInt32{Int32: 7, Valid: true},
		now)

	is.True(card.Stability > 101 && card.Stability < 102)
	fmt.Println("importLog", revLog[0].ImportLog)
	p := fsrs.DefaultParam()
	p.EnableShortTerm = false
	p.EnableFuzz = true
	p.MaximumInterval = 365 * 5

	f := fsrs.NewFSRS(p)
	schedulingCards := f.Repeat(card.Card, now)
	rating := fsrs.Again
	newCard := schedulingCards[rating].Card
	fmt.Println("nc", newCard.Stability, newCard.Due)

	is.True(newCard.Stability != math.Inf(1))

	// Convert a card that was last correct after it was due. This can happen
	// if you quiz on it outside of cardbox.
	card, revLog, _ = convertLeitnerToFsrs(2, 2, 1,
		int64(1730935552),
		int64(1730330752),
		sql.NullInt32{Int32: 1, Valid: true},
		now)
	// stability should be handwaved to a small number
	is.Equal(card.Stability, 1.0)
	schedulingCards = f.Repeat(card.Card, now)
	rating = fsrs.Good
	newCard = schedulingCards[rating].Card
	is.True(!math.IsNaN(newCard.Stability))
	fmt.Println("newCard", newCard.Stability, newCard.Due, newCard.Difficulty)

	// Test handling of scientific notation string (the bug we're fixing)
	card, _, _ = convertLeitnerToFsrs(5, 1, 4,
		"1712380948.0",
		"1.756833312857143e+09", // This is the problematic value from the issue
		sql.NullInt32{Int32: 3, Valid: true},
		now)
	is.True(card.Stability > 0) // Should parse correctly and not crash
}
