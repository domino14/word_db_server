package main

import (
	"fmt"
	"time"

	"github.com/open-spaced-repetition/go-fsrs/v3"
)

// Some experimentation code to figure out this strange API.
func main() {

	p := fsrs.DefaultParam()
	p.EnableShortTerm = false
	p.EnableFuzz = true
	card := fsrs.NewCard()

	now := time.Now()

	f := fsrs.NewFSRS(p)

	schedulingCards := f.Repeat(card, now)
	rating := fsrs.Good

	card = schedulingCards[rating].Card
	fmt.Println("cadrd days", card.ScheduledDays)
	revlog := schedulingCards[rating].ReviewLog
	fmt.Println("revlog", revlog)
	fmt.Println("state", revlog.State)
	fmt.Println("due", card.Due)

	schedulingCards = f.Repeat(card, card.Due)
	fmt.Println("----")
	card = schedulingCards[rating].Card
	revlog = schedulingCards[rating].ReviewLog
	fmt.Println("revlog", revlog)
	fmt.Println("state", revlog.State)
	fmt.Println("due", card.Due)

	schedulingCards = f.Repeat(card, card.Due)
	fmt.Println("----")
	card = schedulingCards[rating].Card
	revlog = schedulingCards[rating].ReviewLog
	fmt.Println("revlog", revlog)
	fmt.Println("state", revlog.State)
	fmt.Println("due", card.Due)
}
