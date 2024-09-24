package wordvault

import (
	"context"
	"fmt"
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
	"github.com/rs/zerolog/log"

	"github.com/domino14/word_db_server/config"
	"github.com/domino14/word_db_server/internal/auth"
	"github.com/domino14/word_db_server/internal/searchserver"
	"github.com/domino14/word_db_server/internal/stores/models"
	pb "github.com/domino14/word_db_server/rpc/api/wordvault"
)

var TestDBURI = os.Getenv("TEST_DB_URI")
var TestDBServerURI = os.Getenv("TEST_DBSERVER_URI")

var MigrationsPath = os.Getenv("DB_MIGRATIONS_PATH")
var TestDBName = "wordvault_test"

var DefaultConfig = &config.Config{
	DataPath: os.Getenv("WDB_DATA_PATH"),
}

func ctxForTests() context.Context {
	ctx := context.Background()
	ctx = log.Logger.WithContext(ctx)
	ctx = auth.StoreUserInContext(ctx, 42, "cesar")
	return ctx
}

func RecreateTestDB() error {
	ctx := context.Background()
	db, err := pgx.Connect(ctx, TestDBServerURI)
	if err != nil {
		return err
	}
	defer db.Close(ctx)
	log.Info().Msg("dropping db")
	_, err = db.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", TestDBName))
	if err != nil {
		return err
	}
	log.Info().Msg("creating db")
	_, err = db.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s", TestDBName))
	if err != nil {
		return err
	}
	log.Info().Msg("running migrations")
	// And create all tables/sequences/etc.
	m, err := migrate.New(MigrationsPath, TestDBURI)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil {
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
	db, err := pgx.Connect(ctx, TestDBServerURI)
	if err != nil {
		return err
	}
	defer db.Close(ctx)

	_, err = db.Exec(ctx, fmt.Sprintf("DROP DATABASE IF EXISTS %s", TestDBName))
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

	dbPool, err := pgxpool.New(ctx, TestDBURI)
	is.NoErr(err)
	defer dbPool.Close()

	q := models.New(dbPool)

	s := NewServer(DefaultConfig, dbPool, q, &searchserver.Server{Config: DefaultConfig})

	_, err = s.AddCards(ctx, connect.NewRequest(&pb.AddCardsRequest{
		Lexicon:    "NWL23",
		Alphagrams: []string{"ADEEGMMO", "ADEEHMMO"},
	}))
	is.NoErr(err)
	_, err = s.AddCards(ctx, connect.NewRequest(&pb.AddCardsRequest{
		Lexicon:    "NWL23",
		Alphagrams: []string{"ADEEHMMO", "AEFFGINR"},
	}))
	is.NoErr(err)

	res, err := s.GetNextScheduled(ctx, connect.NewRequest(&pb.GetNextScheduledRequest{
		Lexicon: "NWL23", Limit: 5,
	}))
	is.NoErr(err)
	fmt.Println(res)
	is.Equal(len(res.Msg.Cards), 3)
	is.Equal(res.Msg.Cards[0].Status, pb.Status_STATUS_NEW)
	is.Equal(res.Msg.Cards[1].Status, pb.Status_STATUS_NEW)
	is.Equal(res.Msg.Cards[2].Status, pb.Status_STATUS_NEW)
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

	dbPool, err := pgxpool.New(ctx, TestDBURI)
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

	dbPool, err := pgxpool.New(ctx, TestDBURI)
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
	is.Equal(info.Msg.Cards[0].NumAsked, int32(3))
	is.Equal(info.Msg.Cards[0].Difficulty, float64(1))
	is.Equal(info.Msg.Cards[0].Status, pb.Status_STATUS_REVIEW)

	is.Equal(info.Msg.Cards[1].Alphagram.Alphagram, "ADEEHMMO")
	is.Equal(info.Msg.Cards[1].Status, pb.Status_STATUS_NEW)

}
