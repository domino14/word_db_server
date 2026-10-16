package wordvault

import (
	"encoding/json"
	"math"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/matryer/is"
	"github.com/open-spaced-repetition/go-fsrs/v4"

	pb "github.com/domino14/word_db_server/api/rpc/wordvault"
	"github.com/domino14/word_db_server/internal/searchserver"
	"github.com/domino14/word_db_server/internal/stores"
	"github.com/domino14/word_db_server/internal/stores/models"
)

// What migration 000008 gives existing users who had no params row.
const v5DefaultParams = `{"RequestRetention": 0.9, "MaximumInterval": 1825, "EnableShortTerm": false, "EnableFuzz": true}`

// upgradeTestServer sets up a server for user 42. If v5User is true, the user
// is an existing (FSRS-5) user; otherwise they're a new user with no params.
func upgradeTestServer(t *testing.T, v5User bool) (*Server, *FakeNower, func()) {
	is := is.New(t)
	err := RecreateTestDB()
	if err != nil {
		panic(err)
	}
	ctx := ctxForTests()
	dbPool, err := pgxpool.New(ctx, testDBURI(true))
	is.NoErr(err)

	s := NewServer(DefaultConfig, dbPool, models.New(dbPool), &searchserver.Server{Config: DefaultConfig})
	fakenower := &FakeNower{}
	fakenower.fakenow, _ = time.Parse(time.RFC3339, "2024-09-22T23:00:00Z")
	s.Nower = fakenower

	if v5User {
		_, err = dbPool.Exec(ctx, `INSERT INTO wordvault_params (user_id, params, fsrs_version)
			VALUES (42, $1, 5)`, v5DefaultParams)
		is.NoErr(err)
	}
	return s, fakenower, dbPool.Close
}

func scoreCard(t *testing.T, s *Server, alphagram string, score pb.Score) time.Time {
	is := is.New(t)
	res, err := s.ScoreCard(ctxForTests(), connect.NewRequest(&pb.ScoreCardRequest{
		Score:     score,
		Lexicon:   "NWL23",
		Alphagram: alphagram,
	}))
	is.NoErr(err)
	return res.Msg.NextScheduled.AsTime()
}

func fsrsVersion(t *testing.T, s *Server) int32 {
	is := is.New(t)
	res, err := s.GetFsrsParameters(ctxForTests(), connect.NewRequest(&pb.GetFsrsParametersRequest{}))
	is.NoErr(err)
	return res.Msg.FsrsVersion
}

func TestFsrsVersionPicksWeights(t *testing.T) {
	is := is.New(t)
	for _, tc := range []struct {
		v5User    bool
		version   int32
		stability float64
	}{
		{v5User: true, version: FsrsVersion5, stability: fsrs5Weights[2]},
		{v5User: false, version: FsrsVersion6, stability: fsrs.DefaultWeights()[2]},
	} {
		s, _, cleanup := upgradeTestServer(t, tc.v5User)
		_, err := s.AddCards(ctxForTests(), connect.NewRequest(&pb.AddCardsRequest{
			Lexicon:    "NWL23",
			Alphagrams: []string{"ADEEGMMO"},
		}))
		is.NoErr(err)
		is.Equal(fsrsVersion(t, s), tc.version)

		// Saving settings must not change the version.
		_, err = s.EditFsrsParameters(ctxForTests(), connect.NewRequest(&pb.EditFsrsParametersRequest{
			Parameters: &pb.FsrsParameters{
				RequestRetention: 0.9,
				Scheduler:        pb.FsrsScheduler_FSRS_SCHEDULER_LONG_TERM,
			},
		}))
		is.NoErr(err)
		is.Equal(fsrsVersion(t, s), tc.version)

		// A new card's first stability is the version's weight for Good.
		scoreCard(t, s, "ADEEGMMO", pb.Score_SCORE_GOOD)
		is.Equal(getCard(t, s, "ADEEGMMO").Stability, tc.stability)
		cleanup()
	}
}

type review struct {
	at     time.Time
	rating fsrs.Rating
}

// reviewHistory scores alphagram with each rating in turn, each an hour after
// the card comes due, starting at start. It returns the reviews.
func reviewHistory(t *testing.T, s *Server, fakenower *FakeNower, alphagram string,
	start time.Time, ratings ...fsrs.Rating) []review {

	fakenower.fakenow = start
	var history []review
	for _, rating := range ratings {
		history = append(history, review{fakenower.fakenow, rating})
		next := scoreCard(t, s, alphagram, pb.Score(rating))
		fakenower.fakenow = next.Add(time.Hour)
	}
	return history
}

