package wordvault

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/matryer/is"
	"github.com/open-spaced-repetition/go-fsrs/v3"
	"github.com/rs/zerolog/log"

	searchpb "github.com/domino14/word_db_server/api/rpc/wordsearcher"
	pb "github.com/domino14/word_db_server/api/rpc/wordvault"
	"github.com/domino14/word_db_server/config"
	"github.com/domino14/word_db_server/internal/auth"
	"github.com/domino14/word_db_server/internal/searchserver"
	"github.com/domino14/word_db_server/internal/stores/models"
)

var DefaultConfig = &config.Config{
	DataPath:          os.Getenv("WDB_DATA_PATH"),
	DBMigrationsPath:  os.Getenv("DB_MIGRATIONS_PATH"),
	MaxNonmemberCards: 10000,
	MaxCardsAdd:       1000,
}

func testDBURI(useDBName bool) string {
	user := os.Getenv("TEST_DBUSER")
	pass := os.Getenv("TEST_DBPASSWORD")
	dbname := os.Getenv("TEST_DBNAME")
	dbhost := os.Getenv("TEST_DBHOST")
	dbport := os.Getenv("TEST_DBPORT")
	sslmode := os.Getenv("TEST_DBSSLMODE")

	if !useDBName {
		dbname = ""
	}

	return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", user, pass, dbhost, dbport, dbname, sslmode)
}

func ctxForTests() context.Context {
	ctx := context.Background()
	ctx = log.Logger.WithContext(ctx)
	ctx = auth.StoreUserInContext(ctx, 42, "cesar", false)
	return ctx
}

func RecreateTestDB() error {
	ctx := context.Background()
	db, err := pgx.Connect(ctx, testDBURI(false))
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	log.Info().Msg("dropping db")
	_, err = db.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", os.Getenv("TEST_DBNAME")))
	if err != nil {
		return err
	}
	log.Info().Msg("creating db")
	_, err = db.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", os.Getenv("TEST_DBNAME")))
	if err != nil {
		return err
	}
	log.Info().Msg("running migrations")
	// And create all tables/sequences/etc.
	m, err := migrate.New(DefaultConfig.DBMigrationsPath, testDBURI(true))
	if err != nil {
		log.Err(err).Msg("on-new")
		return err
	}
	if err := m.Up(); err != nil {
		log.Err(err).Msg("on-up")
		return err
	}
	e1, e2 := m.Close()
	log.Err(e1).Msg("close-source")
	log.Err(e2).Msg("close-database")
	log.Info().Msg("created test db")
	return nil
}

func TeardownTestDB() error {
	ctx := context.Background()
	db, err := pgx.Connect(ctx, testDBURI(false))
	if err != nil {
		return err
	}
	defer db.Close(ctx)

	_, err = db.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", os.Getenv("TEST_DBNAME")))
	if err != nil {
		return err
	}
	return nil
}

func OpenDB(dburi string) (*pgxpool.Pool, error) {
	ctx := context.Background()

	dbPool, err := pgxpool.New(context.Background(), dburi)
	if err != nil {
		return nil, err
	}

	err = dbPool.Ping(ctx)
	if err != nil {
		return nil, err
	}
	return dbPool, nil
}

func TestAddCardsAndQuiz(t *testing.T) {
	is := is.New(t)

	err := RecreateTestDB()
	if err != nil {
		panic(err)
	}
	// defer TeardownTestDB()
	ctx := ctxForTests()

	dbPool, err := pgxpool.New(ctx, testDBURI(true))
	is.NoErr(err)
	defer dbPool.Close()

	q := models.New(dbPool)

	s := NewServer(DefaultConfig, dbPool, q, &searchserver.Server{Config: DefaultConfig})

	added, err := s.AddCards(ctx, connect.NewRequest(&pb.AddCardsRequest{
		Lexicon:    "NWL23",
		Alphagrams: []string{"ADEEGMMO", "ADEEHMMO"},
	}))
	is.NoErr(err)
	is.Equal(added.Msg.NumCardsAdded, int32(2))
	added, err = s.AddCards(ctx, connect.NewRequest(&pb.AddCardsRequest{
		Lexicon:    "NWL23",
		Alphagrams: []string{"ADEEHMMO", "AEFFGINR"},
	}))
	is.NoErr(err)
	is.Equal(added.Msg.NumCardsAdded, int32(1))

	res, err := s.GetNextScheduled(ctx, connect.NewRequest(&pb.GetNextScheduledRequest{
		Lexicon: "NWL23", Limit: 5,
	}))
	is.NoErr(err)
	fmt.Println(res)
	is.Equal(len(res.Msg.Cards), 3)

	for i := range 3 {
		card := fsrs.Card{}
		err = json.Unmarshal(res.Msg.Cards[i].CardJsonRepr, &card)
		is.NoErr(err)
		is.Equal(card.State, fsrs.New)
	}
}

