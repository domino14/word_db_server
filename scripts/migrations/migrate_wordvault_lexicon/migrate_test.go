package main

import (
	"context"
	"fmt"
	"os"
	"testing"

	gomigrate "github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/matryer/is"

	"github.com/domino14/word_db_server/config"
)

func TestDeletedUniqueAlphagrams(t *testing.T) {
	is := is.New(t)

	cfg := &config.Config{}
	cfg.Load(nil)
	fmt.Println(cfg.DataPath)

	alphas, err := deletedUniqueAlphagrams(cfg, "CSW24")
	is.NoErr(err)

	alphaMap := map[string]bool{}
	for _, alpha := range alphas {
		alphaMap[alpha] = true
	}

	is.Equal(alphaMap, map[string]bool{
		"AAMNNORSTW": true,
		"AEMNNORSTW": true,
		"AAMNNRST":   true,
		"AFL":        true,
		"AFLS":       true,
	})
}

func TestDeletedSharedAlphagrams(t *testing.T) {
	is := is.New(t)

	cfg := &config.Config{}
	cfg.Load(nil)
	fmt.Println(cfg.DataPath)

	alphas, err := deletedSharedAlphagrams(cfg, "CSW24")
	is.NoErr(err)

	alphaMap := map[string]bool{}
	for _, alpha := range alphas {
		alphaMap[alpha] = true
	}

	is.Equal(alphaMap, map[string]bool{
		"AEMNNRST": true,
	})
}

func TestDeletedUniqueAlphagramsOSPS(t *testing.T) {
	t.Skip()
	is := is.New(t)

	cfg := &config.Config{}
	cfg.Load(nil)
	fmt.Println(cfg.DataPath)

	alphas, err := deletedUniqueAlphagrams(cfg, "OSPS50")
	is.NoErr(err)
	fmt.Println("Deleted unique alphagrams", alphas)

	alphas, err = deletedSharedAlphagrams(cfg, "OSPS50")
	is.NoErr(err)
	fmt.Println("Deleted shared alphagrams", alphas)

	is.True(false)
}

// testDB creates an empty WordVault database of its own, so this package's
// tests don't clash with the wordvault package's when they run in parallel.
func testDB(t *testing.T, cfg *config.Config) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()
	name := os.Getenv("TEST_DBNAME") + "_lexmigrate"
	uri := func(db string) string {
		return fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=%s", os.Getenv("TEST_DBUSER"),
			os.Getenv("TEST_DBPASSWORD"), os.Getenv("TEST_DBHOST"), os.Getenv("TEST_DBPORT"),
			db, os.Getenv("TEST_DBSSLMODE"))
	}
	conn, err := pgx.Connect(ctx, uri(""))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	for _, q := range []string{"DROP DATABASE IF EXISTS " + name, "CREATE DATABASE " + name} {
		if _, err := conn.Exec(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	m, err := gomigrate.New(cfg.DBMigrationsPath, uri(name))
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Up(); err != nil {
		t.Fatal(err)
	}
	m.Close()
	pool, err := pgxpool.New(ctx, uri(name))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestMigrateMovesDecksAndPlans(t *testing.T) {
	is := is.New(t)
	ctx := context.Background()
	cfg := &config.Config{}
	cfg.Load(nil)
	pool := testDB(t, cfg)

	deck := func(user int, lex, name string) int64 {
		var id int64
		err := pool.QueryRow(ctx, `INSERT INTO wordvault_decks (user_id, lexicon_name, name)
			VALUES ($1, $2, $3) RETURNING id`, user, lex, name).Scan(&id)
		is.NoErr(err)
		return id
	}
	plan := func(user int, lex, name string, deckID *int64) int64 {
		var id int64
		err := pool.QueryRow(ctx, `INSERT INTO wordvault_study_plans
			(user_id, lexicon_name, name, deck_id, batch_unit, batch_size, stages)
			VALUES ($1, $2, $3, $4, 1, 500, '[]') RETURNING id`, user, lex, name, deckID).Scan(&id)
		is.NoErr(err)
		return id
	}

	// User 1 has two NWL18 decks, one of them named like a deck they already
	// made in NWL23, and plans on a deck and on the default deck.
	hard18 := deck(1, "NWL18", "Hard")
	easy18 := deck(1, "NWL18", "Easy")
	hard23 := deck(1, "NWL23", "hard")
	onDeck := plan(1, "NWL18", "Hard 8s", &hard18)
	onDefault := plan(1, "NWL18", "7s", nil)
	_, err := pool.Exec(ctx, `INSERT INTO wordvault_cards (user_id, lexicon_name, alphagram, next_scheduled, deck_id)
		VALUES (1, 'NWL18', 'AEINRST', now(), $1)`, hard18)
	is.NoErr(err)
	// User 2 already has a default-deck plan in NWL23 too.
	old := plan(2, "NWL18", "Old 7s", nil)
	kept := plan(2, "NWL23", "New 7s", nil)

	is.NoErr(migrate(cfg, pool, "NWL18", "NWL23"))

	deckRow := func(id int64) (string, string) {
		var lex, name string
		is.NoErr(pool.QueryRow(ctx, `SELECT lexicon_name, name FROM wordvault_decks WHERE id = $1`, id).Scan(&lex, &name))
		return lex, name
	}
	lex, name := deckRow(hard18)
	is.Equal(lex, "NWL23")
	is.Equal(name, "Hard (NWL18)")
	lex, name = deckRow(easy18)
	is.Equal(lex, "NWL23")
	is.Equal(name, "Easy")
	_, name = deckRow(hard23)
	is.Equal(name, "hard")

	planLex := func(id int64) string {
		var lex string
		is.NoErr(pool.QueryRow(ctx, `SELECT lexicon_name FROM wordvault_study_plans WHERE id = $1`, id).Scan(&lex))
		return lex
	}
	is.Equal(planLex(onDeck), "NWL23")
	is.Equal(planLex(onDefault), "NWL23")
	is.Equal(planLex(old), "NWL23")
	is.Equal(planLex(kept), "NWL23")

	var cardLex string
	var cardDeck int64
	is.NoErr(pool.QueryRow(ctx, `SELECT lexicon_name, deck_id FROM wordvault_cards
		WHERE user_id = 1 AND alphagram = 'AEINRST'`).Scan(&cardLex, &cardDeck))
	is.Equal(cardLex, "NWL23")
	is.Equal(cardDeck, hard18)
}
