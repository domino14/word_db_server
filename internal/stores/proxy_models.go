package stores

import (
	"time"

	"github.com/open-spaced-repetition/go-fsrs/v3"
)

type Card struct {
	fsrs.Card
}

type ReviewLog struct {
	fsrs.ReviewLog
	ImportLog *ImportLog `json:"ImportLog,omitempty"`
}

type ImportLog struct {
	ImportedDate    time.Time
	NumCorrect      int
	NumIncorrect    int
	Streak          int
	LastCorrect     time.Time
	CardboxAtImport int
}
