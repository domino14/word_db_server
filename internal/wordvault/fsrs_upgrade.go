package wordvault

import (
	"context"
	"math"
	"slices"
	"time"

	"connectrpc.com/connect"
	"github.com/open-spaced-repetition/go-fsrs/v4"
	"github.com/rs/zerolog/log"

	pb "github.com/domino14/word_db_server/api/rpc/wordvault"
	"github.com/domino14/word_db_server/internal/auth"
	"github.com/domino14/word_db_server/internal/stores"
	"github.com/domino14/word_db_server/internal/stores/models"
)

// memoryUpdates are the new memory states for a page of cards, in the shape
// SetCardMemoryStates takes.
type memoryUpdates struct {
	ids          []int64
	stabilities  []float64
	difficulties []float64
	lastReviews  []string
}

type upgradeKind int

const (
	upgradeUnchanged upgradeKind = iota
	upgradeRebuilt
	upgradeImported
)

// replayMemoryState computes the stability and difficulty a card would have
// had if its review history had always been scheduled by f. New cards and
// cards imported from Cardbox (which have no real history before the import)
// are left alone.
func replayMemoryState(f *fsrs.FSRS, row models.GetCardsByIDsRow) (fsrs.MemoryState, upgradeKind) {
	card := row.FsrsCard.Card
	unchanged := fsrs.MemoryState{Stability: card.Stability, Difficulty: card.Difficulty}
	if card.State == fsrs.New || card.LastReview.IsZero() {
		return unchanged, upgradeUnchanged
	}
	if slices.ContainsFunc(row.ReviewLog, func(l stores.ReviewLog) bool {
		return l.ImportLog != nil
	}) {
		return unchanged, upgradeImported
	}

	var history []fsrs.ReviewHistory
	for _, l := range row.ReviewLog {
		if l.Rating >= fsrs.Again && l.Rating <= fsrs.Easy {
			history = append(history, fsrs.ReviewHistory{Rating: l.Rating, Review: l.Review})
		}
	}
	if len(history) == 0 {
		return unchanged, upgradeUnchanged
	}
	res, err := f.Reschedule(card, history, fsrs.RescheduleOptions{
		SortReviews: true,
		FirstDue:    history[0].Review,
	})
	if err != nil || len(res.Collections) == 0 {
		log.Err(err).Str("alpha", row.Alphagram).Str("lex", row.LexiconName).
			Msg("fsrs-upgrade-replay-failed")
		return unchanged, upgradeUnchanged
	}
	last := res.Collections[len(res.Collections)-1].Card
	return fsrs.MemoryState{Stability: last.Stability, Difficulty: last.Difficulty}, upgradeRebuilt
}

