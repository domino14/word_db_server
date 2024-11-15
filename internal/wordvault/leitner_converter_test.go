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
	card, revLog, _ := convertLeitnerToFsrs(21, 0, 21, sql.NullInt32{Int32: 1712380948, Valid: true},
		sql.NullInt32{Int32: 1721117219, Valid: true},
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
}
