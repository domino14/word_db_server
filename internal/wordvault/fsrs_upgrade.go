package wordvault

import (
	"context"
	"encoding/json"
	"math"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/open-spaced-repetition/go-fsrs/v4"
	"github.com/rs/zerolog/log"

	pb "github.com/domino14/word_db_server/api/rpc/wordvault"
	"github.com/domino14/word_db_server/internal/auth"
	"github.com/domino14/word_db_server/internal/stores"
	"github.com/domino14/word_db_server/internal/stores/models"
)

type lexiconCardUpdates struct {
	alphagrams     []string
	nextScheduleds []pgtype.Timestamptz
	fsrsCards      [][]byte
}

type upgradeKind int

const (
	upgradeUnchanged upgradeKind = iota
	upgradeRescheduled
	upgradeImported
)

// intervalDays is the number of days until a card with the given stability
// drops to the requested retention, rounded and clamped like go-fsrs does.
func intervalDays(p fsrs.Parameters, stability float64) float64 {
	decay := -p.W[20]
	factor := math.Pow(0.9, 1/decay) - 1
	ivl := stability / factor * (math.Pow(p.RequestRetention, 1/decay) - 1)
	return max(min(math.Round(ivl), p.MaximumInterval), 1)
}

// upgradeCard computes what a card would look like had it always been
// scheduled with the parameters in f. Cards are rebuilt by replaying their
// review history. Cards imported from Cardbox don't have a real history
// before the import, so they keep their memory state and only get a new due
// date.
func upgradeCard(f *fsrs.FSRS, row models.GetAllCardsRow, now time.Time) (fsrs.Card, upgradeKind) {
	card := row.FsrsCard.Card
	if card.State == fsrs.New || card.LastReview.IsZero() {
		return card, upgradeUnchanged
	}

	imported := slices.ContainsFunc(row.ReviewLog, func(l stores.ReviewLog) bool {
		return l.ImportLog != nil
	})
	if imported {
		if card.State != fsrs.Review {
			return card, upgradeUnchanged
		}
		ivl := intervalDays(f.Parameters, card.Stability)
		card.ScheduledDays = uint64(ivl)
		card.Due = card.LastReview.Add(time.Duration(ivl*24) * time.Hour)
		furtherFuzzDueDate(f.Parameters, now, &card)
		return card, upgradeImported
	}

	var history []fsrs.ReviewHistory
	for _, l := range row.ReviewLog {
		if l.Rating >= fsrs.Again && l.Rating <= fsrs.Easy {
			history = append(history, fsrs.ReviewHistory{Rating: l.Rating, Review: l.Review})
		}
	}
	if len(history) == 0 {
		return card, upgradeUnchanged
	}
	res, err := f.Reschedule(card, history, fsrs.RescheduleOptions{
		SortReviews: true,
		FirstDue:    history[0].Review,
		Now:         now,
	})
	if err != nil || len(res.Collections) == 0 {
		log.Err(err).Str("alpha", row.Alphagram).Str("lex", row.LexiconName).
			Msg("fsrs-upgrade-reschedule-failed")
		return card, upgradeUnchanged
	}
	card = res.Collections[len(res.Collections)-1].Card
	furtherFuzzDueDate(f.Parameters, now, &card)
	return card, upgradeRescheduled
}

func median(xs []float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	slices.Sort(xs)
	mid := len(xs) / 2
	if len(xs)%2 == 0 {
		return (xs[mid-1] + xs[mid]) / 2
	}
	return xs[mid]
}