type FakeNower struct{ fakenow time.Time }

func (f FakeNower) Now() time.Time {
	return f.fakenow
}

func TestScoreCard(t *testing.T) {
	is := is.New(t)

	err := RecreateTestDB()
	if err != nil {
		panic(err)
	}
	// defer TeardownTestDB()
	ctx := ctxForTests()

	dbPool, err := pgxpool.New(ctx, testDBURI(true))
	is.NoErr(err)
	defer dbPool.Close()

	q := models.New(dbPool)

	s := NewServer(DefaultConfig, dbPool, q, &searchserver.Server{Config: DefaultConfig})
	fakenower := &FakeNower{}
	s.Nower = fakenower

	_, err = s.AddCards(ctx, connect.NewRequest(&pb.AddCardsRequest{
		Lexicon:    "NWL23",
		Alphagrams: []string{"ADEEGMMO", "ADEEHMMO"},
	}))
	is.NoErr(err)

	// Try a few bad arguments:
	_, err = s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
		Score:     17,
		Lexicon:   "NWL23",
		Alphagram: "ADEEGMMO",
	}))
	is.Equal(err.Error(), "invalid_argument: invalid score")

	_, err = s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
		Score:     3,
		Lexicon:   "NWL23",
		Alphagram: "AEFFGINR",
	}))
	is.Equal(err.Error(), "invalid_argument: card with your input parameters was not found")

	fakenower.fakenow, err = time.Parse(time.RFC3339, "2024-09-22T23:00:00Z")
	is.NoErr(err)
	// Score a few times.
	res, err := s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
		Score:     pb.Score_SCORE_EASY,
		Lexicon:   "NWL23",
		Alphagram: "ADEEGMMO",
	}))
	is.NoErr(err)

	fakenower.fakenow = res.Msg.NextScheduled.AsTime()
	res, err = s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
		Score:     pb.Score_SCORE_EASY,
		Lexicon:   "NWL23",
		Alphagram: "ADEEGMMO",
	}))
	is.NoErr(err)
	fakenower.fakenow = res.Msg.NextScheduled.AsTime()

	res, err = s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
		Score:     pb.Score_SCORE_EASY,
		Lexicon:   "NWL23",
		Alphagram: "ADEEGMMO",
	}))
	is.NoErr(err)
	// Create a time three years after the fake now time above.
	// The card is scheduled in the far future after marking it easy just three times.
	threeyearsafter, err := time.Parse(time.RFC3339, "2027-09-22T23:00:00Z")
	is.NoErr(err)
	is.True(res.Msg.NextScheduled.AsTime().After(threeyearsafter))
}

