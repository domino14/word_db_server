package wordvault

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/matryer/is"
	"github.com/open-spaced-repetition/go-fsrs/v4"

	pb "github.com/domino14/word_db_server/api/rpc/wordvault"
)

// v3LogEntry is a review log entry as go-fsrs v3 stored it.
type v3LogEntry struct {
	Rating        fsrs.Rating
	ScheduledDays uint64
	ElapsedDays   uint64
	Review        time.Time
	State         fsrs.State
}

// TestUpgradeFsrsLoad measures the upgrade on an account the size of the
// largest ones in prod. It's slow, so it only runs with WDB_UPGRADE_LOAD set
// to the number of cards, e.g. WDB_UPGRADE_LOAD=95000.
func TestUpgradeFsrsLoad(t *testing.T) {
	numCards, _ := strconv.Atoi(os.Getenv("WDB_UPGRADE_LOAD"))
	if numCards == 0 {
		t.Skip("set WDB_UPGRADE_LOAD to run")
	}
	is := is.New(t)
	s, fakenower, cleanup := upgradeTestServer(t, true)
	defer cleanup()
	ctx := ctxForTests()

	// Build cards with FSRS-5 histories of 1-15 reviews over the past two
	// years. Prod averages 2-3 log entries per card; this is heavier.
	params, _, err := s.fsrsParamsAndVersion(ctx, 42, nil)
	is.NoErr(err)
	f := fsrs.NewFSRS(params)
	rng := rand.New(rand.NewPCG(1, 2))
	now := fakenower.fakenow
	rows := make([][]any, 0, numCards)
	totalLog := 0
	for i := range numCards {
		t0 := now.Add(-time.Duration(rng.IntN(730*24)) * time.Hour)
		card := fsrs.NewCard(t0)
		var log []v3LogEntry
		at := t0
		for range 1 + rng.IntN(15) {
			if at.After(now) {
				break
			}
			rating := fsrs.Good
			if r := rng.IntN(10); r < 2 {
				rating = fsrs.Again
			} else if r == 9 {
				rating = fsrs.Easy
			}
			info, err := f.Next(card, at, rating)
			is.NoErr(err)
			log = append(log, v3LogEntry{rating, card.ScheduledDays, 0, at, card.State})
			card = info.Card
			at = card.Due.Add(time.Duration(rng.IntN(72)) * time.Hour)
		}
		totalLog += len(log)
		cardJSON, err := json.Marshal(card)
		is.NoErr(err)
		logJSON, err := json.Marshal(log)
		is.NoErr(err)
		rows = append(rows, []any{int64(42), "NWL23", fmt.Sprintf("A%07d", i),
			card.Due, cardJSON, logJSON})
	}
	_, err = s.DBPool.CopyFrom(ctx, pgx.Identifier{"wordvault_cards"},
		[]string{"user_id", "lexicon_name", "alphagram", "next_scheduled", "fsrs_card", "review_log"},
		pgx.CopyFromRows(rows))
	is.NoErr(err)
	rows = nil
	t.Logf("inserted %d cards, %d log entries", numCards, totalLog)

	measure := func(name string, fn func()) {
		runtime.GC()
		var before runtime.MemStats
		runtime.ReadMemStats(&before)
		var peak atomic.Uint64
		done := make(chan struct{})
		go func() {
			var m runtime.MemStats
			tick := time.NewTicker(20 * time.Millisecond)
			defer tick.Stop()
			for {
				select {
				case <-done:
					return
				case <-tick.C:
					runtime.ReadMemStats(&m)
					if m.HeapAlloc > peak.Load() {
						peak.Store(m.HeapAlloc)
					}
				}
			}
		}()
		start := time.Now()
		fn()
		elapsed := time.Since(start)
		close(done)
		var after runtime.MemStats
		runtime.ReadMemStats(&after)
		t.Logf("%s: %v, peak heap %d MB above baseline, %d MB allocated", name, elapsed,
			(int64(peak.Load())-int64(before.HeapAlloc))>>20,
			(after.TotalAlloc-before.TotalAlloc)>>20)
	}

	measure("preview", func() {
		res, err := s.PreviewFsrsUpgrade(ctx, connect.NewRequest(&pb.PreviewFsrsUpgradeRequest{}))
		is.NoErr(err)
		t.Logf("summary: %v", res.Msg.Summary)
	})
	measure("upgrade", func() {
		res, err := s.UpgradeFsrs(ctx, connect.NewRequest(&pb.UpgradeFsrsRequest{}))
		is.NoErr(err)
		is.Equal(res.Msg.Summary.RescheduledCards, uint32(numCards))
	})
}
