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

func shortTermServer(t *testing.T, version int16) (*Server, *FakeNower, func()) {
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
	s.Nower = fakenower

	// Short-term params as stored by go-fsrs v3, which had no learning steps.
	_, err = dbPool.Exec(ctx, `INSERT INTO wordvault_params (user_id, params, fsrs_version)
		VALUES (42, $1, $2)`, v3ShortTermParams, version)
	is.NoErr(err)
	return s, fakenower, dbPool.Close
}

const v3ShortTermParams = `{"RequestRetention": 0.9, "MaximumInterval": 1825,
	"Weights": [0.40255, 1.18385, 3.173, 15.69105, 7.1949, 0.5345, 1.4604, 0.0046,
	1.54575, 0.1192, 1.01925, 1.9395, 0.11, 0.29605, 2.2698, 0.2315, 2.9898,
	0.51655, 0.6621], "Decay": -0.5, "Factor": 0.2345679012345678,
	"EnableShortTerm": true, "EnableFuzz": true}`

// A card graduating to a 1-day interval, minus the most furtherFuzzDueDate
// can move it earlier.
const graduatedMin = 21 * time.Hour

func getCard(t *testing.T, s *Server, alphagram string) fsrs.Card {
	is := is.New(t)
	info, err := s.GetCardInformation(ctxForTests(), connect.NewRequest(&pb.GetCardInfoRequest{
		Lexicon:    "NWL23",
		Alphagrams: []string{alphagram},
	}))
	is.NoErr(err)
	is.Equal(len(info.Msg.Cards), 1)
	card := fsrs.Card{}
	is.NoErr(json.Unmarshal(info.Msg.Cards[0].CardJsonRepr, &card))
	return card
}

func TestShortTermScheduler(t *testing.T) {
	for _, version := range []int16{FsrsVersion5, FsrsVersion6} {
		testShortTermScheduler(t, version)
	}
}

func testShortTermScheduler(t *testing.T, version int16) {
	is := is.New(t)
	s, fakenower, cleanup := shortTermServer(t, version)
	defer cleanup()
	ctx := ctxForTests()

	fakenower.fakenow, _ = time.Parse(time.RFC3339, "2024-09-22T23:00:00Z")
	_, err := s.AddCards(ctx, connect.NewRequest(&pb.AddCardsRequest{
		Lexicon:    "NWL23",
		Alphagrams: []string{"ADEEGMMO"},
	}))
	is.NoErr(err)

	score := func(sc pb.Score) time.Duration {
		res, err := s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
			Score:     sc,
			Lexicon:   "NWL23",
			Alphagram: "ADEEGMMO",
		}))
		is.NoErr(err)
		next := res.Msg.NextScheduled.AsTime()
		d := next.Sub(fakenower.fakenow)
		fakenower.fakenow = next.Add(time.Minute)
		return d
	}

	// New card, missed: first learning step.
	is.Equal(score(pb.Score_SCORE_AGAIN), time.Minute)
	is.Equal(getCard(t, s, "ADEEGMMO").State, fsrs.Learning)
	// Good moves to the next learning step.
	is.Equal(score(pb.Score_SCORE_GOOD), 10*time.Minute)
	is.Equal(getCard(t, s, "ADEEGMMO").State, fsrs.Learning)
	// Good on the last step graduates the card.
	is.True(score(pb.Score_SCORE_GOOD) >= graduatedMin)
	card := getCard(t, s, "ADEEGMMO")
	is.Equal(card.State, fsrs.Review)
	is.Equal(card.Lapses, uint64(0))

	// Missing a review card puts it in relearning.
	is.Equal(score(pb.Score_SCORE_AGAIN), 10*time.Minute)
	card = getCard(t, s, "ADEEGMMO")
	is.Equal(card.State, fsrs.Relearning)
	is.Equal(card.Lapses, uint64(1))
	// Missing it again keeps it in relearning.
	is.Equal(score(pb.Score_SCORE_AGAIN), 10*time.Minute)
	is.Equal(getCard(t, s, "ADEEGMMO").State, fsrs.Relearning)
	// Getting it right graduates it again.
	is.True(score(pb.Score_SCORE_GOOD) >= graduatedMin)
	is.Equal(getCard(t, s, "ADEEGMMO").State, fsrs.Review)
}

