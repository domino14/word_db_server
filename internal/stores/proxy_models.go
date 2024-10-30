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
	ImportLog *ImportLog `json:"import_log,omitempty"`
}

type ImportLog struct {
	ImportedDate    time.Time `json:"imported_date"`
	NumCorrect      int       `json:"num_correct"`
	NumIncorrect    int       `json:"num_incorrect"`
	Streak          int       `json:"streak"`
	LastCorrect     time.Time `json:"last_correct"`
	CardboxAtImport int       `json:"cardbox_at_import"`
}