// testParams are a test user's FSRS parameters for an FSRS version, built
// independently of the server's code, without fuzz.
func testParams(t *testing.T, version int16, retention float64) fsrs.Parameters {
	is := is.New(t)
	p := fsrs.DefaultParam()
	p.W = fsrs.DefaultWeights()
	if version == FsrsVersion5 {
		w, err := fsrs.ConvertV5Weights(fsrs5Weights)
		is.NoErr(err)
		p.W = w
	}
	p.RequestRetention = retention
	p.MaximumInterval = 1825
	p.EnableShortTerm = false
	p.EnableFuzz = false
	return p
}

// replay is the card the history would have produced under p.
func replay(t *testing.T, p fsrs.Parameters, history []review) fsrs.Card {
	is := is.New(t)
	f := fsrs.NewFSRS(p)
	card := fsrs.NewCard(history[0].at)
	for _, r := range history {
		info, err := f.Next(card, r.at, r.rating)
		is.NoErr(err)
		card = info.Card
	}
	return card
}

// goodInterval is the interval in days that p gives card if it's answered
// Good at its due date, or at now if it's overdue.
func goodInterval(t *testing.T, p fsrs.Parameters, card fsrs.Card, now time.Time) float64 {
	is := is.New(t)
	at := card.Due
	if at.Before(now) {
		at = now
	}
	info, err := fsrs.NewFSRS(p).Next(card, at, fsrs.Good)
	is.NoErr(err)
	return math.Round(info.Card.Due.Sub(at).Hours() / 24)
}

func cardRows(t *testing.T, s *Server, alphagrams []string) map[string]models.GetCardsRow {
	is := is.New(t)
	rows, err := s.Queries.GetCards(ctxForTests(), models.GetCardsParams{
		UserID:      42,
		LexiconName: "NWL23",
		Alphagrams:  alphagrams,
	})
	is.NoErr(err)
	is.Equal(len(rows), len(alphagrams))
	byAlpha := map[string]models.GetCardsRow{}
	for _, r := range rows {
		byAlpha[r.Alphagram] = r
	}
	return byAlpha
}

