package main

import (
	"context"
	"os"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	wglconfig "github.com/domino14/word-golib/config"
	"github.com/domino14/word-golib/tilemapping"
	pb "github.com/domino14/word_db_server/api/rpc/wordsearcher"
	"github.com/domino14/word_db_server/config"
	"github.com/domino14/word_db_server/internal/common"
	"github.com/domino14/word_db_server/internal/searchserver"
)

func migrate(cfg *config.Config, pool *pgxpool.Pool, fromLex, toLex string) error {
	ctx := context.Background()

	updateReq := searchserver.WordSearch([]*pb.SearchRequest_SearchParam{
		searchserver.SearchDescLexicon(toLex),
		searchserver.SearchDescNotInLexicon(pb.SearchRequest_PREVIOUS_VERSION),
		// Search for questions that have at least 2 anagrams. This new set
		// will be a superset of all alphagrams that could potentially be
		// in the old WordVault that need to be upgraded. The exceptions would
		// be if they added and removed two different anagrams that share
		// an alphagram.
		// We'll deal with that situation in the future.
		searchserver.SearchDescNumAnagrams(2, 100),
	}, false)
	s := &searchserver.Server{
		Config: cfg,
	}
	sr, err := s.Search(ctx, connect.NewRequest(updateReq))
	if err != nil {
		return err
	}

	// If any new words that were added are an anagram of any word currently in the
	// wordvault, this alphagram in the word vault needs to be reset.
	alphagramsStrs := make([]string, len(sr.Msg.Alphagrams))
	for i := 0; i < len(alphagramsStrs); i++ {
		alphagramsStrs[i] = sr.Msg.Alphagrams[i].Alphagram
	}
	log.Info().Int("num-alphagrams", len(alphagramsStrs)).Msg("max-affected-alphagrams")

	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	changeLexQuery := `UPDATE wordvault_cards SET lexicon_name = $1 WHERE lexicon_name = $2`

	t, err := tx.Exec(ctx, changeLexQuery, toLex, fromLex)
	if err != nil {
		return err
	}
	log.Info().Int("rows-affected", int(t.RowsAffected())).Msg("changed-lexica")

	// Now change the cards themselves

	changeCardsQuery := `
UPDATE wordvault_cards
SET
    fsrs_card = jsonb_set(
        jsonb_set(
            fsrs_card,
            '{Due}',
            to_jsonb(to_char(random_timestamp, 'YYYY-MM-DD"T"HH24:MI:SS"Z"'))
        ),
        '{Stability}',
        '1.0'::jsonb
    ),
    next_scheduled = random_timestamp
FROM (
    SELECT id, (NOW() + (INTERVAL '7 days' * RANDOM()))::timestamp AS random_timestamp
    FROM wordvault_cards
    WHERE lexicon_name = $1 AND alphagram = ANY($2)
) subquery
WHERE wordvault_cards.id = subquery.id;
	`
	t, err = tx.Exec(ctx, changeCardsQuery, toLex, alphagramsStrs)
	if err != nil {
		return err
	}
	log.Info().Int("rows-affected", int(t.RowsAffected())).Msg("rescheduled-new-cards")

	// Delete cards that have deleted words.
	deletedUniqueAlphas, err := deletedUniqueAlphagrams(cfg, toLex)
	deleteCardsQuery := `
		DELETE FROM wordvault_cards WHERE lexicon_name = $1 AND alphagram = ANY($2)`

	if len(deletedUniqueAlphas) > 0 {
		t, err = tx.Exec(ctx, deleteCardsQuery, toLex, deletedUniqueAlphas)
		if err != nil {
			return err
		}
		log.Info().Int("rows-affected", int(t.RowsAffected())).Msg("deleted-cards")
	}

	// Finally, reschedule cards that have deleted anagrams.
	deletedSharedAlphas, err := deletedSharedAlphagrams(cfg, toLex)
	if err != nil {
		return err
	}
	if len(deletedSharedAlphas) > 0 {
		t, err = tx.Exec(ctx, changeCardsQuery, toLex, deletedSharedAlphas)
		if err != nil {
			return err
		}
		log.Info().Int("rows-affected", int(t.RowsAffected())).Msg("rescheduled-deleted-shared-cards")
	}
	return tx.Commit(ctx)
}