func TestGetCards(t *testing.T) {
	is := is.New(t)

	err := RecreateTestDB()
	if err != nil {
		panic(err)
	}
	// defer TeardownTestDB()
	ctx := ctxForTests()

	dbPool, err := pgxpool.New(ctx, testDBURI(true))
	is.NoErr(err)
	defer dbPool.Close()

	q := models.New(dbPool)

	s := NewServer(DefaultConfig, dbPool, q, &searchserver.Server{Config: DefaultConfig})
	fakenower := &FakeNower{}
	s.Nower = fakenower

	_, err = s.AddCards(ctx, connect.NewRequest(&pb.AddCardsRequest{
		Lexicon:    "NWL23",
		Alphagrams: []string{"ADEEGMMO", "ADEEHMMO"},
	}))
	is.NoErr(err)

	fakenower.fakenow, err = time.Parse(time.RFC3339, "2024-09-22T23:00:00Z")
	is.NoErr(err)
	// Score a few times.
	res, err := s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
		Score:     pb.Score_SCORE_EASY,
		Lexicon:   "NWL23",
		Alphagram: "ADEEGMMO",
	}))
	is.NoErr(err)

	fakenower.fakenow = res.Msg.NextScheduled.AsTime()
	res, err = s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
		Score:     pb.Score_SCORE_EASY,
		Lexicon:   "NWL23",
		Alphagram: "ADEEGMMO",
	}))
	is.NoErr(err)
	fakenower.fakenow = res.Msg.NextScheduled.AsTime()

	res, err = s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
		Score:     pb.Score_SCORE_EASY,
		Lexicon:   "NWL23",
		Alphagram: "ADEEGMMO",
	}))
	is.NoErr(err)
	// Set it to some far off time in the future (hopefully i'm still alive)
	fakenower.fakenow, err = time.Parse(time.RFC3339, "2100-09-22T23:00:00Z")
	is.NoErr(err)
	info, err := s.GetCardInformation(ctx, connect.NewRequest(&pb.GetCardInfoRequest{
		Lexicon:    "NWL23",
		Alphagrams: []string{"ADEEGMMO", "ADEEHMMO"},
	}))
	is.NoErr(err)
	fmt.Println(info)
	is.Equal(len(info.Msg.Cards), 2)
	is.Equal(info.Msg.Cards[0].Retrievability, 0.43596977331178927)
	is.Equal(info.Msg.Cards[0].Alphagram.Alphagram, "ADEEGMMO")

	card := fsrs.Card{}
	err = json.Unmarshal(info.Msg.Cards[0].CardJsonRepr, &card)

	is.Equal(card.Reps, uint64(3))
	is.Equal(card.Difficulty, float64(1))
	is.Equal(card.State, fsrs.Review)

	err = json.Unmarshal(info.Msg.Cards[1].CardJsonRepr, &card)

	is.Equal(info.Msg.Cards[1].Alphagram.Alphagram, "ADEEHMMO")
	is.Equal(card.State, fsrs.New)

}

func TestEditCardScore(t *testing.T) {
	is := is.New(t)

	err := RecreateTestDB()
	if err != nil {
		panic(err)
	}
	// defer TeardownTestDB()
	ctx := ctxForTests()

	dbPool, err := pgxpool.New(ctx, testDBURI(true))
	is.NoErr(err)
	defer dbPool.Close()

	q := models.New(dbPool)

	s := NewServer(DefaultConfig, dbPool, q, &searchserver.Server{Config: DefaultConfig})
	fakenower := &FakeNower{}
	s.Nower = fakenower

	resp, err := s.WordSearchServer.Search(ctx, connect.NewRequest(
		searchserver.WordSearch([]*searchpb.SearchRequest_SearchParam{
			searchserver.SearchDescLexicon("NWL23"),
			searchserver.SearchDescLength(7, 7),
			searchserver.SearchDescProbRange(7601, 8000),
		}, false)))
	is.NoErr(err)

	alphaStrs := []string{}
	for i := range resp.Msg.Alphagrams {
		alphaStrs = append(alphaStrs, resp.Msg.Alphagrams[i].Alphagram)
	}

	_, err = s.AddCards(ctx, connect.NewRequest(&pb.AddCardsRequest{
		Lexicon:    "NWL23",
		Alphagrams: alphaStrs,
	}))

	is.NoErr(err)

	fakenower.fakenow, err = time.Parse(time.RFC3339, "2024-09-22T23:00:00Z")
	is.NoErr(err)
	// Score a few times.
	res, err := s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
		Score:     pb.Score_SCORE_EASY,
		Lexicon:   "NWL23",
		Alphagram: "AEGLPSU",
	}))
	is.NoErr(err)

	fakenower.fakenow = res.Msg.NextScheduled.AsTime()
	res, err = s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
		Score:     pb.Score_SCORE_EASY,
		Lexicon:   "NWL23",
		Alphagram: "AEGLPSU",
	}))
	is.NoErr(err)
	fakenower.fakenow = res.Msg.NextScheduled.AsTime().Add(5 * time.Second)

	// Let's get the cards that are due as of fakenow.

	cards, err := s.GetNextScheduled(ctx, connect.NewRequest(&pb.GetNextScheduledRequest{
		Lexicon: "NWL23",
		Limit:   500,
	}))
	is.NoErr(err)
	is.Equal(len(cards.Msg.Cards), 400)
	cidx := -1
	for i := range 400 {
		if cards.Msg.Cards[i].Alphagram.Alphagram == "AEGLPSU" {
			cidx = i
		}
	}

	// Now we made a mistake. We accidentally marked it missed.
	res, err = s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
		Score:     pb.Score_SCORE_AGAIN,
		Lexicon:   "NWL23",
		Alphagram: "AEGLPSU",
	}))
	is.NoErr(err)

	// Oops, let's mark it easy again 5 seconds later.
	fakenower.fakenow = fakenower.fakenow.Add(5 * time.Second)
	res, err = s.EditLastScore(ctx, connect.NewRequest(&pb.EditLastScoreRequest{
		NewScore:     pb.Score_SCORE_EASY,
		Lexicon:      "NWL23",
		Alphagram:    "AEGLPSU",
		LastCardRepr: cards.Msg.Cards[cidx].CardJsonRepr,
	}))
	is.NoErr(err)

	// Create a time three years after the fake now time above.
	// The card is scheduled in the far future after marking it easy just three times.
	threeyearsafter, err := time.Parse(time.RFC3339, "2027-09-22T23:00:00Z")
	is.NoErr(err)
	is.True(res.Msg.NextScheduled.AsTime().After(threeyearsafter))

	info, err := s.GetCardInformation(ctx, connect.NewRequest(&pb.GetCardInfoRequest{
		Lexicon:    "NWL23",
		Alphagrams: []string{"AEGLPSU"},
	}))

	is.Equal(len(info.Msg.Cards), 1)
	is.Equal(info.Msg.Cards[0].Alphagram.Alphagram, "AEGLPSU")

	card := fsrs.Card{}
	err = json.Unmarshal(info.Msg.Cards[0].CardJsonRepr, &card)

	is.Equal(card.Reps, uint64(3))
	is.Equal(card.Difficulty, float64(1))
	is.Equal(card.Lapses, uint64(0))
	is.Equal(card.State, fsrs.Review)

}