// goodIntervalDays is the interval, in days, that f would give card if it
// were answered Good when it next comes up: at its due date, or now if it's
// overdue. ok is false for cards that aren't in review, whose next Good is a
// learning step rather than an interval.
func goodIntervalDays(f *fsrs.FSRS, card fsrs.Card, now time.Time) (days float64, ok bool) {
	if card.State != fsrs.Review {
		return 0, false
	}
	at := card.Due
	if at.Before(now) {
		at = now
	}
	info, err := f.Next(card, at, fsrs.Good)
	if err != nil {
		return 0, false
	}
	return math.Round(info.Card.Due.Sub(at).Hours() / 24), true
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

// Cards are upgraded a page at a time so that big accounts (the largest have
// ~95k cards) don't need all their cards and review logs in memory at once.
const fsrsUpgradePageSize = 2000

// planFsrsUpgrade computes the upgraded memory state of all of a user's
// cards, a page at a time, and passes each page's updates to apply. It doesn't
// write anything itself; apply may be nil. Due dates are kept.
func (s *Server) planFsrsUpgrade(ctx context.Context, q *models.Queries, dbid int64, now time.Time,
	apply func(memoryUpdates) error) (*pb.FsrsUpgradeSummary, error) {

	current, version, err := s.fsrsParamsAndVersion(ctx, dbid, q)
	if err != nil {
		return nil, err
	}
	if version >= FsrsVersion6 {
		return nil, invalidArgError("your cards are already on the latest FSRS version")
	}
	// Fuzz only changes due dates, which are kept; leaving it off makes the
	// interval comparison exact.
	current.EnableFuzz = false
	target := current
	applyFsrsVersion(&target, FsrsVersion6)

	decks, err := q.GetDecks(ctx, dbid)
	if err != nil {
		return nil, err
	}
	type schedulers struct{ current, target *fsrs.FSRS }
	deckSchedulers := map[int64]schedulers{0: {fsrs.NewFSRS(current), fsrs.NewFSRS(target)}}
	for _, d := range decks {
		cp, err := deckParams(d.FsrsParamsOverride, current)
		if err != nil {
			return nil, err
		}
		tp, err := deckParams(d.FsrsParamsOverride, target)
		if err != nil {
			return nil, err
		}
		deckSchedulers[d.ID] = schedulers{fsrs.NewFSRS(cp), fsrs.NewFSRS(tp)}
	}

	ids, err := q.GetCardIDs(ctx, dbid)
	if err != nil {
		return nil, err
	}

	summary := &pb.FsrsUpgradeSummary{
		CurrentVersion: int32(version),
		TargetVersion:  FsrsVersion6,
		TotalCards:     uint32(len(ids)),
	}
	week := now.Add(7 * 24 * time.Hour)
	month := now.Add(30 * 24 * time.Hour)
	var intervals, goodBefore, goodAfter []float64

	for page := range slices.Chunk(ids, fsrsUpgradePageSize) {
		rows, err := q.GetCardsByIDs(ctx, models.GetCardsByIDsParams{UserID: dbid, Ids: page})
		if err != nil {
			return nil, err
		}
		var updates memoryUpdates
		for _, row := range rows {
			due := row.NextScheduled.Time
			if !due.After(now) {
				summary.DueNowBefore++
			}
			if !due.After(week) {
				summary.DueWithinWeekBefore++
			}
			if !due.After(month) {
				summary.DueWithinMonthBefore++
			}
			card := row.FsrsCard.Card
			if card.State != fsrs.New && !card.LastReview.IsZero() {
				intervals = append(intervals, due.Sub(card.LastReview).Hours()/24)
			}

			f, ok := deckSchedulers[row.DeckID]
			if !ok {
				f = deckSchedulers[0]
			}
			memory, kind := replayMemoryState(f.target, row)
			switch kind {
			case upgradeUnchanged:
				summary.UnchangedCards++
				continue
			case upgradeImported:
				summary.ImportedCards++
				continue
			}
			summary.RescheduledCards++

			upgraded := card
			upgraded.Stability, upgraded.Difficulty = memory.Stability, memory.Difficulty
			before, ok1 := goodIntervalDays(f.current, card, now)
			after, ok2 := goodIntervalDays(f.target, upgraded, now)
			if ok1 && ok2 {
				goodBefore = append(goodBefore, before)
				goodAfter = append(goodAfter, after)
				if after < before {
					summary.GoodIntervalShorter++
				} else if after > before {
					summary.GoodIntervalLonger++
				}
			}

			if memory.Stability == card.Stability && memory.Difficulty == card.Difficulty {
				continue
			}
			updates.ids = append(updates.ids, row.ID)
			updates.stabilities = append(updates.stabilities, memory.Stability)
			updates.difficulties = append(updates.difficulties, memory.Difficulty)
			updates.lastReviews = append(updates.lastReviews, row.LastReview)
		}
		if apply != nil && len(updates.ids) > 0 {
			if err := apply(updates); err != nil {
				return nil, err
			}
		}
	}
	summary.DueNowAfter = summary.DueNowBefore
	summary.DueWithinWeekAfter = summary.DueWithinWeekBefore
	summary.DueWithinMonthAfter = summary.DueWithinMonthBefore
	summary.MedianIntervalDaysBefore = median(intervals)
	summary.MedianIntervalDaysAfter = summary.MedianIntervalDaysBefore
	summary.MedianGoodIntervalDaysBefore = median(goodBefore)
	summary.MedianGoodIntervalDaysAfter = median(goodAfter)
	return summary, nil
}

func (s *Server) PreviewFsrsUpgrade(ctx context.Context, req *connect.Request[pb.PreviewFsrsUpgradeRequest]) (
	*connect.Response[pb.PreviewFsrsUpgradeResponse], error) {
	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	summary, err := s.planFsrsUpgrade(ctx, s.Queries, int64(user.DBID), s.Nower.Now(), nil)
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

	summary, err := s.planFsrsUpgrade(ctx, qtx, int64(user.DBID), s.Nower.Now(),
		func(u memoryUpdates) error {
			_, err := qtx.SetCardMemoryStates(ctx, models.SetCardMemoryStatesParams{
				UserID:       int64(user.DBID),
				Ids:          u.ids,
				Stabilities:  u.stabilities,
				Difficulties: u.difficulties,
				LastReviews:  u.lastReviews,
			})
			return err
		})
	if err != nil {
		return nil, err
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
