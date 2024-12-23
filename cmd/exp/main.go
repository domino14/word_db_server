package main

import (
	"fmt"
	"time"

	"github.com/open-spaced-repetition/go-fsrs/v3"
)

// Some experimentation code to figure out this API.
func main() {
	p := fsrs.DefaultParam()
	p.EnableShortTerm = false
	p.EnableFuzz = true
	p.MaximumInterval = 365 * 5
	card := fsrs.NewCard()
	now := time.Now()
	card.Due = now
	f := fsrs.NewFSRS(p)
	// card.Stability = 1000000
	// card.Difficulty = 1
	card.State = fsrs.New // this needs to be set if changing the stab/diff manually.
	for range 10 {
		schedulingCards := f.Repeat(card, card.Due)
		rating := fsrs.Good
		card = schedulingCards[rating].Card
		fmt.Printf("due: %s, stability: %.2f, difficulty: %.2f\n", card.Due.Format(time.RFC3339), card.Stability, card.Difficulty)
	}
}