func TestIntervalVariability(t *testing.T) {
	is := is.New(t)
	err := RecreateTestDB()
	if err != nil {
		panic(err)
	}
	// defer TeardownTestDB()
	ctx := ctxForTests()

	dbPool, err := pgxpool.New(ctx, testDBURI(true))
	is.NoErr(err)
	defer dbPool.Close()

	q := models.New(dbPool)

	s := NewServer(DefaultConfig, dbPool, q, &searchserver.Server{Config: DefaultConfig})
	fakenower := &FakeNower{}
	s.Nower = fakenower

	added, err := s.AddCards(ctx, connect.NewRequest(&pb.AddCardsRequest{
		Lexicon:    "NWL23",
		Alphagrams: []string{"ADEEGMMO"},
	}))
	is.NoErr(err)
	is.Equal(added.Msg.NumCardsAdded, int32(1))

	added, err = s.AddCards(ctx, connect.NewRequest(&pb.AddCardsRequest{
		Lexicon:    "NWL23",
		Alphagrams: []string{"ADEEGMMO"},
	}))
	is.NoErr(err)
	is.Equal(added.Msg.NumCardsAdded, int32(0))

	fakenower.fakenow, err = time.Parse(time.RFC3339, "2024-09-22T23:00:00Z")
	// Add a small fuzz because fsrs seeds based on the passed-in time.
	fakenower.fakenow = fakenower.fakenow.Add(time.Duration(rand.Int32()) * time.Microsecond)
	is.NoErr(err)
	res, err := s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
		Score:     pb.Score_SCORE_HARD,
		Lexicon:   "NWL23",
		Alphagram: "ADEEGMMO",
	}))
	is.NoErr(err)

	fakenower.fakenow = res.Msg.NextScheduled.AsTime().Add(5 * time.Second)
	res, err = s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
		Score:     pb.Score_SCORE_HARD,
		Lexicon:   "NWL23",
		Alphagram: "ADEEGMMO",
	}))
	is.NoErr(err)
	fakenower.fakenow = res.Msg.NextScheduled.AsTime().Add(5 * time.Second)

	res, err = s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
		Score:     pb.Score_SCORE_HARD,
		Lexicon:   "NWL23",
		Alphagram: "ADEEGMMO",
	}))
	is.NoErr(err)
	fakenower.fakenow = res.Msg.NextScheduled.AsTime().Add(5 * time.Second)

	res, err = s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
		Score:     pb.Score_SCORE_GOOD,
		Lexicon:   "NWL23",
		Alphagram: "ADEEGMMO",
	}))
	is.NoErr(err)
	fakenower.fakenow = res.Msg.NextScheduled.AsTime().Add(5 * time.Second)

	res, err = s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
		Score:     pb.Score_SCORE_EASY,
		Lexicon:   "NWL23",
		Alphagram: "ADEEGMMO",
	}))
	is.NoErr(err)
	fakenower.fakenow = res.Msg.NextScheduled.AsTime().Add(5 * time.Second)

	res, err = s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
		Score:     pb.Score_SCORE_AGAIN,
		Lexicon:   "NWL23",
		Alphagram: "ADEEGMMO",
	}))
	is.NoErr(err)
	fakenower.fakenow = res.Msg.NextScheduled.AsTime().Add(5 * time.Second)

	res, err = s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
		Score:     pb.Score_SCORE_GOOD,
		Lexicon:   "NWL23",
		Alphagram: "ADEEGMMO",
	}))
	is.NoErr(err)

	// Honestly the purpose of this test was just to run it a bunch of times
	// and verify that the following prints out times that are spread out.
	fmt.Println("next scheduled", res.Msg.NextScheduled)
	// Create a time one year after the fake now time above. The card
	// should have been scheduled before this.
	oneyearafter, err := time.Parse(time.RFC3339, "2025-09-22T23:00:00Z")
	is.NoErr(err)
	is.True(res.Msg.NextScheduled.AsTime().Before(oneyearafter))
}

