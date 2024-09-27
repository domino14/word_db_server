package wordvault

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func toPGTimestamp(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Valid: true, Time: t}
}