// Cards put into relearning by go-fsrs v3 have no RemainingSteps. Missing
// one again must keep it in relearning rather than graduating it.
func TestShortTermLegacyRelearningCard(t *testing.T) {
	is := is.New(t)
	s, fakenower, cleanup := shortTermServer(t, FsrsVersion5)
	defer cleanup()
	ctx := ctxForTests()

	fakenower.fakenow, _ = time.Parse(time.RFC3339, "2024-09-22T23:00:00Z")
	_, err := s.AddCards(ctx, connect.NewRequest(&pb.AddCardsRequest{
		Lexicon:    "NWL23",
		Alphagrams: []string{"ADEEGMMO", "ADEEHMMO"},
	}))
	is.NoErr(err)

	// What v3 stored for a card just missed in review mode.
	missedAt := fakenower.fakenow
	legacyCard := stores.Card{Card: fsrs.Card{
		Due:        missedAt.Add(5 * time.Minute),
		Stability:  2.5,
		Difficulty: 6.0,
		Reps:       4,
		Lapses:     1,
		State:      fsrs.Relearning,
		LastReview: missedAt,
	}}
	legacyLog := []byte(`{"Rating":1,"ScheduledDays":0,"ElapsedDays":12,"Review":"` +
		missedAt.Format(time.RFC3339) + `","State":2}`)
	for _, alpha := range []string{"ADEEGMMO", "ADEEHMMO"} {
		is.NoErr(s.Queries.UpdateCard(ctx, models.UpdateCardParams{
			FsrsCard:      legacyCard,
			NextScheduled: toPGTimestamp(legacyCard.Due),
			UserID:        42, // the user from ctxForTests
			LexiconName:   "NWL23",
			Alphagram:     alpha,
			ReviewLogItem: legacyLog,
		}))
	}

	fakenower.fakenow = missedAt.Add(6 * time.Minute)
	res, err := s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
		Score:     pb.Score_SCORE_AGAIN,
		Lexicon:   "NWL23",
		Alphagram: "ADEEGMMO",
	}))
	is.NoErr(err)
	is.Equal(res.Msg.NextScheduled.AsTime().Sub(fakenower.fakenow), 10*time.Minute)
	is.Equal(getCard(t, s, "ADEEGMMO").State, fsrs.Relearning)

	// Good graduates it, as v3 did.
	res, err = s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
		Score:     pb.Score_SCORE_GOOD,
		Lexicon:   "NWL23",
		Alphagram: "ADEEHMMO",
	}))
	is.NoErr(err)
	is.True(res.Msg.NextScheduled.AsTime().Sub(fakenower.fakenow) >= graduatedMin)
	is.Equal(getCard(t, s, "ADEEHMMO").State, fsrs.Review)
}

func TestFurtherFuzzDueDate(t *testing.T) {
	is := is.New(t)
	now, _ := time.Parse(time.RFC3339, "2024-09-22T23:00:00Z")
	p := fsrs.DefaultParam()
	p.EnableFuzz = true

	// Spread of due times for a card in the given state, due a day from now.
	spread := func(p fsrs.Parameters, state fsrs.State) time.Duration {
		lo, hi := time.Duration(math.MaxInt64), time.Duration(0)
		for range 200 {
			card := fsrs.Card{State: state, Due: now.Add(24 * time.Hour)}
			furtherFuzzDueDate(p, now, &card)
			d := card.Due.Sub(now)
			is.True(d >= 21*time.Hour && d <= 27*time.Hour)
			lo, hi = min(lo, d), max(hi, d)
		}
		return hi - lo
	}

	// Review cards are jittered by up to 3 hours either way, in both modes.
	is.True(spread(p, fsrs.Review) > 4*time.Hour)
	p.EnableShortTerm = true
	is.True(spread(p, fsrs.Review) > 4*time.Hour)
	// Learning steps are exact.
	is.Equal(spread(p, fsrs.Learning), time.Duration(0))
	is.Equal(spread(p, fsrs.Relearning), time.Duration(0))
	// And nothing happens with fuzz off.
	p.EnableFuzz = false
	is.Equal(spread(p, fsrs.Review), time.Duration(0))
}