func TestUpgradeFsrs(t *testing.T) {
	is := is.New(t)
	s, fakenower, cleanup := upgradeTestServer(t, true)
	defer cleanup()
	ctx := ctxForTests()

	deck, err := s.AddDeck(ctx, connect.NewRequest(&pb.AddDeckRequest{Name: "Low retention", Lexicon: "NWL23"}))
	is.NoErr(err)
	deckID := deck.Msg.Deck.Id
	_, err = s.EditDeck(ctx, connect.NewRequest(&pb.EditDeckRequest{
		Id:   deckID,
		Name: "Low retention",
		FsrsParametersOverride: &pb.FsrsParameters{
			Scheduler:        pb.FsrsScheduler_FSRS_SCHEDULER_LONG_TERM,
			RequestRetention: 0.8,
		},
	}))
	is.NoErr(err)
	_, err = s.AddCards(ctx, connect.NewRequest(&pb.AddCardsRequest{
		Lexicon:    "NWL23",
		Alphagrams: []string{"ADEEGMMO", "AEILNOR", "ADEEHMMO", "AEGLPSU"},
	}))
	is.NoErr(err)
	_, err = s.AddCards(ctx, connect.NewRequest(&pb.AddCardsRequest{
		Lexicon:    "NWL23",
		Alphagrams: []string{"AEINRST"},
		DeckId:     uint64(deckID),
	}))
	is.NoErr(err)

	// Review histories under FSRS-5. From the simulations, a card that was
	// always known gets longer intervals under FSRS-6 and one that was
	// missed gets shorter ones.
	start := fakenower.fakenow
	histories := map[string][]review{
		"ADEEGMMO": reviewHistory(t, s, fakenower, "ADEEGMMO", start,
			fsrs.Good, fsrs.Good, fsrs.Good, fsrs.Good),
		"AEILNOR": reviewHistory(t, s, fakenower, "AEILNOR", start,
			fsrs.Good, fsrs.Good, fsrs.Again, fsrs.Good, fsrs.Good),
		"AEINRST": reviewHistory(t, s, fakenower, "AEINRST", start,
			fsrs.Good, fsrs.Again, fsrs.Good, fsrs.Hard),
	}
	retention := map[string]float64{"ADEEGMMO": 0.9, "AEILNOR": 0.9, "AEINRST": 0.8}

	// AEGLPSU was imported from Cardbox, then never reviewed.
	importedAt := start.Add(-10 * 24 * time.Hour)
	imported := stores.Card{Card: fsrs.Card{
		Due:        importedAt.Add(20 * 24 * time.Hour),
		Stability:  30,
		Difficulty: 5,
		Reps:       6,
		State:      fsrs.Review,
		LastReview: importedAt,
	}}
	importLog, err := json.Marshal(stores.ReviewLog{
		ReviewLog: fsrs.ReviewLog{Review: importedAt},
		ImportLog: &stores.ImportLog{ImportedDate: importedAt, NumCorrect: 6, CardboxAtImport: 4},
	})
	is.NoErr(err)
	is.NoErr(s.Queries.UpdateCard(ctx, models.UpdateCardParams{
		FsrsCard:      imported,
		NextScheduled: toPGTimestamp(imported.Due),
		UserID:        42,
		LexiconName:   "NWL23",
		Alphagram:     "AEGLPSU",
		ReviewLogItem: importLog,
	}))
	// ADEEHMMO stays new.

	// Upgrade two months after the first reviews: some cards are overdue and
	// some aren't.
	now := start.Add(60 * 24 * time.Hour)
	fakenower.fakenow = now
	alphagrams := []string{"ADEEGMMO", "AEILNOR", "AEINRST", "ADEEHMMO", "AEGLPSU"}
	before := cardRows(t, s, alphagrams)

	// Work out what the upgrade should do, independently of its code.
	var dueNow, dueWeek, dueMonth, shorter, longer uint32
	var goodBefore, goodAfter []float64
	expected := map[string]fsrs.Card{}
	for _, a := range alphagrams {
		row := before[a]
		due := row.NextScheduled.Time
		is.True(due.Equal(row.FsrsCard.Due))
		if !due.After(now) {
			dueNow++
		}
		if !due.After(now.Add(7 * 24 * time.Hour)) {
			dueWeek++
		}
		if !due.After(now.Add(30 * 24 * time.Hour)) {
			dueMonth++
		}
		history, ok := histories[a]
		if !ok {
			expected[a] = row.FsrsCard.Card
			continue
		}
		v6 := replay(t, testParams(t, FsrsVersion6, retention[a]), history)
		card := row.FsrsCard.Card
		card.Stability, card.Difficulty = v6.Stability, v6.Difficulty
		expected[a] = card

		b := goodInterval(t, testParams(t, FsrsVersion5, retention[a]), row.FsrsCard.Card, now)
		af := goodInterval(t, testParams(t, FsrsVersion6, retention[a]), card, now)
		goodBefore = append(goodBefore, b)
		goodAfter = append(goodAfter, af)
		if af < b {
			shorter++
		} else if af > b {
			longer++
		}
	}
	// The test covers both directions and overdue cards.
	is.True(shorter >= 1)
	is.True(longer >= 1)
	is.True(dueNow >= 1)
	is.True(dueMonth < 5)

	checkSummary := func(sum *pb.FsrsUpgradeSummary) {
		is.Equal(sum.CurrentVersion, int32(FsrsVersion5))
		is.Equal(sum.TargetVersion, int32(FsrsVersion6))
		is.Equal(sum.TotalCards, uint32(5))
		is.Equal(sum.RescheduledCards, uint32(3))
		is.Equal(sum.ImportedCards, uint32(1))
		is.Equal(sum.UnchangedCards, uint32(1))
		// Due dates don't change.
		is.Equal(sum.DueNowBefore, dueNow)
		is.Equal(sum.DueNowAfter, dueNow)
		is.Equal(sum.DueWithinWeekBefore, dueWeek)
		is.Equal(sum.DueWithinWeekAfter, dueWeek)
		is.Equal(sum.DueWithinMonthBefore, dueMonth)
		is.Equal(sum.DueWithinMonthAfter, dueMonth)
		is.Equal(sum.MovedEarlier, uint32(0))
		is.Equal(sum.MovedLater, uint32(0))
		is.Equal(sum.MedianIntervalDaysBefore, sum.MedianIntervalDaysAfter)
		// Intervals drift.
		is.Equal(sum.GoodIntervalShorter, shorter)
		is.Equal(sum.GoodIntervalLonger, longer)
		is.Equal(sum.MedianGoodIntervalDaysBefore, median(goodBefore))
		is.Equal(sum.MedianGoodIntervalDaysAfter, median(goodAfter))
	}

	is.Equal(fsrsVersion(t, s), int32(FsrsVersion5))
	preview, err := s.PreviewFsrsUpgrade(ctx, connect.NewRequest(&pb.PreviewFsrsUpgradeRequest{}))
	is.NoErr(err)
	checkSummary(preview.Msg.Summary)
	// The preview doesn't change anything.
	is.Equal(fsrsVersion(t, s), int32(FsrsVersion5))
	is.Equal(cardRows(t, s, alphagrams), before)

	upgraded, err := s.UpgradeFsrs(ctx, connect.NewRequest(&pb.UpgradeFsrsRequest{}))
	is.NoErr(err)
	checkSummary(upgraded.Msg.Summary)
	is.Equal(fsrsVersion(t, s), int32(FsrsVersion6))

	// Each card's memory state is what FSRS-6 would have given it all along;
	// everything else, including the due date and review log, is unchanged.
	after := cardRows(t, s, alphagrams)
	for _, a := range alphagrams {
		is.Equal(after[a].FsrsCard.Card, expected[a])
		is.True(after[a].NextScheduled.Time.Equal(before[a].NextScheduled.Time))
		is.Equal(after[a].ReviewLog, before[a].ReviewLog)
		is.Equal(after[a].DeckID, before[a].DeckID)
	}
	for a := range histories {
		is.True(after[a].FsrsCard.Stability != before[a].FsrsCard.Stability)
	}

	// Upgrading twice isn't allowed.
	_, err = s.UpgradeFsrs(ctx, connect.NewRequest(&pb.UpgradeFsrsRequest{}))
	is.True(err != nil)
	_, err = s.PreviewFsrsUpgrade(ctx, connect.NewRequest(&pb.PreviewFsrsUpgradeRequest{}))
	is.True(err != nil)

	// The next review of a rebuilt card continues from its FSRS-6 memory
	// state.
	for _, a := range []string{"ADEEGMMO", "AEINRST"} {
		card := expected[a]
		fakenower.fakenow = card.Due.Add(time.Hour)
		info, err := fsrs.NewFSRS(testParams(t, FsrsVersion6, retention[a])).
			Next(card, fakenower.fakenow, fsrs.Good)
		is.NoErr(err)
		scoreCard(t, s, a, pb.Score_SCORE_GOOD)
		got := getCard(t, s, a)
		is.Equal(got.Stability, info.Card.Stability)
		is.Equal(got.Difficulty, info.Card.Difficulty)
	}

	// New cards use FSRS-6 weights.
	scoreCard(t, s, "ADEEHMMO", pb.Score_SCORE_GOOD)
	is.Equal(getCard(t, s, "ADEEHMMO").Stability, fsrs.DefaultWeights()[2])
}

