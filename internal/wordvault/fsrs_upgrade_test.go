package wordvault

import (
	"encoding/json"
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

func TestUpgradeFsrs(t *testing.T) {
	is := is.New(t)
	s, fakenower, cleanup := upgradeTestServer(t, true)
	defer cleanup()
	ctx := ctxForTests()

	_, err := s.AddCards(ctx, connect.NewRequest(&pb.AddCardsRequest{
		Lexicon:    "NWL23",
		Alphagrams: []string{"ADEEGMMO", "ADEEHMMO", "AEGLPSU"},
	}))
	is.NoErr(err)

	// ADEEGMMO has a review history under FSRS-5.
	type review struct {
		at     time.Time
		rating fsrs.Rating
	}
	var history []review
	for _, rating := range []fsrs.Rating{fsrs.Good, fsrs.Good, fsrs.Again, fsrs.Good, fsrs.Easy} {
		history = append(history, review{fakenower.fakenow, rating})
		next := scoreCard(t, s, "ADEEGMMO", pb.Score(rating))
		fakenower.fakenow = next.Add(time.Hour)
	}
	v5Card := getCard(t, s, "ADEEGMMO")

	// AEGLPSU was imported from Cardbox, then never reviewed.
	importedAt := fakenower.fakenow.Add(-10 * 24 * time.Hour)
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

	is.Equal(fsrsVersion(t, s), int32(FsrsVersion5))
	preview, err := s.PreviewFsrsUpgrade(ctx, connect.NewRequest(&pb.PreviewFsrsUpgradeRequest{}))
	is.NoErr(err)
	sum := preview.Msg.Summary
	is.Equal(sum.CurrentVersion, int32(FsrsVersion5))
	is.Equal(sum.TargetVersion, int32(FsrsVersion6))
	is.Equal(sum.TotalCards, uint32(3))
	is.Equal(sum.RescheduledCards, uint32(1))
	is.Equal(sum.ImportedCards, uint32(1))
	is.Equal(sum.UnchangedCards, uint32(1))
	// The preview doesn't change anything.
	is.Equal(fsrsVersion(t, s), int32(FsrsVersion5))
	is.Equal(getCard(t, s, "ADEEGMMO"), v5Card)

	upgraded, err := s.UpgradeFsrs(ctx, connect.NewRequest(&pb.UpgradeFsrsRequest{}))
	is.NoErr(err)
	is.Equal(upgraded.Msg.Summary.RescheduledCards, uint32(1))
	is.Equal(upgraded.Msg.Summary.ImportedCards, uint32(1))
	is.Equal(fsrsVersion(t, s), int32(FsrsVersion6))

	// The reviewed card matches the same history scheduled with FSRS-6 all along.
	p, _, err := s.fsrsParamsAndVersion(ctx, 42, nil)
	is.NoErr(err)
	f := fsrs.NewFSRS(p)
	expected := fsrs.NewCard(history[0].at)
	for _, r := range history {
		info, err := f.Next(expected, r.at, r.rating)
		is.NoErr(err)
		expected = info.Card
	}
	got := getCard(t, s, "ADEEGMMO")
	is.Equal(got.Stability, expected.Stability)
	is.Equal(got.Difficulty, expected.Difficulty)
	is.Equal(got.Reps, expected.Reps)
	is.Equal(got.Lapses, expected.Lapses)
	is.True(got.Stability != v5Card.Stability)
	// Due may differ by our extra time-of-day fuzz.
	is.True(got.Due.Sub(expected.Due).Abs() <= 12*time.Hour)

	// The imported card keeps its memory state and gets an FSRS-6 due date.
	gotImported := getCard(t, s, "AEGLPSU")
	is.Equal(gotImported.Stability, imported.Stability)
	is.Equal(gotImported.Difficulty, imported.Difficulty)
	expectedDue := importedAt.Add(time.Duration(intervalDays(p, 30)*24) * time.Hour)
	is.True(gotImported.Due.Sub(expectedDue).Abs() <= 12*time.Hour)

	// The new card is untouched.
	is.Equal(getCard(t, s, "ADEEHMMO").State, fsrs.New)

	// next_scheduled matches the cards' due dates.
	rows, err := s.Queries.GetCards(ctx, models.GetCardsParams{
		UserID:      42,
		LexiconName: "NWL23",
		Alphagrams:  []string{"ADEEGMMO", "AEGLPSU"},
	})
	is.NoErr(err)
	for _, row := range rows {
		is.True(row.NextScheduled.Time.Equal(row.FsrsCard.Due))
	}

	// Upgrading twice isn't allowed.
	_, err = s.UpgradeFsrs(ctx, connect.NewRequest(&pb.UpgradeFsrsRequest{}))
	is.True(err != nil)
	_, err = s.PreviewFsrsUpgrade(ctx, connect.NewRequest(&pb.PreviewFsrsUpgradeRequest{}))
	is.True(err != nil)

	// New reviews use FSRS-6 weights.
	scoreCard(t, s, "ADEEHMMO", pb.Score_SCORE_GOOD)
	is.Equal(getCard(t, s, "ADEEHMMO").Stability, fsrs.DefaultWeights()[2])
}

func TestUpgradeFsrsNewUser(t *testing.T) {
	is := is.New(t)
	s, _, cleanup := upgradeTestServer(t, false)
	defer cleanup()
	_, err := s.PreviewFsrsUpgrade(ctxForTests(), connect.NewRequest(&pb.PreviewFsrsUpgradeRequest{}))
	is.True(err != nil)
}
