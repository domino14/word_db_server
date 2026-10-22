package wordvault

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/matryer/is"

	pb "github.com/domino14/word_db_server/api/rpc/wordvault"
	"github.com/domino14/word_db_server/internal/searchserver"
	"github.com/domino14/word_db_server/internal/stores/models"
)

func leaderboardServer(t *testing.T, ctx context.Context) (*Server, *FakeNower, *pgxpool.Pool) {
	is := is.New(t)
	err := RecreateTestDB()
	if err != nil {
		panic(err)
	}
	dbPool, err := pgxpool.New(ctx, testDBURI(true))
	is.NoErr(err)
	t.Cleanup(dbPool.Close)

	s := NewServer(DefaultConfig, dbPool, models.New(dbPool), &searchserver.Server{Config: DefaultConfig})
	fakenower := &FakeNower{}
	s.Nower = fakenower

	// auth_user belongs to Aerolith; the test DB only has WordVault's tables.
	_, err = dbPool.Exec(ctx, `CREATE TABLE auth_user (id BIGINT NOT NULL, username TEXT)`)
	is.NoErr(err)
	_, err = dbPool.Exec(ctx, `INSERT INTO auth_user (id, username) VALUES (42, 'cesar'), (43, 'other')`)
	is.NoErr(err)
	_, err = s.AddCards(ctx, connect.NewRequest(&pb.AddCardsRequest{
		Lexicon:    "NWL23",
		Alphagrams: []string{"ADEEGMMO", "ADEEHMMO", "AEFFGINR"},
	}))
	is.NoErr(err)
	return s, fakenower, dbPool
}

func pgDate(t *testing.T, day string) time.Time {
	d, err := time.Parse("2006-01-02", day)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestLeaderboards(t *testing.T) {
	is := is.New(t)
	ctx := ctxForTests()
	s, fakenower, dbPool := leaderboardServer(t, ctx)

	score := func(now, alphagram string) {
		fakenower.fakenow, _ = time.Parse(time.RFC3339, now)
		_, err := s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
			Score:     pb.Score_SCORE_GOOD,
			Lexicon:   "NWL23",
			Alphagram: alphagram,
		}))
		is.NoErr(err)
	}
	daily := func() map[string]int32 {
		res, err := s.GetDailyLeaderboard(ctx, connect.NewRequest(&pb.GetDailyLeaderboardRequest{}))
		is.NoErr(err)
		m := map[string]int32{}
		for _, item := range res.Msg.Items {
			m[item.User] = item.CardsStudied
		}
		return m
	}

	// 23:00 on Sep 22, Pacific.
	score("2024-09-23T06:00:00Z", "ADEEGMMO")
	is.Equal(daily(), map[string]int32{"cesar": 1})
	// The same card again the same day still counts once.
	score("2024-09-23T06:10:00Z", "ADEEGMMO")
	is.Equal(daily(), map[string]int32{"cesar": 1})

	// From Wordwalls: only ADEEHMMO is new today, but both are Wordwalls cards.
	fakenower.fakenow = fakenower.fakenow.Add(time.Minute)
	_, err := s.BatchScoreCards(ctx, connect.NewRequest(&pb.BatchScoreCardsRequest{
		Lexicon: "NWL23",
		Scores: []*pb.CardScore{
			{Alphagram: "ADEEGMMO", Score: pb.Score_SCORE_GOOD},
			{Alphagram: "ADEEHMMO", Score: pb.Score_SCORE_AGAIN},
		},
	}))
	is.NoErr(err)
	is.Equal(daily(), map[string]int32{"cesar": 2})
	var wordwallsCards int32
	err = dbPool.QueryRow(ctx, `SELECT wordwalls_cards FROM wordvault_daily_studied
		WHERE user_id = 42 AND day = '2024-09-22'`).Scan(&wordwallsCards)
	is.NoErr(err)
	is.Equal(wordwallsCards, int32(2))

	// Midnight Pacific starts a new day, though it's been 07:00 UTC for a while.
	score("2024-09-23T07:00:00Z", "AEFFGINR")
	is.Equal(daily(), map[string]int32{"cesar": 1})

	q := models.New(dbPool)
	for _, row := range []models.AddDailyStudiedParams{
		{Day: toPGDate(pgDate(t, "2024-09-20")), UserID: 43, Cards: 5},
		// Eight days back, so not in the week.
		{Day: toPGDate(pgDate(t, "2024-09-15")), UserID: 42, Cards: 100},
	} {
		is.NoErr(q.AddDailyStudied(ctx, row))
	}

	res, err := s.GetWeeklyLeaderboard(ctx, connect.NewRequest(&pb.GetWeeklyLeaderboardRequest{}))
	is.NoErr(err)
	is.Equal(res.Msg.Days, []string{"2024-09-17", "2024-09-18", "2024-09-19", "2024-09-20",
		"2024-09-21", "2024-09-22", "2024-09-23"})
	is.Equal(len(res.Msg.Items), 2)
	is.Equal(res.Msg.Items[0].User, "other")
	is.Equal(res.Msg.Items[0].CardsStudied, int32(5))
	is.Equal(res.Msg.Items[0].CardsStudiedByDay, []int32{0, 0, 0, 5, 0, 0, 0})
	is.Equal(res.Msg.Items[1].User, "cesar")
	is.Equal(res.Msg.Items[1].CardsStudied, int32(3))
	is.Equal(res.Msg.Items[1].CardsStudiedByDay, []int32{0, 0, 0, 0, 0, 2, 1})
}

