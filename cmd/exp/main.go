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
	card := fsrs.NewCard()
	now := time.Now()
	card.Due = now
	f := fsrs.NewFSRS(p)

	for range 10 {
		schedulingCards := f.Repeat(card, card.Due)
		rating := fsrs.Hard
		card = schedulingCards[rating].Card
		revlog := schedulingCards[rating].ReviewLog
		fmt.Println("state", revlog.State)
		fmt.Println("due", card.Due)
	}
}