func TestCardMemberLimits(t *testing.T) {
	is := is.New(t)
	err := RecreateTestDB()
	if err != nil {
		panic(err)
	}
	// defer TeardownTestDB()
	ctx := ctxForTests()

	dbPool, err := pgxpool.New(ctx, testDBURI(true))
	is.NoErr(err)
	defer dbPool.Close()

	q := models.New(dbPool)
	config := *DefaultConfig
	config.MaxNonmemberCards = 500

	s := NewServer(&config, dbPool, q, &searchserver.Server{Config: &config})

	resp, err := s.WordSearchServer.Search(ctx, connect.NewRequest(
		searchserver.WordSearch([]*searchpb.SearchRequest_SearchParam{
			searchserver.SearchDescLexicon("NWL23"),
			searchserver.SearchDescLength(7, 7),
			searchserver.SearchDescProbRange(7601, 8000),
		}, false)))
	is.NoErr(err)

	alphaStrs := []string{}
	for i := range resp.Msg.Alphagrams {
		alphaStrs = append(alphaStrs, resp.Msg.Alphagrams[i].Alphagram)
	}

	added, err := s.AddCards(ctx, connect.NewRequest(&pb.AddCardsRequest{
		Lexicon:    "NWL23",
		Alphagrams: alphaStrs,
	}))

	is.NoErr(err)
	is.Equal(added.Msg.NumCardsAdded, int32(400))

	resp, err = s.WordSearchServer.Search(ctx, connect.NewRequest(
		searchserver.WordSearch([]*searchpb.SearchRequest_SearchParam{
			searchserver.SearchDescLexicon("NWL23"),
			searchserver.SearchDescLength(8, 8),
			searchserver.SearchDescProbRange(8601, 9000),
		}, false)))
	is.NoErr(err)

	alphaStrs = []string{}
	for i := range resp.Msg.Alphagrams {
		alphaStrs = append(alphaStrs, resp.Msg.Alphagrams[i].Alphagram)
	}

	added, err = s.AddCards(ctx, connect.NewRequest(&pb.AddCardsRequest{
		Lexicon:    "NWL23",
		Alphagrams: alphaStrs,
	}))
	is.Equal(err.Error(), "invalid_argument: "+ErrNeedMembership.Error())

}