// Daily stats count each card once, by its first review of the day, so that
// relearning a missed card doesn't hide the miss (#60).
func TestShortTermDailyProgress(t *testing.T) {
	is := is.New(t)
	s, fakenower, cleanup := shortTermServer(t, FsrsVersion6)
	defer cleanup()
	ctx := ctxForTests()

	fakenower.fakenow, _ = time.Parse(time.RFC3339, "2024-09-21T10:00:00Z")
	_, err := s.AddCards(ctx, connect.NewRequest(&pb.AddCardsRequest{
		Lexicon:    "NWL23",
		Alphagrams: []string{"ADEEGMMO", "ADEEHMMO", "AEILNOR"},
	}))
	is.NoErr(err)
	score := func(alpha string, sc pb.Score) {
		_, err := s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
			Score: sc, Lexicon: "NWL23", Alphagram: alpha,
		}))
		is.NoErr(err)
		fakenower.fakenow = fakenower.fakenow.Add(15 * time.Minute)
	}
	// Yesterday: two cards learned.
	for _, alpha := range []string{"ADEEHMMO", "AEILNOR"} {
		score(alpha, pb.Score_SCORE_EASY)
	}

	fakenower.fakenow, _ = time.Parse(time.RFC3339, "2024-09-22T10:00:00Z")
	// A new card, missed, then relearned.
	score("ADEEGMMO", pb.Score_SCORE_AGAIN)
	score("ADEEGMMO", pb.Score_SCORE_GOOD)
	score("ADEEGMMO", pb.Score_SCORE_GOOD)
	// A review card, missed, then relearned.
	score("ADEEHMMO", pb.Score_SCORE_AGAIN)
	score("ADEEHMMO", pb.Score_SCORE_GOOD)
	// A review card, known.
	score("AEILNOR", pb.Score_SCORE_GOOD)

	want := map[string]int32{
		"New": 1, "NewMissed": 1, "NewHard": 0, "NewGood": 0, "NewEasy": 0,
		"Reviewed": 2, "ReviewedMissed": 1, "ReviewedHard": 0, "ReviewedGood": 1, "ReviewedEasy": 0,
		"Repeats": 3,
	}
	res, err := s.GetDailyProgress(ctx, connect.NewRequest(&pb.GetDailyProgressRequest{Timezone: "UTC"}))
	is.NoErr(err)
	is.Equal(res.Msg.ProgressStats, want)

	byDeck, err := s.GetDailyProgressByDeck(ctx, connect.NewRequest(&pb.GetDailyProgressByDeckRequest{Timezone: "UTC"}))
	is.NoErr(err)
	is.Equal(len(byDeck.Msg.Items), 1)
	is.Equal(byDeck.Msg.Items[0].ProgressStats, want)
}

// go-fsrs v3 let stability shrink to 0 on cards missed many times in a row,
// then stored a NaN interval as ScheduledDays 2^63. go-fsrs v4 rejects cards
// with stability below its minimum, so these couldn't be scored at all.
func TestLegacyZeroStabilityCard(t *testing.T) {
	for _, shortTerm := range []bool{false, true} {
		for _, rating := range []pb.Score{pb.Score_SCORE_AGAIN, pb.Score_SCORE_GOOD} {
			testLegacyZeroStabilityCard(t, shortTerm, rating)
		}
	}
}

func testLegacyZeroStabilityCard(t *testing.T, shortTerm bool, rating pb.Score) {
	is := is.New(t)
	s, fakenower, cleanup := shortTermServer(t, FsrsVersion5)
	defer cleanup()
	ctx := ctxForTests()
	_, err := s.DBPool.Exec(ctx, `UPDATE wordvault_params
		SET params = jsonb_set(params, '{EnableShortTerm}', to_jsonb($1::bool)) WHERE user_id = 42`, shortTerm)
	is.NoErr(err)

	fakenower.fakenow, _ = time.Parse(time.RFC3339, "2026-09-29T23:58:13Z")
	_, err = s.AddCards(ctx, connect.NewRequest(&pb.AddCardsRequest{
		Lexicon:    "NWL23",
		Alphagrams: []string{"ADEEGMMO"},
	}))
	is.NoErr(err)

	// A card as v3 stored it in production.
	var legacyCard stores.Card
	is.NoErr(json.Unmarshal([]byte(`{"Due": "2026-09-30T00:17:15.83832413Z", "Reps": 74,
		"State": 2, "Lapses": 8, "Stability": 0, "Difficulty": 9.904869569550643,
		"LastReview": "2026-09-29T23:58:13.83832413Z", "ElapsedDays": 14,
		"ScheduledDays": 9223372036854775808}`), &legacyCard))
	is.NoErr(s.Queries.UpdateCard(ctx, models.UpdateCardParams{
		FsrsCard:      legacyCard,
		NextScheduled: toPGTimestamp(legacyCard.Due),
		UserID:        42, // the user from ctxForTests
		LexiconName:   "NWL23",
		Alphagram:     "ADEEGMMO",
		ReviewLogItem: []byte(`{"Rating":1,"ScheduledDays":9223372036854775808,"ElapsedDays":14,` +
			`"Review":"2026-09-29T23:58:13.83832413Z","State":2}`),
	}))

	fakenower.fakenow, _ = time.Parse(time.RFC3339, "2026-10-16T12:00:00Z")
	res, err := s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
		Score:     rating,
		Lexicon:   "NWL23",
		Alphagram: "ADEEGMMO",
	}))
	is.NoErr(err)
	is.True(res.Msg.NextScheduled.AsTime().After(fakenower.fakenow))
	card := getCard(t, s, "ADEEGMMO")
	is.True(card.Stability >= minStability)
	is.True(card.ScheduledDays < 36500)
	is.True(!math.IsNaN(card.Difficulty))
}