// planFsrsUpgrade computes the upgraded state of all of a user's cards
// without writing anything.
func (s *Server) planFsrsUpgrade(ctx context.Context, q *models.Queries, dbid int64, now time.Time) (
	*pb.FsrsUpgradeSummary, map[string]*lexiconCardUpdates, error) {

	global, version, err := s.fsrsParamsAndVersion(ctx, dbid, q)
	if err != nil {
		return nil, nil, err
	}
	if version >= FsrsVersion6 {
		return nil, nil, invalidArgError("your cards are already on the latest FSRS version")
	}
	applyFsrsVersion(&global, FsrsVersion6)

	decks, err := q.GetDecks(ctx, dbid)
	if err != nil {
		return nil, nil, err
	}
	schedulers := map[int64]*fsrs.FSRS{0: fsrs.NewFSRS(global)}
	for _, d := range decks {
		p, err := deckParams(d.FsrsParamsOverride, global, FsrsVersion6)
		if err != nil {
			return nil, nil, err
		}
		schedulers[d.ID] = fsrs.NewFSRS(p)
	}

	rows, err := q.GetAllCards(ctx, dbid)
	if err != nil {
		return nil, nil, err
	}

	summary := &pb.FsrsUpgradeSummary{
		CurrentVersion: int32(version),
		TargetVersion:  FsrsVersion6,
		TotalCards:     uint32(len(rows)),
	}
	week := now.Add(7 * 24 * time.Hour)
	month := now.Add(30 * 24 * time.Hour)
	countDue := func(due time.Time, nowCt, weekCt, monthCt *uint32) {
		if !due.After(now) {
			*nowCt++
		}
		if !due.After(week) {
			*weekCt++
		}
		if !due.After(month) {
			*monthCt++
		}
	}
	var intervalsBefore, intervalsAfter []float64
	updates := map[string]*lexiconCardUpdates{}

	for _, row := range rows {
		f, ok := schedulers[row.DeckID]
		if !ok {
			f = schedulers[0]
		}
		oldCard := row.FsrsCard.Card
		oldDue := row.NextScheduled.Time
		newCard, kind := upgradeCard(f, row, now)
		newDue := oldDue
		if kind != upgradeUnchanged {
			newDue = newCard.Due
		}

		countDue(oldDue, &summary.DueNowBefore, &summary.DueWithinWeekBefore, &summary.DueWithinMonthBefore)
		countDue(newDue, &summary.DueNowAfter, &summary.DueWithinWeekAfter, &summary.DueWithinMonthAfter)

		switch kind {
		case upgradeUnchanged:
			summary.UnchangedCards++
			continue
		case upgradeRescheduled:
			summary.RescheduledCards++
		case upgradeImported:
			summary.ImportedCards++
		}

		if diff := newDue.Sub(oldDue); diff <= -24*time.Hour {
			summary.MovedEarlier++
		} else if diff >= 24*time.Hour {
			summary.MovedLater++
		}
		intervalsBefore = append(intervalsBefore, oldDue.Sub(oldCard.LastReview).Hours()/24)
		intervalsAfter = append(intervalsAfter, newDue.Sub(newCard.LastReview).Hours()/24)

		cardbts, err := json.Marshal(stores.Card{Card: newCard})
		if err != nil {
			return nil, nil, err
		}
		u, ok := updates[row.LexiconName]
		if !ok {
			u = &lexiconCardUpdates{}
			updates[row.LexiconName] = u
		}
		u.alphagrams = append(u.alphagrams, row.Alphagram)
		u.nextScheduleds = append(u.nextScheduleds, toPGTimestamp(newDue))
		u.fsrsCards = append(u.fsrsCards, cardbts)
	}
	summary.MedianIntervalDaysBefore = median(intervalsBefore)
	summary.MedianIntervalDaysAfter = median(intervalsAfter)
	return summary, updates, nil
}

func (s *Server) PreviewFsrsUpgrade(ctx context.Context, req *connect.Request[pb.PreviewFsrsUpgradeRequest]) (
	*connect.Response[pb.PreviewFsrsUpgradeResponse], error) {
	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	summary, _, err := s.planFsrsUpgrade(ctx, s.Queries, int64(user.DBID), s.Nower.Now())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.PreviewFsrsUpgradeResponse{Summary: summary}), nil
}

func (s *Server) UpgradeFsrs(ctx context.Context, req *connect.Request[pb.UpgradeFsrsRequest]) (
	*connect.Response[pb.UpgradeFsrsResponse], error) {
	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	maintenance, err := s.appMaintenance(ctx)
	if err != nil {
		return nil, err
	}
	if maintenance {
		return nil, ErrMaintenance
	}

	tx, err := s.DBPool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	summary, updates, err := s.planFsrsUpgrade(ctx, qtx, int64(user.DBID), s.Nower.Now())
	if err != nil {
		return nil, err
	}
	for lexicon, u := range updates {
		err = qtx.BulkUpdateCards(ctx, models.BulkUpdateCardsParams{
			Alphagrams:     u.alphagrams,
			NextScheduleds: u.nextScheduleds,
			FsrsCards:      u.fsrsCards,
			UserID:         int64(user.DBID),
			LexiconName:    lexicon,
		})
		if err != nil {
			return nil, err
		}
	}
	err = qtx.SetFsrsVersion(ctx, models.SetFsrsVersionParams{
		UserID:      int64(user.DBID),
		FsrsVersion: FsrsVersion6,
	})
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	log.Ctx(ctx).Info().Interface("summary", summary).Msg("fsrs-upgraded")
	return connect.NewResponse(&pb.UpgradeFsrsResponse{Summary: summary}), nil
}
