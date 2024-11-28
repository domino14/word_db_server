// Code generated by sqlc. DO NOT EDIT.
// versions:
//   sqlc v1.27.0

package models

import (
	"github.com/domino14/word_db_server/internal/stores"
	"github.com/jackc/pgx/v5/pgtype"
	go_fsrs "github.com/open-spaced-repetition/go-fsrs/v3"
)

type AuthUser struct {
	ID       int64
	Username pgtype.Text
}

type WordvaultCard struct {
	UserID        int64
	LexiconName   string
	Alphagram     string
	NextScheduled pgtype.Timestamptz
	FsrsCard      stores.Card
	ReviewLog     []stores.ReviewLog
	ID            int64
}

type WordvaultParam struct {
	UserID int64
	Params go_fsrs.Parameters
}