func TestOverdueCount(t *testing.T) {
	is := is.New(t)
	err := RecreateTestDB()
	if err != nil {
		panic(err)
	}
	// defer TeardownTestDB()
	ctx := ctxForTests()

	dbPool, err := pgxpool.New(ctx, testDBURI(true))
	is.NoErr(err)
	defer dbPool.Close()

	q := models.New(dbPool)
	config := *DefaultConfig
	config.MaxNonmemberCards = 500

	s := NewServer(&config, dbPool, q, &searchserver.Server{Config: &config})
	fakenower := &FakeNower{}
	s.Nower = fakenower
	fakenower.fakenow, _ = time.Parse(time.RFC3339, "2024-09-22T23:00:00Z")

	resp, _ := s.WordSearchServer.Search(ctx, connect.NewRequest(
		searchserver.WordSearch([]*searchpb.SearchRequest_SearchParam{
			searchserver.SearchDescLexicon("NWL23"),
			searchserver.SearchDescLength(7, 7),
			searchserver.SearchDescProbRange(7601, 8000),
		}, false)))

	alphaStrs := []string{}
	for i := range resp.Msg.Alphagrams {
		alphaStrs = append(alphaStrs, resp.Msg.Alphagrams[i].Alphagram)
	}

	s.AddCards(ctx, connect.NewRequest(&pb.AddCardsRequest{
		Lexicon:    "NWL23",
		Alphagrams: alphaStrs,
	}))

	res, err := s.NextScheduledCount(ctx, connect.NewRequest(&pb.NextScheduledCountRequest{
		OnlyOverdue: true,
	}))
	is.NoErr(err)
	is.Equal(res.Msg.Breakdown["overdue"], uint32(400))

	// test tz handling
	// Set the time to an hour before we added the cards. A little ghetto,
	// but it'll do for our tests
	fakenower.fakenow, _ = time.Parse(time.RFC3339, "2024-09-22T22:00:00Z")
	// Then get the scheduled counts. It's 2024-09-23 in Singapore at the above time.
	res, err = s.NextScheduledCount(ctx, connect.NewRequest(&pb.NextScheduledCountRequest{
		OnlyOverdue: false,
		Timezone:    "Asia/Singapore",
	}))
	is.NoErr(err)
	is.Equal(res.Msg.Breakdown["overdue"], uint32(0))
	is.Equal(res.Msg.Breakdown["2024-09-23"], uint32(400))

	res, err = s.NextScheduledCount(ctx, connect.NewRequest(&pb.NextScheduledCountRequest{
		OnlyOverdue: false,
		Timezone:    "America/New_York",
	}))
	is.NoErr(err)
	is.Equal(res.Msg.Breakdown["overdue"], uint32(0))
	is.Equal(res.Msg.Breakdown["2024-09-22"], uint32(400))

	// Restore the fake time.
	fakenower.fakenow, _ = time.Parse(time.RFC3339, "2024-09-22T23:00:00Z")

	for _, alpha := range alphaStrs {
		score := rand.IntN(4) + 1
		_, err := s.ScoreCard(ctx, connect.NewRequest(&pb.ScoreCardRequest{
			Score:     pb.Score(score),
			Lexicon:   "NWL23",
			Alphagram: alpha,
		}))
		is.NoErr(err)
	}

	// Scored 400 cards.
	res, err = s.NextScheduledCount(ctx, connect.NewRequest(&pb.NextScheduledCountRequest{
		OnlyOverdue: true,
	}))
	is.NoErr(err)
	is.Equal(res.Msg.Breakdown["overdue"], uint32(0))

	// Set the time to a couple days in the future and get a full breakdown of questions
	// due. There should be some overdue, and some due in the future (the ones that were
	// marked easier).
	fakenower.fakenow, _ = time.Parse(time.RFC3339, "2024-09-24T23:00:00Z")
	res, err = s.NextScheduledCount(ctx, connect.NewRequest(&pb.NextScheduledCountRequest{
		OnlyOverdue: false,
	}))

	is.NoErr(err)
	is.True(res.Msg.Breakdown["overdue"] > 0 && res.Msg.Breakdown["overdue"] != 400)
	sum := uint32(0)
	for _, v := range res.Msg.Breakdown {
		sum += v
	}
	is.Equal(sum, uint32(400))
}