func main() {
	cfg := &config.Config{}
	cfg.Load(nil)
	log.Info().Msgf("Loaded config: %v", cfg)

	if len(os.Args) < 3 {
		panic("need 2 arguments: before and after lexica")
	}

	zerolog.SetGlobalLevel(zerolog.InfoLevel)
	if strings.ToLower(cfg.LogLevel) == "debug" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}
	log.Debug().Msg("debug logging is on")

	dbPool, err := pgxpool.New(context.Background(), cfg.DBConnUri)
	if err != nil {
		panic(err)
	}
	err = migrate(cfg, dbPool, os.Args[1], os.Args[2])
	if err != nil {
		panic(err)
	}
	log.Info().Msg("done migrating")

}

func getPotentiallyDeletedAlphagrams(cfg *config.Config, toLex string) (map[string]bool, error) {
	ctx := context.Background()

	// Return all words that were deleted from `toLex`.
	deletedWordsReq := searchserver.WordSearch([]*pb.SearchRequest_SearchParam{
		searchserver.SearchDescLexicon(toLex),
		searchserver.SearchDescDeleted(),
	}, false)
	s := &searchserver.Server{
		Config: cfg,
	}
	sr, err := s.Search(ctx, connect.NewRequest(deletedWordsReq))
	if err != nil {
		return nil, err
	}
	// For the DELETED request, Msg.Alphagrams actually contains the words, and
	// not the alphagrams. So we must alphagrammize them and determine if they're
	// still in the lexicon.
	words := []common.Word{}

	wglconfig := &wglconfig.Config{DataPath: cfg.DataPath}
	dist, err := tilemapping.ProbableLetterDistribution(wglconfig, toLex)
	if err != nil {
		return nil, err
	}

	for _, alpha := range sr.Msg.Alphagrams {
		word := common.InitializeWord(alpha.Alphagram, dist)
		words = append(words, word)
	}

	potentiallyDeletedAlphas := map[string]bool{}
	for idx := range words {
		potentiallyDeletedAlphas[words[idx].MakeAlphagram()] = true
	}
	return potentiallyDeletedAlphas, nil
}

func deletedUniqueAlphagrams(cfg *config.Config, toLex string) ([]string, error) {
	ctx := context.Background()
	s := &searchserver.Server{
		Config: cfg,
	}

	potentiallyDeletedAlphas, err := getPotentiallyDeletedAlphagrams(cfg, toLex)
	if err != nil {
		return nil, err
	}
	alphasList := []string{}
	for k := range potentiallyDeletedAlphas {
		alphasList = append(alphasList, k)
	}

	deletedAlphasReq := searchserver.WordSearch([]*pb.SearchRequest_SearchParam{
		searchserver.SearchDescLexicon(toLex),
		searchserver.SearchDescAlphagramList(alphasList),
	}, false)

	sr, err := s.Search(ctx, connect.NewRequest(deletedAlphasReq))
	if err != nil {
		return nil, err
	}

	for _, alpha := range sr.Msg.Alphagrams {
		delete(potentiallyDeletedAlphas, alpha.Alphagram)
	}
	// all the alphagrams that remain are actual ones that were totally deleted.
	finalAlphasList := []string{}
	for k := range potentiallyDeletedAlphas {
		finalAlphasList = append(finalAlphasList, k)
	}
	return finalAlphasList, nil

}

func deletedSharedAlphagrams(cfg *config.Config, toLex string) ([]string, error) {
	// Return all alphagrams that had a word that was deleted from `toLex`,
	// but where valid words remain.
	// this function is almost the same as the one above.

	ctx := context.Background()
	s := &searchserver.Server{
		Config: cfg,
	}

	potentiallyDeletedAlphas, err := getPotentiallyDeletedAlphagrams(cfg, toLex)
	if err != nil {
		return nil, err
	}
	alphasList := []string{}
	for k := range potentiallyDeletedAlphas {
		alphasList = append(alphasList, k)
	}

	deletedAlphasReq := searchserver.WordSearch([]*pb.SearchRequest_SearchParam{
		searchserver.SearchDescLexicon(toLex),
		searchserver.SearchDescAlphagramList(alphasList),
	}, false)
	// Alphagrams that are still in the lexicon were not fully deleted. Only
	// some of their words were deleted.
	sr, err := s.Search(ctx, connect.NewRequest(deletedAlphasReq))
	if err != nil {
		return nil, err
	}
	finalAlphasList := []string{}

	for _, alpha := range sr.Msg.Alphagrams {
		finalAlphasList = append(finalAlphasList, alpha.Alphagram)
	}

	return finalAlphasList, nil

}