func TestBackfillDailyStudied(t *testing.T) {
	is := is.New(t)
	ctx := ctxForTests()
	s, fakenower, dbPool := leaderboardServer(t, ctx)

	for _, sc := range []struct{ now, alphagram string }{
		{"2024-09-20T18:00:00Z", "ADEEGMMO"},
		{"2024-09-20T19:00:00Z", "ADEEGMMO"},
		{"2024-09-20T19:00:00Z", "ADEEHMMO"},
		// Sep 21 in UTC, still Sep 20 Pacific.
		{"2024-09-21T03:00:00Z", "AEFFGINR"},
		{"2024-09-22T18:00:00Z", "ADEEGMMO"},
	} {
		fakenower.fakenow, _ = time.Parse(time.RFC3339, sc.now)
		_, err := s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
			Score:     pb.Score_SCORE_GOOD,
			Lexicon:   "NWL23",
			Alphagram: sc.alphagram,
		}))
		is.NoErr(err)
	}

	studied := func() map[string]int32 {
		rows, err := models.New(dbPool).GetLeaderboard(ctx, models.GetLeaderboardParams{
			FirstDay: toPGDate(pgDate(t, "2024-09-01")),
			LastDay:  toPGDate(pgDate(t, "2024-09-30")),
		})
		is.NoErr(err)
		m := map[string]int32{}
		for _, row := range rows {
			m[row.Day.Time.Format("2006-01-02")] = row.Cards
		}
		return m
	}
	want := map[string]int32{"2024-09-20": 3, "2024-09-22": 1}
	is.Equal(studied(), want)

	_, err := dbPool.Exec(ctx, `DELETE FROM wordvault_daily_studied`)
	is.NoErr(err)
	// A count that's already higher than the review logs say is kept.
	_, err = dbPool.Exec(ctx, `INSERT INTO wordvault_daily_studied (day, user_id, cards)
		VALUES ('2024-09-22', 42, 7)`)
	is.NoErr(err)

	n, err := models.New(dbPool).BackfillDailyStudied(ctx, toPGDate(pgDate(t, "2024-09-20")))
	is.NoErr(err)
	is.Equal(n, int64(2))
	is.Equal(studied(), map[string]int32{"2024-09-20": 3, "2024-09-22": 7})

	// Days before first_day aren't touched.
	_, err = dbPool.Exec(ctx, `DELETE FROM wordvault_daily_studied`)
	is.NoErr(err)
	_, err = models.New(dbPool).BackfillDailyStudied(ctx, toPGDate(pgDate(t, "2024-09-21")))
	is.NoErr(err)
	is.Equal(studied(), map[string]int32{"2024-09-22": 1})
}