// A card reviewed between reading it and writing its new memory state keeps
// the state from that review.
func TestSetCardMemoryStatesSkipsReviewedCards(t *testing.T) {
	is := is.New(t)
	s, fakenower, cleanup := upgradeTestServer(t, true)
	defer cleanup()
	ctx := ctxForTests()

	_, err := s.AddCards(ctx, connect.NewRequest(&pb.AddCardsRequest{
		Lexicon:    "NWL23",
		Alphagrams: []string{"ADEEGMMO", "AEILNOR"},
	}))
	is.NoErr(err)
	scoreCard(t, s, "ADEEGMMO", pb.Score_SCORE_GOOD)
	scoreCard(t, s, "AEILNOR", pb.Score_SCORE_GOOD)

	ids, err := s.Queries.GetCardIDs(ctx, 42)
	is.NoErr(err)
	rows, err := s.Queries.GetCardsByIDs(ctx, models.GetCardsByIDsParams{UserID: 42, Ids: ids})
	is.NoErr(err)
	is.Equal(len(rows), 2)
	update := models.SetCardMemoryStatesParams{UserID: 42}
	for _, r := range rows {
		update.Ids = append(update.Ids, r.ID)
		// Not round numbers, so that a lossy JSON round trip would show.
		update.Stabilities = append(update.Stabilities, 12.345678901234567)
		update.Difficulties = append(update.Difficulties, 3.3333333333333335)
		update.LastReviews = append(update.LastReviews, r.LastReview)
	}

	// AEILNOR gets reviewed in the meantime.
	fakenower.fakenow = fakenower.fakenow.Add(4 * 24 * time.Hour)
	scoreCard(t, s, "AEILNOR", pb.Score_SCORE_GOOD)
	reviewed := getCard(t, s, "AEILNOR")
	untouched := getCard(t, s, "ADEEGMMO")

	n, err := s.Queries.SetCardMemoryStates(ctx, update)
	is.NoErr(err)
	is.Equal(n, int64(1))

	is.Equal(getCard(t, s, "AEILNOR"), reviewed)
	got := getCard(t, s, "ADEEGMMO")
	is.Equal(got.Stability, 12.345678901234567)
	is.Equal(got.Difficulty, 3.3333333333333335)
	got.Stability, got.Difficulty = untouched.Stability, untouched.Difficulty
	is.Equal(got, untouched)

	// Another user's cards can't be updated.
	update.UserID = 43
	n, err = s.Queries.SetCardMemoryStates(ctx, update)
	is.NoErr(err)
	is.Equal(n, int64(0))
}

func TestUpgradeFsrsNewUser(t *testing.T) {
	is := is.New(t)
	s, _, cleanup := upgradeTestServer(t, false)
	defer cleanup()
	_, err := s.PreviewFsrsUpgrade(ctxForTests(), connect.NewRequest(&pb.PreviewFsrsUpgradeRequest{}))
	is.True(err != nil)
}
