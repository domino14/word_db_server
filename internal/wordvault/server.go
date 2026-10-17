package wordvault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"sort"
	"strings"
	"time"

	"connectrpc.com/connect"
	wglconfig "github.com/domino14/word-golib/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/open-spaced-repetition/go-fsrs/v4"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	searchpb "github.com/domino14/word_db_server/api/rpc/wordsearcher"
	pb "github.com/domino14/word_db_server/api/rpc/wordvault"
	"github.com/domino14/word_db_server/config"
	"github.com/domino14/word_db_server/internal/auth"
	"github.com/domino14/word_db_server/internal/searchserver"
	"github.com/domino14/word_db_server/internal/stores"
	"github.com/domino14/word_db_server/internal/stores/models"
)

var ErrNeedMembership = errors.New("adding these cards would put you over your limit; please upgrade your account to add more cards <3")
var ErrMaybeRefreshApp = invalidArgError("Card with your input parameters was not found. Please refresh this page as the app may have updated.")
var ErrMaintenance = connect.NewError(connect.CodeUnavailable, errors.New("WordVault App is currently undergoing maintenance. Please wait a few moments and try again."))
var ErrNoSearchResults = connect.NewError(connect.CodeNotFound, errors.New("no cards match the search criteria"))

const CardInOtherDeckPreviewLimit = 10
const JustReviewedInterval = time.Second * 10

type nower interface {
	Now() time.Time
}

type RealNower struct{}

func (r RealNower) Now() time.Time {
	return time.Now()
}

type Server struct {
	Config           *config.Config
	Queries          *models.Queries
	DBPool           *pgxpool.Pool
	WordSearchServer *searchserver.Server
	Nower            nower
}

func NewServer(cfg *config.Config, dbPool *pgxpool.Pool, queries *models.Queries, wordSearchServer *searchserver.Server) *Server {
	return &Server{cfg, queries, dbPool, wordSearchServer, RealNower{}}
}

func unauthenticated(msg string) *connect.Error {
	return connect.NewError(connect.CodeUnauthenticated, errors.New(msg))
}

func invalidArgError(msg string) *connect.Error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New(msg))
}

// resolveAlphagrams returns the alphagrams a card request applies to. These
// are either given directly, or found by running searchParams in the given
// lexicon. Exactly one of the two must be non-empty.
func (s *Server) resolveAlphagrams(ctx context.Context, lexicon string, alphagrams []string,
	searchParams []*searchpb.SearchRequest_SearchParam) ([]string, error) {

	if len(searchParams) == 0 {
		return alphagrams, nil
	}
	if len(alphagrams) > 0 {
		return nil, invalidArgError("cannot specify both alphagrams and search_params")
	}
	if lexicon == "" {
		return nil, invalidArgError("need a lexicon")
	}
	// Always search in the request's lexicon, regardless of any lexicon
	// condition the client sent.
	params := []*searchpb.SearchRequest_SearchParam{searchserver.SearchDescLexicon(lexicon)}
	for _, p := range searchParams {
		switch p.Condition {
		case searchpb.SearchRequest_LEXICON:
			continue
		case searchpb.SearchRequest_DELETED_WORD:
			return nil, invalidArgError("cannot search for deleted words here")
		}
		params = append(params, p)
	}
	if len(params) == 1 {
		return nil, invalidArgError("need at least one search condition besides the lexicon")
	}
	resp, err := s.WordSearchServer.Search(ctx, connect.NewRequest(searchserver.WordSearch(params, false)))
	if err != nil {
		return nil, err
	}
	if len(resp.Msg.Alphagrams) == 0 {
		return nil, ErrNoSearchResults
	}
	found := make([]string, len(resp.Msg.Alphagrams))
	for i, a := range resp.Msg.Alphagrams {
		found[i] = a.Alphagram
	}
	return found, nil
}

// lexiconAlphagrams checks that every alphagram exists in the lexicon and
// returns them as the lexicon stores them, without duplicates. A card for an
// alphagram the lexicon doesn't have (e.g. one with a blank) would come back
// from review with no words, so these are rejected rather than added.
func (s *Server) lexiconAlphagrams(ctx context.Context, lexicon string, alphagrams []string) ([]string, error) {
	cfg := &wglconfig.Config{DataPath: s.Config.DataPath}
	resp, err := s.WordSearchServer.Search(ctx, connect.NewRequest(searchserver.WordSearch(
		[]*searchpb.SearchRequest_SearchParam{
			searchserver.SearchDescLexicon(lexicon),
			searchserver.SearchDescAlphagramList(lookupAlphagrams(cfg, lexicon, alphagrams)),
		}, false)))
	if err != nil {
		return nil, err
	}
	inLexicon := map[string]bool{}
	for _, a := range resp.Msg.Alphagrams {
		inLexicon[a.Alphagram] = true
	}

	out := make([]string, 0, len(alphagrams))
	added := map[string]bool{}
	var invalid []string
	for _, a := range alphagrams {
		stored := a
		if !inLexicon[stored] {
			stored = lexiconAlphagram(cfg, lexicon, a)
		}
		if !inLexicon[stored] {
			invalid = append(invalid, a)
			continue
		}
		if !added[stored] {
			added[stored] = true
			out = append(out, stored)
		}
	}
	if len(invalid) > 0 {
		preview := invalid
		if len(preview) > 5 {
			preview = append(slices.Clone(preview[:5]), "...")
		}
		return nil, invalidArgError(fmt.Sprintf("%s has no %s: %s", lexicon,
			pluralize(len(invalid), "such alphagram", "such alphagrams"), strings.Join(preview, ", ")))
	}
	return out, nil
}

func pluralize(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func (s *Server) GetCardInformation(ctx context.Context, req *connect.Request[pb.GetCardInfoRequest]) (
	*connect.Response[pb.Cards], error) {

	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}

	// Load from user params
	params, err := s.fsrsParams(ctx, int64(user.DBID), nil)
	if err != nil {
		return nil, err
	}
	f := fsrs.NewFSRS(params)

	rows, err := s.Queries.GetCards(ctx, models.GetCardsParams{
		UserID:      int64(user.DBID),
		LexiconName: req.Msg.Lexicon,
		Alphagrams: lookupAlphagrams(&wglconfig.Config{DataPath: s.Config.DataPath},
			req.Msg.Lexicon, req.Msg.Alphagrams),
	})
	if err != nil {
		return nil, err
	}

	cards := make([]*pb.Card, len(rows))
	for i := range rows {
		fcard := rows[i].FsrsCard
		fixed := fcard.Card
		fixLegacyCard(&fixed)
		retrievability, err := f.Retrievability(fixed, s.Nower.Now())
		if err != nil {
			// Don't fail the whole request over one card with bad data.
			log.Err(err).Str("alpha", rows[i].Alphagram).Msg("retrievability-error")
		}
		cardbts, err := json.Marshal(fcard)
		if err != nil {
			return nil, err
		}
		revlogbts, err := json.Marshal(rows[i].ReviewLog)
		if err != nil {
			return nil, err
		}
		cards[i] = &pb.Card{
			Lexicon: req.Msg.Lexicon,
			// Just return the alphagram here. The purpose of this endpoint is for
			// its metadata, not to quiz on any of the cards.
			Alphagram:      &searchpb.Alphagram{Alphagram: rows[i].Alphagram},
			CardJsonRepr:   cardbts,
			Retrievability: retrievability,
			ReviewLog:      revlogbts,
		}
		cards[i].DeckId = uint64(rows[i].DeckID)
	}
	return connect.NewResponse(&pb.Cards{Cards: cards}), nil
}

func (s *Server) GetNextScheduled(ctx context.Context, req *connect.Request[pb.GetNextScheduledRequest]) (
	*connect.Response[pb.Cards], error) {
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

	params := models.GetNextScheduledParams{
		UserID:        int64(user.DBID),
		LexiconName:   req.Msg.Lexicon,
		Limit:         int32(req.Msg.Limit),
		NextScheduled: toPGTimestamp(s.Nower.Now()),
		DeckID:        int64(req.Msg.DeckId),
	}
	rows, err := s.Queries.GetNextScheduled(ctx, params)
	if err != nil {
		return nil, err
	}

	if len(rows) == 0 {
		return connect.NewResponse(&pb.Cards{}), nil
	}
	cards := make([]*pb.Card, len(rows))

	alphagrams := make([]string, len(rows))
	for i := range rows {
		alphagrams[i] = rows[i].Alphagram
	}
	// expand does not return the alphagrams in the order they came in.
	expandResponse, err := s.WordSearchServer.Search(
		ctx,
		connect.NewRequest(
			searchserver.WordSearch([]*searchpb.SearchRequest_SearchParam{
				searchserver.SearchDescLexicon(req.Msg.Lexicon),
				searchserver.SearchDescAlphagramList(alphagrams),
			}, true)))
	if err != nil {
		return nil, err
	}
	expandMap := map[string]*searchpb.Alphagram{}
	for _, alpha := range expandResponse.Msg.Alphagrams {
		expandMap[alpha.Alphagram] = alpha
	}

	for i := range rows {
		fcard := rows[i].FsrsCard
		cardbts, err := json.Marshal(fcard)
		if err != nil {
			return nil, err
		}
		cards[i] = &pb.Card{
			Lexicon:      req.Msg.Lexicon,
			Alphagram:    expandMap[rows[i].Alphagram],
			CardJsonRepr: cardbts,
		}
	}
	return connect.NewResponse(&pb.Cards{Cards: cards}), nil
}

func (s *Server) GetSingleNextScheduled(ctx context.Context, req *connect.Request[pb.GetSingleNextScheduledRequest]) (
	*connect.Response[pb.GetSingleNextScheduledResponse], error) {
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

	params, err := s.fsrsParamsForDeck(ctx, int64(user.DBID), int64(req.Msg.DeckId), nil)

	if err != nil {
		return nil, err
	}

	sqlParams := models.GetSingleNextScheduledParams{
		UserID:               int64(user.DBID),
		LexiconName:          req.Msg.Lexicon,
		NextScheduled:        toPGTimestamp(s.Nower.Now()),
		IsShortTermScheduler: params.EnableShortTerm,
		DeckID:               int64(req.Msg.DeckId),
	}

	row, err := s.Queries.GetSingleNextScheduled(ctx, sqlParams)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// Not an error.
			return connect.NewResponse(&pb.GetSingleNextScheduledResponse{}), nil
		}
		return nil, err
	}
	expandResponse, err := s.WordSearchServer.Search(
		ctx,
		connect.NewRequest(
			searchserver.WordSearch([]*searchpb.SearchRequest_SearchParam{
				searchserver.SearchDescLexicon(req.Msg.Lexicon),
				searchserver.SearchDescAlphagramList([]string{row.Alphagram}),
			}, true)))
	if err != nil {
		return nil, err
	}
	if len(expandResponse.Msg.Alphagrams) != 1 {
		return nil, errors.New("unexpected expand response!")
	}

	card := pb.Card{
		Lexicon:   req.Msg.Lexicon,
		Alphagram: expandResponse.Msg.Alphagrams[0],
		// sqlc can't detect that row.FsrsCard is of type fsrs.Card
		// because of the way the query is written, so we just pass
		// the raw bytes as they are.
		CardJsonRepr: row.FsrsCard,
	}
	card.DeckId = uint64(row.DeckID)

	resp := &pb.GetSingleNextScheduledResponse{
		Card:         &card,
		OverdueCount: uint32(row.TotalCount),
	}

	return connect.NewResponse(resp), nil
}

// FSRS versions a user can be on. See db/migrations/000008_fsrs_version.
const (
	FsrsVersion5 = 5 // weights from go-fsrs v3; existing users until they upgrade
	FsrsVersion6 = 6 // go-fsrs v4 default weights
)

// fsrs5Weights are the go-fsrs v3 default weights, which every user had
// before the go-fsrs v4 upgrade.
var fsrs5Weights = [19]float64{0.40255, 1.18385, 3.173, 15.69105, 7.1949,
	0.5345, 1.4604, 0.0046, 1.54575, 0.1192, 1.01925, 1.9395, 0.11, 0.29605,
	2.2698, 0.2315, 2.9898, 0.51655, 0.6621}

func weightsForVersion(version int16) fsrs.Weights {
	if version == FsrsVersion5 {
		w, err := fsrs.ConvertV5Weights(fsrs5Weights)
		if err != nil {
			panic(err) // the weights above are constant and finite
		}
		return w
	}
	return fsrs.DefaultWeights()
}

// applyFsrsVersion sets the weights for the user's FSRS version. Users can't
// customize weights, so stored weights are ignored: params stored by go-fsrs
// v3 have 19 weights, which would otherwise unmarshal into an invalid
// 21-weight array. Params stored by go-fsrs v3 also lack learning steps;
// nil steps would mean "no steps" in go-fsrs v4, so use the defaults.
func applyFsrsVersion(params *fsrs.Parameters, version int16) {
	params.W = weightsForVersion(version)
	if params.LearningSteps == nil {
		params.LearningSteps = fsrs.DefaultLearningSteps()
	}
	if params.RelearningSteps == nil {
		params.RelearningSteps = fsrs.DefaultRelearningSteps()
	}
}

// fsrsParamsAndVersion loads the user's global FSRS parameters and version.
func (s *Server) fsrsParamsAndVersion(ctx context.Context, dbid int64, maybeQ *models.Queries) (fsrs.Parameters, int16, error) {
	q := s.Queries
	if maybeQ != nil {
		log.Debug().Int64("userID", dbid).Msg("querying-params-with-tx")
		q = maybeQ
	}
	var params fsrs.Parameters
	var version int16
	row, err := q.LoadFsrsParams(ctx, dbid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No params exist for this user, so this is a new user.
			log.Debug().Int64("userID", dbid).Msg("no-params-found")
			params = fsrs.DefaultParam()
			params.EnableShortTerm = false
			params.EnableFuzz = true
			params.MaximumInterval = 365 * 5 // Default is 100 years, which is a bit optimistic
			version = FsrsVersion6
		} else {
			return fsrs.Parameters{}, 0, err
		}
	} else {
		params, version = row.Params, row.FsrsVersion
	}
	applyFsrsVersion(&params, version)
	return params, version, nil
}

func (s *Server) fsrsParams(ctx context.Context, dbid int64, maybeQ *models.Queries) (fsrs.Parameters, error) {
	params, _, err := s.fsrsParamsAndVersion(ctx, dbid, maybeQ)
	return params, err
}

// minStability is go-fsrs v4's lowest stability for a card that has been
// reviewed; it rejects cards below it.
const minStability = 0.001

// fixLegacyCard fixes up cards last scheduled by go-fsrs v3 so that go-fsrs v4
// accepts them and schedules them as v3 would have.
//
// v3 didn't track RemainingSteps. go-fsrs v4 never leaves a learning card at 0
// remaining steps (it graduates it instead), and treats 0 as "graduate on any
// grade", so even Again would send the card out for days. Treat these cards as
// being on their last step: Good graduates (as in v3), Again restarts the
// steps and Hard repeats the step.
//
// v3 also had no lower bound on stability, so cards missed many times in a
// row could end up with a stability of 0 or close to it, which v4 refuses to
// schedule. Raise it to v4's minimum.
func fixLegacyCard(card *fsrs.Card) {
	if (card.State == fsrs.Learning || card.State == fsrs.Relearning) && card.RemainingSteps == 0 {
		card.RemainingSteps = 1
	}
	if card.State != fsrs.New && !(card.Stability >= minStability) {
		card.Stability = minStability
	}
}

// deckOverride is what a deck can override in the user's FSRS parameters.
// Older overrides were stored as a whole fsrs.Parameters (from
// fsrs.DefaultParam(), so with fuzz off and a 100-year maximum interval); only
// these fields are read from them.
type deckOverride struct {
	RequestRetention float64
	EnableShortTerm  bool
}

// deckParams returns the FSRS parameters for a deck: the user's global params
// (already set up for their version), with the deck's override applied.
func deckParams(override []byte, global fsrs.Parameters) (fsrs.Parameters, error) {
	if len(override) == 0 {
		return global, nil
	}
	var o deckOverride
	if err := json.Unmarshal(override, &o); err != nil {
		return fsrs.Parameters{}, err
	}
	params := global
	params.RequestRetention = o.RequestRetention
	params.EnableShortTerm = o.EnableShortTerm
	return params, nil
}

// fsrsParamsForDeck returns FSRS parameters for a specific deck.
// It first checks for a deck-specific override, then falls back to global user params.
// If deckID is 0 (default deck), it uses global params directly.
//
// Accepts optional `maybeQ` instance of Queries for transactional consistency
// if there is an ongoing transaction.
func (s *Server) fsrsParamsForDeck(ctx context.Context, dbid int64, deckID int64, maybeQ *models.Queries) (fsrs.Parameters, error) {
	global, err := s.fsrsParams(ctx, dbid, maybeQ)
	if err != nil {
		return fsrs.Parameters{}, err
	}
	if deckID == 0 {
		// Default deck uses global params
		return global, nil
	}

	q := s.Queries
	if maybeQ != nil {
		q = maybeQ
	}

	deck, err := q.GetDeck(ctx, models.GetDeckParams{
		ID:     deckID,
		UserID: dbid,
	})
	if err != nil {
		return fsrs.Parameters{}, err
	}
	return deckParams(deck.FsrsParamsOverride, global)
}

func (s *Server) appMaintenance(ctx context.Context) (bool, error) {
	var exists bool

	err := s.DBPool.
		QueryRow(ctx, `
            SELECT EXISTS (
                SELECT 1
                FROM waffle_switch
                WHERE name = 'disable_games' AND active = 't'
            )
        `).
		Scan(&exists)

	if err != nil {
		// Check if the error is related to the table not existing.
		// If so, don't return an error. We want to make this app as
		// independent as we can.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "42P01" {
			log.Info().AnErr("pg-err", pgErr).Msg("waffle-table-not-defined")
			return false, nil
		}
		return false, err
	}

	return exists, nil
}

func (s *Server) ScoreCard(ctx context.Context, req *connect.Request[pb.ScoreCardRequest]) (
	*connect.Response[pb.ScoreCardResponse], error) {
	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	now := s.Nower.Now()
	if req.Msg.Score < 1 || req.Msg.Score > 4 {
		return nil, invalidArgError("invalid score")
	}
	if req.Msg.Lexicon == "" || req.Msg.Alphagram == "" {
		return nil, invalidArgError("no such lexicon or alphagram")
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

	cardrow, err := qtx.GetCard(ctx, models.GetCardParams{
		UserID:      int64(user.DBID),
		LexiconName: req.Msg.Lexicon,
		Alphagram:   req.Msg.Alphagram})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMaybeRefreshApp
		} else {
			return nil, err
		}
	}

	params, err := s.fsrsParamsForDeck(ctx, int64(user.DBID), cardrow.DeckID, qtx)
	if err != nil {
		return nil, err
	}

	revlog := cardrow.ReviewLog
	if justReviewed(revlog, now) {
		return nil, invalidArgError("this card was just reviewed")
	}

	card, rlog, err := scoreAndUpdateCard(ctx, qtx, params, int64(user.DBID), req.Msg.Lexicon,
		req.Msg.Alphagram, cardrow.FsrsCard, fsrs.Rating(req.Msg.Score), now)
	if err != nil {
		return nil, err
	}
	err = tx.Commit(ctx)
	if err != nil {
		return nil, err
	}
	cardJson, err := json.Marshal(card)
	if err != nil {
		return nil, err
	}
	log := log.Ctx(ctx)
	log.Info().Str("alpha", req.Msg.Alphagram).Str("lex", req.Msg.Lexicon).
		Int("score", int(req.Msg.Score)).
		Interface("revlog", rlog).
		Interface("card", card).
		Str("next-scheduled", card.Due.String()).Msg("card-scored")

	return connect.NewResponse(&pb.ScoreCardResponse{
		NextScheduled: timestamppb.New(card.Due),
		CardJsonRepr:  cardJson,
	}), nil

}

func justReviewed(revlog []stores.ReviewLog, now time.Time) bool {
	return len(revlog) > 0 && now.Sub(revlog[len(revlog)-1].Review) < JustReviewedInterval
}

// scoreAndUpdateCard applies a rating to a card and saves the new card state
// and review log entry, in the caller's transaction.
func scoreAndUpdateCard(ctx context.Context, qtx *models.Queries, params fsrs.Parameters,
	userID int64, lexicon, alphagram string, card stores.Card, rating fsrs.Rating,
	now time.Time) (stores.Card, fsrs.ReviewLog, error) {

	fixLegacyCard(&card.Card)
	info, err := fsrs.NewFSRS(params).Next(card.Card, now, rating)
	if err != nil {
		return stores.Card{}, fsrs.ReviewLog{}, err
	}
	card = stores.Card{Card: info.Card}
	rlogbts, err := json.Marshal(info.ReviewLog)
	if err != nil {
		return stores.Card{}, fsrs.ReviewLog{}, err
	}
	furtherFuzzDueDate(params, now, &card.Card)
	err = qtx.UpdateCard(ctx, models.UpdateCardParams{
		FsrsCard:      card,
		NextScheduled: toPGTimestamp(card.Due),
		UserID:        userID,
		LexiconName:   lexicon,
		Alphagram:     alphagram,
		ReviewLogItem: rlogbts,
	})
	if err != nil {
		return stores.Card{}, fsrs.ReviewLog{}, err
	}
	return card, info.ReviewLog, nil
}

// ScoreCards gives the same score to several cards in one transaction.
// Cards that don't exist or were just reviewed are skipped and reported back.
func (s *Server) ScoreCards(ctx context.Context, req *connect.Request[pb.ScoreCardsRequest]) (
	*connect.Response[pb.ScoreCardsResponse], error) {
	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	if req.Msg.Score < 1 || req.Msg.Score > 4 {
		return nil, invalidArgError("invalid score")
	}
	if req.Msg.Lexicon == "" || len(req.Msg.Alphagrams) == 0 {
		return nil, invalidArgError("need a lexicon and at least one alphagram")
	}
	scores := make([]*pb.CardScore, len(req.Msg.Alphagrams))
	for i, a := range req.Msg.Alphagrams {
		scores[i] = &pb.CardScore{Alphagram: a, Score: req.Msg.Score}
	}
	scored, notScored, err := s.scoreCardsInTx(ctx, int64(user.DBID), req.Msg.Lexicon, scores)
	if err != nil {
		return nil, err
	}
	log.Ctx(ctx).Info().Str("lex", req.Msg.Lexicon).Int("score", int(req.Msg.Score)).
		Int("num-scored", len(scored)).Int("num-not-scored", len(notScored)).Msg("cards-scored")

	return connect.NewResponse(&pb.ScoreCardsResponse{
		NumScored: uint32(len(scored)),
		NotScored: notScored,
	}), nil
}

// BatchScoreCards scores several cards in one transaction, each with its own
// score. Cards that don't exist or were just reviewed are skipped and
// reported back.
func (s *Server) BatchScoreCards(ctx context.Context, req *connect.Request[pb.BatchScoreCardsRequest]) (
	*connect.Response[pb.BatchScoreCardsResponse], error) {
	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	if req.Msg.Lexicon == "" || len(req.Msg.Scores) == 0 {
		return nil, invalidArgError("need a lexicon and at least one score")
	}
	for _, sc := range req.Msg.Scores {
		if sc.Score < 1 || sc.Score > 4 {
			return nil, invalidArgError("invalid score")
		}
	}
	scored, notScored, err := s.scoreCardsInTx(ctx, int64(user.DBID), req.Msg.Lexicon, req.Msg.Scores)
	if err != nil {
		return nil, err
	}
	log.Ctx(ctx).Info().Str("lex", req.Msg.Lexicon).
		Int("num-scored", len(scored)).Int("num-not-scored", len(notScored)).Msg("cards-batch-scored")

	return connect.NewResponse(&pb.BatchScoreCardsResponse{
		Scored:    scored,
		NotScored: notScored,
	}), nil
}

// scoreCardsInTx scores the given cards in one transaction. It returns the
// cards it scored and the requested alphagrams it skipped, because there is
// no such card or the card was just reviewed. Alphagrams are reported back as
// they were given.
func (s *Server) scoreCardsInTx(ctx context.Context, userID int64, lexicon string,
	scores []*pb.CardScore) ([]*pb.ScoredCard, []string, error) {

	if len(scores) > s.Config.MaxCardsAdd {
		return nil, nil, invalidArgError(fmt.Sprintf("cannot score more than %d cards at a time", s.Config.MaxCardsAdd))
	}
	now := s.Nower.Now()

	cfg := &wglconfig.Config{DataPath: s.Config.DataPath}
	// The stored alphagram may be in lexicon order rather than as given, so
	// map both forms back to the request. A card requested more than once is
	// scored once, with its first score, and its repeats are not reported.
	reqIdx := map[string]int{}
	repeat := make([]bool, len(scores))
	alphagrams := make([]string, len(scores))
	for i, sc := range scores {
		alphagrams[i] = sc.Alphagram
		sorted := lexiconAlphagram(cfg, lexicon, sc.Alphagram)
		if _, ok := reqIdx[sorted]; ok {
			repeat[i] = true
			continue
		}
		reqIdx[sc.Alphagram] = i
		reqIdx[sorted] = i
	}

	maintenance, err := s.appMaintenance(ctx)
	if err != nil {
		return nil, nil, err
	}
	if maintenance {
		return nil, nil, ErrMaintenance
	}

	tx, err := s.DBPool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	rows, err := qtx.GetCards(ctx, models.GetCardsParams{
		UserID:      userID,
		LexiconName: lexicon,
		Alphagrams:  lookupAlphagrams(cfg, lexicon, alphagrams),
	})
	if err != nil {
		return nil, nil, err
	}

	// Cards in different decks can have different scheduling parameters.
	deckParams := map[int64]fsrs.Parameters{}
	due := make([]*time.Time, len(scores))
	for i := range rows {
		idx, ok := reqIdx[rows[i].Alphagram]
		if !ok || due[idx] != nil || justReviewed(rows[i].ReviewLog, now) {
			continue
		}
		params, ok := deckParams[rows[i].DeckID]
		if !ok {
			params, err = s.fsrsParamsForDeck(ctx, userID, rows[i].DeckID, qtx)
			if err != nil {
				return nil, nil, err
			}
			deckParams[rows[i].DeckID] = params
		}
		card, _, err := scoreAndUpdateCard(ctx, qtx, params, userID, lexicon,
			rows[i].Alphagram, rows[i].FsrsCard, fsrs.Rating(scores[idx].Score), now)
		if err != nil {
			return nil, nil, err
		}
		due[idx] = &card.Due
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, nil, err
	}

	scored := []*pb.ScoredCard{}
	notScored := []string{}
	for i, sc := range scores {
		if repeat[i] {
			continue
		}
		if due[i] == nil {
			notScored = append(notScored, sc.Alphagram)
			continue
		}
		scored = append(scored, &pb.ScoredCard{
			Alphagram:     sc.Alphagram,
			NextScheduled: timestamppb.New(*due[i]),
		})
	}
	return scored, notScored, nil
}

func (s *Server) EditLastScore(ctx context.Context, req *connect.Request[pb.EditLastScoreRequest]) (
	*connect.Response[pb.ScoreCardResponse], error) {
	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}

	now := s.Nower.Now()
	if req.Msg.NewScore < 1 || req.Msg.NewScore > 4 {
		return nil, invalidArgError("invalid score")
	}
	if req.Msg.Lexicon == "" || req.Msg.Alphagram == "" {
		return nil, invalidArgError("no such lexicon or alphagram")
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

	cardrow, err := qtx.GetCard(ctx, models.GetCardParams{
		UserID:      int64(user.DBID),
		LexiconName: req.Msg.Lexicon,
		Alphagram:   req.Msg.Alphagram})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrMaybeRefreshApp
		} else {
			return nil, err
		}
	}
	if len(cardrow.ReviewLog) == 0 {
		return nil, invalidArgError("this card has no review history")
	}

	// Load params for this card's deck
	params, err := s.fsrsParamsForDeck(ctx, int64(user.DBID), cardrow.DeckID, qtx)
	if err != nil {
		return nil, err
	}
	f := fsrs.NewFSRS(params)

	var card fsrs.Card
	lastLog := cardrow.ReviewLog[len(cardrow.ReviewLog)-1]
	if lastLog.ImportLog == nil && !lastLog.Due.IsZero() {
		// Review logs written by go-fsrs v4 record the card's pre-review
		// state, so we can roll back to it on the server.
		card, err = f.Rollback(cardrow.FsrsCard.Card, lastLog.ReviewLog)
		if err != nil {
			return nil, err
		}
	} else {
		// Older (go-fsrs v3) logs don't have the pre-review state; fall back
		// to the copy of the card the client kept from before the review.
		err = json.Unmarshal(req.Msg.LastCardRepr, &card)
		if err != nil {
			return nil, invalidArgError("last card was not properly provided")
		}
	}

	// And re-schedule the card.
	fixLegacyCard(&card)
	info, err := f.Next(card, now, fsrs.Rating(req.Msg.NewScore))
	if err != nil {
		return nil, err
	}
	card = info.Card
	newrlog := info.ReviewLog
	rlogbts, err := json.Marshal(newrlog)
	if err != nil {
		return nil, err
	}
	furtherFuzzDueDate(params, now, &card)
	// Overwrite last log with this new log.
	err = qtx.UpdateCardReplaceLastLog(ctx, models.UpdateCardReplaceLastLogParams{
		FsrsCard:      stores.Card{Card: card},
		NextScheduled: toPGTimestamp(card.Due),
		UserID:        int64(user.DBID),
		LexiconName:   req.Msg.Lexicon,
		Alphagram:     req.Msg.Alphagram,
		ReviewLogItem: rlogbts,
	})
	if err != nil {
		return nil, err
	}
	err = tx.Commit(ctx)
	if err != nil {
		return nil, err
	}
	cardJson, err := json.Marshal(card)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.ScoreCardResponse{
		NextScheduled: timestamppb.New(card.Due),
		CardJsonRepr:  cardJson,
	}), nil

}

func (s *Server) AddCards(ctx context.Context, req *connect.Request[pb.AddCardsRequest]) (
	*connect.Response[pb.AddCardsResponse], error) {
	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	alphagrams, err := s.resolveAlphagrams(ctx, req.Msg.Lexicon, req.Msg.Alphagrams, req.Msg.SearchParams)
	if err != nil {
		return nil, err
	}
	if len(alphagrams) == 0 {
		return nil, invalidArgError("need to add at least one card")
	}
	if len(alphagrams) > s.Config.MaxCardsAdd {
		if len(req.Msg.SearchParams) > 0 {
			return nil, invalidArgError(fmt.Sprintf("your search matched %d cards; cannot add more than %d cards at a time",
				len(alphagrams), s.Config.MaxCardsAdd))
		}
		return nil, invalidArgError(fmt.Sprintf("cannot add more than %d cards at a time", s.Config.MaxCardsAdd))
	}
	if len(req.Msg.SearchParams) == 0 {
		// Alphagrams from a search are already as the lexicon stores them.
		alphagrams, err = s.lexiconAlphagrams(ctx, req.Msg.Lexicon, alphagrams)
		if err != nil {
			return nil, err
		}
	}
	resp, err := s.addCards(ctx, user, req.Msg.Lexicon, alphagrams, req.Msg.DeckId)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(resp), nil
}

// addCards adds new cards for alphagrams, which must already be as the
// lexicon stores them, to a deck. Alphagrams already in the vault are left
// where they are.
func (s *Server) addCards(ctx context.Context, user *auth.AuthedUser, lexicon string,
	alphagrams []string, deckID uint64) (*pb.AddCardsResponse, error) {

	if !user.Member {
		rows, err := s.Queries.GetNumCardsInVault(ctx, int64(user.DBID))
		if err != nil {
			return nil, err
		}
		total := 0
		for i := range rows {
			total += int(rows[i].CardCount)
		}
		if total+len(alphagrams) > s.Config.MaxNonmemberCards {
			return nil, connect.NewError(connect.CodeInvalidArgument, ErrNeedMembership)
		}
	}

	// Just add the same card to all rows for ease for now.
	card := fsrs.NewCard()
	now := s.Nower.Now()

	card.Due = now
	cardbts, err := json.Marshal(card)
	if err != nil {
		return nil, err
	}

	// Randomize the alphagrams to avoid any bias in how they came in.
	if s.Config.SmallJitterOnAddCard {
		rand.Shuffle(len(alphagrams), func(i, j int) {
			alphagrams[i], alphagrams[j] = alphagrams[j], alphagrams[i]
		})
	}

	nextScheduleds := make([]pgtype.Timestamptz, len(alphagrams))
	cards := make([][]byte, len(alphagrams))
	for i := range alphagrams {
		// Add a little bit of "jitter" to the time to establish a deterministic ordering.
		if s.Config.SmallJitterOnAddCard {
			nextScheduleds[i] = toPGTimestamp(now.Add(time.Duration(i) * time.Millisecond))
		} else {
			nextScheduleds[i] = toPGTimestamp(now)
		}
		cards[i] = cardbts // This is by reference but it's ok after marshalling.
	}

	tx, err := s.DBPool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	countParams := models.CountCardsInOtherDecksParams{
		UserID:      int64(user.DBID),
		LexiconName: lexicon,
		Alphagrams:  alphagrams,
		DeckID:      int64(deckID),
	}
	numInOtherDeck, err := qtx.CountCardsInOtherDecks(ctx, countParams)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	var previewRows []*pb.CardPreview
	if numInOtherDeck > 0 {
		cardPreviewParams := models.GetCardsInOtherDecksParams{
			UserID:      int64(user.DBID),
			LexiconName: lexicon,
			Alphagrams:  alphagrams,
			DeckID:      int64(deckID),
			Limit:       CardInOtherDeckPreviewLimit,
		}
		rows, err := qtx.GetCardsInOtherDecks(ctx, cardPreviewParams)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		previewRows = make([]*pb.CardPreview, len(rows))
		for i := range rows {
			previewRows[i] = &pb.CardPreview{
				Lexicon:   lexicon,
				Alphagram: rows[i].Alphagram,
				DeckId:    uint64(rows[i].DeckID),
			}
		}
	}

	addParams := models.AddCardsParams{
		UserID:      int64(user.DBID),
		LexiconName: lexicon,
		// sqlc compiler can't detect this is a special type. It's ok.
		FsrsCards:      cards,
		Alphagrams:     alphagrams,
		NextScheduleds: nextScheduleds,
		DeckID:         int64(deckID),
	}
	numInserted, err := qtx.AddCards(ctx, addParams)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	err = tx.Commit(ctx)
	if err != nil {
		return nil, err
	}

	return &pb.AddCardsResponse{
		NumCardsAdded:            uint32(numInserted),
		NumCardsInOtherDecks:     uint32(numInOtherDeck),
		CardsInOtherDecksPreview: previewRows,
	}, nil
}

func (s *Server) MoveCards(ctx context.Context, req *connect.Request[pb.MoveCardsRequest]) (
	*connect.Response[pb.MoveCardsResponse], error) {
	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	if req.Msg.Lexicon == "" {
		return nil, invalidArgError("need a lexicon")
	}
	alphagrams, err := s.resolveAlphagrams(ctx, req.Msg.Lexicon, req.Msg.Alphagrams, req.Msg.SearchParams)
	if err != nil {
		return nil, err
	}
	if len(alphagrams) == 0 {
		return nil, invalidArgError("need at least one card to move")
	}

	var numMoved int64

	if !req.Msg.FromAllDecks {
		if req.Msg.SourceDeckId == req.Msg.TargetDeckId {
			return nil, invalidArgError("source and target deck cannot be the same")
		}
		params := models.MoveCardsFromDeckParams{
			UserID:       int64(user.DBID),
			LexiconName:  req.Msg.Lexicon,
			Alphagrams:   alphagrams,
			TargetDeckID: int64(req.Msg.TargetDeckId),
			SourceDeckID: int64(req.Msg.SourceDeckId),
		}
		numMoved, err = s.Queries.MoveCardsFromDeck(ctx, params)
	} else {
		if req.Msg.SourceDeckId != 0 {
			return nil, invalidArgError("cannot specify source_deck_id when from_all_decks is true")
		}
		params := models.MoveCardsParams{
			UserID:       int64(user.DBID),
			LexiconName:  req.Msg.Lexicon,
			Alphagrams:   alphagrams,
			TargetDeckID: int64(req.Msg.TargetDeckId),
		}
		numMoved, err = s.Queries.MoveCards(ctx, params)
	}

	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.MoveCardsResponse{NumCardsMoved: uint32(numMoved)}), nil
}

func (s *Server) GetCardCount(ctx context.Context, req *connect.Request[pb.GetCardCountRequest]) (
	*connect.Response[pb.CardCountResponse], error) {
	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	rows, err := s.Queries.GetNumCardsInVault(ctx, int64(user.DBID))
	if err != nil {
		return nil, err
	}
	cardCount := map[string]uint32{}

	total := uint32(0)
	for i := range rows {
		total += uint32(rows[i].CardCount)
		cardCount[rows[i].LexiconName] = uint32(rows[i].CardCount)
	}

	return connect.NewResponse(&pb.CardCountResponse{
		NumCards:   cardCount,
		TotalCards: total,
	}), nil
}

func (s *Server) GetCardCountByDeck(ctx context.Context, req *connect.Request[pb.GetCardCountByDeckRequest]) (
	*connect.Response[pb.GetCardCountByDeckResponse], error) {
	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	if req.Msg.Lexicon == "" {
		return nil, invalidArgError("must provide a lexicon")
	}

	rows, err := s.Queries.GetNumCardsInVaultByDeck(ctx, models.GetNumCardsInVaultByDeckParams{
		UserID:      int64(user.DBID),
		LexiconName: req.Msg.Lexicon,
	})
	if err != nil {
		return nil, err
	}

	items := make([]*pb.DeckCardCount, len(rows))
	for i := range rows {
		items[i] = &pb.DeckCardCount{
			DeckId: uint64(rows[i].DeckID),
			Count:  uint32(rows[i].CardCount),
		}
	}
	return connect.NewResponse(&pb.GetCardCountByDeckResponse{Items: items}), nil
}

func (s *Server) NextScheduledCount(ctx context.Context, req *connect.Request[pb.NextScheduledCountRequest]) (
	*connect.Response[pb.NextScheduledBreakdown], error) {
	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	log := log.Ctx(ctx)
	breakdown := map[string]uint32{}
	log.Info().Interface("req", req.Msg).Msg("next-scheduled-count")

	if req.Msg.Lexicon == "" {
		return nil, invalidArgError("must provide a lexicon")
	}

	if req.Msg.OnlyOverdue {
		ocCount, err := s.Queries.GetOverdueCount(ctx, models.GetOverdueCountParams{
			UserID:      int64(user.DBID),
			Now:         toPGTimestamp(s.Nower.Now()),
			LexiconName: req.Msg.Lexicon,
		})
		if err != nil {
			return nil, err
		}
		breakdown["overdue"] = uint32(ocCount)
	} else {
		tz := "UTC"
		if req.Msg.Timezone != "" {
			tz = req.Msg.Timezone
		}
		rows, err := s.Queries.GetNextScheduledBreakdown(ctx, models.GetNextScheduledBreakdownParams{
			UserID:      int64(user.DBID),
			Now:         toPGTimestamp(s.Nower.Now()),
			Tz:          tz,
			LexiconName: req.Msg.Lexicon,
		})
		if err != nil {
			return nil, err
		}
		for i := range rows {
			var s string
			switch rows[i].ScheduledDate.InfinityModifier {
			case pgtype.Finite:
				s = rows[i].ScheduledDate.Time.Format("2006-01-02")
			case pgtype.Infinity:
				s = "infinity"
			case pgtype.NegativeInfinity:
				s = "overdue"
			}
			breakdown[s] = uint32(rows[i].QuestionCount)
		}
	}

	return connect.NewResponse(&pb.NextScheduledBreakdown{Breakdown: breakdown}), nil
}

func (s *Server) NextScheduledCountByDeck(ctx context.Context, req *connect.Request[pb.NextScheduledCountByDeckRequest]) (
	*connect.Response[pb.NextScheduledCountByDeckResponse], error) {
	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	log := log.Ctx(ctx)
	log.Info().Interface("req", req.Msg).Msg("next-scheduled-count")

	if req.Msg.Lexicon == "" {
		return nil, invalidArgError("must provide a lexicon")
	}
	var breakdowns []*pb.DeckBreakdown

	if req.Msg.OnlyOverdue {
		ocCounts, err := s.Queries.GetOverdueCountByDeck(ctx, models.GetOverdueCountByDeckParams{
			UserID:      int64(user.DBID),
			Now:         toPGTimestamp(s.Nower.Now()),
			LexiconName: req.Msg.Lexicon,
		})
		if err != nil {
			return nil, err
		}

		for _, ocCount := range ocCounts {
			breakdown := map[string]uint32{}
			breakdown["overdue"] = uint32(ocCount.Count)

			breakdowns = append(breakdowns, &pb.DeckBreakdown{
				DeckId:    uint64(ocCount.DeckID),
				Breakdown: breakdown,
			})
		}
	} else {
		tz := "UTC"
		if req.Msg.Timezone != "" {
			tz = req.Msg.Timezone
		}
		rows, err := s.Queries.GetNextScheduledBreakdownByDeck(ctx, models.GetNextScheduledBreakdownByDeckParams{
			UserID:      int64(user.DBID),
			Now:         toPGTimestamp(s.Nower.Now()),
			Tz:          tz,
			LexiconName: req.Msg.Lexicon,
		})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			m := map[string]uint32{}
			if err := json.Unmarshal(row.Breakdown, &m); err != nil {
				return nil, err
			}
			breakdowns = append(breakdowns, &pb.DeckBreakdown{
				DeckId:    uint64(row.DeckID),
				Breakdown: m,
			})
		}
	}

	return connect.NewResponse(&pb.NextScheduledCountByDeckResponse{Breakdowns: breakdowns}), nil
}

type postponement struct {
	alphagram                string
	card                     *fsrs.Card
	forgettingCurve          float64
	elapsedDaysAfterPostpone float64
}

func forgettingCurve(elapsedDays, stability, factor, decay float64) float64 {
	return math.Pow(1+factor*elapsedDays/stability, decay)
}

func (s *Server) Postpone(ctx context.Context, req *connect.Request[pb.PostponeRequest]) (
	*connect.Response[pb.PostponeResponse], error) {

	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	if req.Msg.NumToPostpone == 0 {
		return nil, invalidArgError("need at least one card to postpone")
	}
	log := log.Ctx(ctx)

	params, err := s.fsrsParams(ctx, int64(user.DBID), nil)
	if err != nil {
		return nil, err
	}
	desiredRetention := params.RequestRetention

	tx, err := s.DBPool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	duecards, err := qtx.PostponementQuery(ctx, models.PostponementQueryParams{
		UserID:        int64(user.DBID),
		LexiconName:   req.Msg.Lexicon,
		NextScheduled: toPGTimestamp(s.Nower.Now()),
	})
	if err != nil {
		return nil, err
	}
	if len(duecards) == 0 {
		return nil, invalidArgError("there are no cards to postpone")
	}

	now := s.Nower.Now()
	log.Info().Int("ncards", len(duecards)).Msg("potential-cards-to-postpone")
	postponements := make([]postponement, len(duecards))
	for i := range duecards {
		card := &duecards[i].FsrsCard
		postponements[i].card = &card.Card
		postponements[i].alphagram = duecards[i].Alphagram
		ivl := card.ScheduledDays

		elapsedDays := now.Sub(card.LastReview).Hours() / 24.0
		postponements[i].elapsedDaysAfterPostpone = elapsedDays + float64(ivl)*0.075
		postponements[i].forgettingCurve = forgettingCurve(
			max(postponements[i].elapsedDaysAfterPostpone, 0), card.Stability,
			params.Factor, params.Decay)
	}

	sort.Slice(postponements, func(i, j int) bool {
		forgettingOddsIncreaseI := (1/postponements[i].forgettingCurve-1)/(1/desiredRetention-1) - 1
		forgettingOddsIncreaseJ := (1/postponements[j].forgettingCurve-1)/(1/desiredRetention-1) - 1
		if forgettingOddsIncreaseI == forgettingOddsIncreaseJ {
			// Favor postponing cards with higher stability first.
			return postponements[i].card.Stability > postponements[j].card.Stability
		}
		return forgettingOddsIncreaseI < forgettingOddsIncreaseJ
	})

	var cnt uint32
	for i := range postponements {
		if cnt >= req.Msg.NumToPostpone {
			break
		}
		ivl := postponements[i].card.ScheduledDays
		elapsedDays := now.Sub(postponements[i].card.LastReview).Hours() / 24
		delay := elapsedDays - float64(ivl)
		newIvl := min(
			max(1, math.Ceil(float64(ivl)*(1.05+0.05*rand.Float64()))+delay), params.MaximumInterval,
		)
		// card := updateCardDue(postponements[i].card, newIvl)
		postponements[i].card.ScheduledDays = uint64(newIvl)
		newIvlDuration := time.Duration(newIvl * 24.0 * float64(time.Hour))
		postponements[i].card.Due = postponements[i].card.LastReview.Add(newIvlDuration)
		cnt++
	}

	alphagrams := make([]string, cnt)
	nextScheduleds := make([]pgtype.Timestamptz, cnt)
	cards := make([][]byte, cnt)
	for i := range cnt {
		alphagrams[i] = postponements[i].alphagram
		nextScheduleds[i] = toPGTimestamp(postponements[i].card.Due)
		cards[i], err = json.Marshal(postponements[i].card)
		if err != nil {
			return nil, err
		}
	}

	// then update all the postponed cards in the db.
	err = qtx.BulkUpdateCards(ctx, models.BulkUpdateCardsParams{
		Alphagrams:     alphagrams,
		NextScheduleds: nextScheduleds,
		FsrsCards:      cards,
		UserID:         int64(user.DBID),
		LexiconName:    req.Msg.Lexicon,
	})
	if err != nil {
		return nil, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&pb.PostponeResponse{NumPostponed: cnt}), nil
}

func validateDeleteParams(onlyNewQuestions bool, onlyAlphagrams []string, allQuestions bool,
	searchParams []*searchpb.SearchRequest_SearchParam) error {
	if len(searchParams) > 0 {
		if allQuestions || onlyNewQuestions || len(onlyAlphagrams) > 0 {
			return invalidArgError("search_params cannot be combined with only_alphagrams, only_new_questions or all_questions")
		}
	} else if allQuestions {
		if len(onlyAlphagrams) > 0 || onlyNewQuestions {
			return invalidArgError("all_questions cannot be combined with only_alphagrams or only_new_questions")
		}
	} else if onlyNewQuestions {
		if len(onlyAlphagrams) > 0 {
			return invalidArgError("only_new_questions cannot be combined with only_alphagrams")
		}
	} else {
		if len(onlyAlphagrams) == 0 {
			return invalidArgError("must specify one of: only_new_questions, only_alphagrams, search_params, or all_questions")
		}
	}
	return nil
}

func (s *Server) Delete(ctx context.Context, req *connect.Request[pb.DeleteRequest]) (
	*connect.Response[pb.DeleteResponse], error) {

	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}

	if req.Msg.Lexicon == "" {
		return nil, invalidArgError("need a lexicon")
	}
	if err := validateDeleteParams(req.Msg.OnlyNewQuestions, req.Msg.OnlyAlphagrams, req.Msg.AllQuestions, req.Msg.SearchParams); err != nil {
		return nil, err
	}
	onlyAlphagrams, err := s.resolveAlphagrams(ctx, req.Msg.Lexicon, req.Msg.OnlyAlphagrams, req.Msg.SearchParams)
	if err != nil {
		return nil, err
	}
	var deletedRows int64
	if req.Msg.OnlyNewQuestions {
		deletedRows, err = s.Queries.DeleteNewCards(ctx, models.DeleteNewCardsParams{
			UserID:      int64(user.DBID),
			LexiconName: req.Msg.Lexicon,
		})
	} else if req.Msg.AllQuestions {
		// delete them all!
		deletedRows, err = s.Queries.DeleteCards(ctx, models.DeleteCardsParams{
			UserID: int64(user.DBID), LexiconName: req.Msg.Lexicon,
		})
	} else if len(onlyAlphagrams) > 0 {
		deletedRows, err = s.Queries.DeleteCardsWithAlphagrams(ctx, models.DeleteCardsWithAlphagramsParams{
			UserID:      int64(user.DBID),
			LexiconName: req.Msg.Lexicon,
			Alphagrams:  onlyAlphagrams,
		})
	} else {
		return nil, invalidArgError("invalid parameters for delete request")
	}
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.DeleteResponse{NumDeleted: uint32(deletedRows)}), nil
}

func (s *Server) DeleteFromDeck(ctx context.Context, req *connect.Request[pb.DeleteFromDeckRequest]) (
	*connect.Response[pb.DeleteResponse], error) {

	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}

	if req.Msg.Lexicon == "" {
		return nil, invalidArgError("need a lexicon")
	}
	if err := validateDeleteParams(req.Msg.OnlyNewQuestions, req.Msg.OnlyAlphagrams, req.Msg.AllQuestions, req.Msg.SearchParams); err != nil {
		return nil, err
	}
	onlyAlphagrams, err := s.resolveAlphagrams(ctx, req.Msg.Lexicon, req.Msg.OnlyAlphagrams, req.Msg.SearchParams)
	if err != nil {
		return nil, err
	}

	var deletedRows int64
	deckID := int64(req.Msg.DeckId)

	if req.Msg.OnlyNewQuestions {
		deletedRows, err = s.Queries.DeleteNewCardsFromDeck(ctx, models.DeleteNewCardsFromDeckParams{
			UserID:      int64(user.DBID),
			LexiconName: req.Msg.Lexicon,
			DeckID:      deckID,
		})
	} else if req.Msg.AllQuestions {
		deletedRows, err = s.Queries.DeleteCardsFromDeck(ctx, models.DeleteCardsFromDeckParams{
			UserID:      int64(user.DBID),
			LexiconName: req.Msg.Lexicon,
			DeckID:      deckID,
		})
	} else if len(onlyAlphagrams) > 0 {
		deletedRows, err = s.Queries.DeleteCardsWithAlphagramsFromDeck(ctx, models.DeleteCardsWithAlphagramsFromDeckParams{
			UserID:      int64(user.DBID),
			LexiconName: req.Msg.Lexicon,
			Alphagrams:  onlyAlphagrams,
			DeckID:      deckID,
		})
	}
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.DeleteResponse{NumDeleted: uint32(deletedRows)}), nil
}

// lastReviewDayBounds returns the start and end (exclusive) of the day that
// contains now in the given timezone, in UTC and formatted to compare as text
// against the LastReview timestamps stored in fsrs_card.
func lastReviewDayBounds(now time.Time, timezone string) (string, string, error) {
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return "", "", invalidArgError("invalid timezone: " + timezone)
	}
	y, m, d := now.In(loc).Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, loc)
	end := start.AddDate(0, 0, 1)
	const layout = "2006-01-02T15:04:05"
	return start.UTC().Format(layout), end.UTC().Format(layout), nil
}

func (s *Server) GetDailyProgress(ctx context.Context, req *connect.Request[pb.GetDailyProgressRequest]) (
	*connect.Response[pb.GetDailyProgressResponse], error) {

	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}

	dayStart, dayEnd, err := lastReviewDayBounds(s.Nower.Now(), req.Msg.Timezone)
	if err != nil {
		return nil, err
	}
	progress, err := s.Queries.GetDailyProgress(ctx, models.GetDailyProgressParams{
		UserID:   int64(user.DBID),
		DayStart: dayStart,
		DayEnd:   dayEnd,
	})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.GetDailyProgressResponse{
		ProgressStats: map[string]int32{
			"New":            int32(progress.NewCards),
			"Reviewed":       int32(progress.ReviewedCards),
			"NewMissed":      int32(progress.NewRating1),
			"NewHard":        int32(progress.NewRating2),
			"NewGood":        int32(progress.NewRating3),
			"NewEasy":        int32(progress.NewRating4),
			"ReviewedMissed": int32(progress.ReviewedRating1),
			"ReviewedHard":   int32(progress.ReviewedRating2),
			"ReviewedGood":   int32(progress.ReviewedRating3),
			"ReviewedEasy":   int32(progress.ReviewedRating4),
			"Repeats":        int32(progress.Repeats),
		},
	}), nil
}

func (s *Server) GetDailyProgressByDeck(ctx context.Context, req *connect.Request[pb.GetDailyProgressByDeckRequest]) (
	*connect.Response[pb.GetDailyProgressByDeckResponse], error) {

	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}

	dayStart, dayEnd, err := lastReviewDayBounds(s.Nower.Now(), req.Msg.Timezone)
	if err != nil {
		return nil, err
	}
	rows, err := s.Queries.GetDailyProgressByDeck(ctx, models.GetDailyProgressByDeckParams{
		UserID:   int64(user.DBID),
		DayStart: dayStart,
		DayEnd:   dayEnd,
	})
	if err != nil {
		return nil, err
	}

	items := make([]*pb.DailyProgressByDeckItem, len(rows))
	for i := range rows {
		var deckID *wrapperspb.Int64Value
		if rows[i].DeckID.Valid {
			deckID = wrapperspb.Int64(rows[i].DeckID.Int64)
		}
		items[i] = &pb.DailyProgressByDeckItem{
			DeckId: deckID,
			ProgressStats: map[string]int32{
				"New":            int32(rows[i].NewCards),
				"Reviewed":       int32(rows[i].ReviewedCards),
				"NewMissed":      int32(rows[i].NewRating1),
				"NewHard":        int32(rows[i].NewRating2),
				"NewGood":        int32(rows[i].NewRating3),
				"NewEasy":        int32(rows[i].NewRating4),
				"ReviewedMissed": int32(rows[i].ReviewedRating1),
				"ReviewedHard":   int32(rows[i].ReviewedRating2),
				"ReviewedGood":   int32(rows[i].ReviewedRating3),
				"ReviewedEasy":   int32(rows[i].ReviewedRating4),
				"Repeats":        int32(rows[i].Repeats),
			},
		}
	}

	return connect.NewResponse(&pb.GetDailyProgressByDeckResponse{Items: items}), nil
}

func (s *Server) GetDailyLeaderboard(ctx context.Context, req *connect.Request[pb.GetDailyLeaderboardRequest]) (
	*connect.Response[pb.GetDailyLeaderboardResponse], error) {

	// interceptor still requires login, but we don't check it here  🤔

	leaderboardRows, err := s.Queries.GetDailyLeaderboard(ctx, req.Msg.Timezone)
	if err != nil {
		return nil, err
	}

	resp := &pb.GetDailyLeaderboardResponse{
		Items: make([]*pb.GetDailyLeaderboardResponse_LeaderboardItem, len(leaderboardRows)),
	}

	for i := range leaderboardRows {
		resp.Items[i] = &pb.GetDailyLeaderboardResponse_LeaderboardItem{
			User:         leaderboardRows[i].Username.String,
			CardsStudied: int32(leaderboardRows[i].CardsStudiedToday),
		}
	}
	return connect.NewResponse(resp), nil
}

func (s *Server) GetFsrsParameters(ctx context.Context, req *connect.Request[pb.GetFsrsParametersRequest]) (
	*connect.Response[pb.GetFsrsParametersResponse], error) {
	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}

	dbparams, version, err := s.fsrsParamsAndVersion(ctx, int64(user.DBID), nil)
	if err != nil {
		return nil, err
	}

	params := &pb.FsrsParameters{
		Scheduler:        pb.FsrsScheduler_FSRS_SCHEDULER_LONG_TERM,
		RequestRetention: dbparams.RequestRetention,
	}

	if dbparams.EnableShortTerm {
		params.Scheduler = pb.FsrsScheduler_FSRS_SCHEDULER_SHORT_TERM
	}

	resp := &pb.GetFsrsParametersResponse{
		Parameters:  params,
		FsrsVersion: int32(version),
	}

	return connect.NewResponse(resp), nil
}

func (s *Server) EditFsrsParameters(ctx context.Context, req *connect.Request[pb.EditFsrsParametersRequest]) (
	*connect.Response[pb.EditFsrsParametersResponse], error) {
	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}

	if req.Msg.Parameters.RequestRetention < 0.7 || req.Msg.Parameters.RequestRetention > 0.97 {
		return nil, invalidArgError("invalid retention value")
	}

	if req.Msg.Parameters.Scheduler != pb.FsrsScheduler_FSRS_SCHEDULER_SHORT_TERM && req.Msg.Parameters.Scheduler != pb.FsrsScheduler_FSRS_SCHEDULER_LONG_TERM {
		return nil, invalidArgError("invalid scheduler value")
	}

	tx, err := s.DBPool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	params, err := s.fsrsParams(ctx, int64(user.DBID), qtx)
	if err != nil {
		return nil, err
	}

	if req.Msg.Parameters.Scheduler == pb.FsrsScheduler_FSRS_SCHEDULER_SHORT_TERM {
		params.EnableShortTerm = true
	} else {
		params.EnableShortTerm = false
	}
	params.RequestRetention = req.Msg.Parameters.RequestRetention

	err = qtx.SetFsrsParams(ctx, models.SetFsrsParamsParams{
		Params: params,
		UserID: int64(user.DBID),
	})
	if err != nil {
		return nil, err
	}

	err = tx.Commit(ctx)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.EditFsrsParametersResponse{}), nil
}

func (s *Server) AddDeck(ctx context.Context, req *connect.Request[pb.AddDeckRequest]) (
	*connect.Response[pb.AddDeckResponse], error) {

	user := auth.UserFromContext(ctx)
	deckName := strings.TrimSpace(req.Msg.Name)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	if req.Msg.Lexicon == "" {
		return nil, invalidArgError("need a lexicon")
	}
	if deckName == "" {
		return nil, invalidArgError("need a name")
	}

	deck, err := s.Queries.AddDeck(ctx, models.AddDeckParams{
		UserID:      int64(user.DBID),
		LexiconName: req.Msg.Lexicon,
		Name:        deckName,
	})

	if err != nil {
		var pgErr *pgconn.PgError
		// unique_violation error code: https://www.postgresql.org/docs/14/errcodes-appendix.html
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, invalidArgError("deck with this name already exists")
		}
		return nil, err
	}

	return connect.NewResponse(&pb.AddDeckResponse{
		Deck: &pb.Deck{
			Id:      deck.ID,
			Name:    deckName,
			Lexicon: deck.LexiconName,
		}}), nil
}

func (s *Server) GetDecks(ctx context.Context, req *connect.Request[pb.GetDecksRequest]) (
	*connect.Response[pb.GetDecksResponse], error) {

	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}

	decks, err := s.Queries.GetDecks(ctx, int64(user.DBID))

	if err != nil {
		return nil, err
	}

	resp := &pb.GetDecksResponse{
		Decks: make([]*pb.Deck, len(decks)),
	}

	for i := range decks {
		resp.Decks[i] = &pb.Deck{
			Id:      decks[i].ID,
			Name:    decks[i].Name,
			Lexicon: decks[i].LexiconName,
		}
		// Only populate FsrsParametersOverride if the deck has custom settings
		if decks[i].FsrsParamsOverride != nil {
			var params fsrs.Parameters
			if err := json.Unmarshal(decks[i].FsrsParamsOverride, &params); err != nil {
				return nil, err
			}
			resp.Decks[i].FsrsParametersOverride = &pb.FsrsParameters{
				Scheduler:        pb.FsrsScheduler_FSRS_SCHEDULER_LONG_TERM,
				RequestRetention: params.RequestRetention,
			}
			if params.EnableShortTerm {
				resp.Decks[i].FsrsParametersOverride.Scheduler = pb.FsrsScheduler_FSRS_SCHEDULER_SHORT_TERM
			}
		}
	}

	return connect.NewResponse(resp), nil
}

func (s *Server) EditDeck(ctx context.Context, req *connect.Request[pb.EditDeckRequest]) (
	*connect.Response[pb.EditDeckResponse], error) {

	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}

	if req.Msg.Id == 0 {
		return nil, invalidArgError("need a deck")
	}

	if req.Msg.Name == "" {
		return nil, invalidArgError("need a name")
	}

	// Handle FSRS parameters - nil means clear the override
	var fsrsParamsOverride []byte
	if req.Msg.FsrsParametersOverride != nil {
		// Validate the parameters
		if req.Msg.FsrsParametersOverride.RequestRetention < 0.7 || req.Msg.FsrsParametersOverride.RequestRetention > 0.97 {
			return nil, invalidArgError("invalid retention value")
		}
		if req.Msg.FsrsParametersOverride.Scheduler != pb.FsrsScheduler_FSRS_SCHEDULER_SHORT_TERM &&
			req.Msg.FsrsParametersOverride.Scheduler != pb.FsrsScheduler_FSRS_SCHEDULER_LONG_TERM {
			return nil, invalidArgError("invalid scheduler value")
		}

		var err error
		fsrsParamsOverride, err = json.Marshal(deckOverride{
			RequestRetention: req.Msg.FsrsParametersOverride.RequestRetention,
			EnableShortTerm:  req.Msg.FsrsParametersOverride.Scheduler == pb.FsrsScheduler_FSRS_SCHEDULER_SHORT_TERM,
		})
		if err != nil {
			return nil, err
		}
	}

	deck, err := s.Queries.EditDeck(ctx, models.EditDeckParams{
		ID:                 req.Msg.Id,
		Name:               req.Msg.Name,
		FsrsParamsOverride: fsrsParamsOverride,
		// We provide user ID just to stop users from spoofing
		// the ID of another deck that they don't own.
		UserID: int64(user.DBID),
	})

	if err != nil {
		return nil, err
	}

	resp := &pb.EditDeckResponse{
		Deck: &pb.Deck{
			Id:      deck.ID,
			Name:    deck.Name,
			Lexicon: deck.LexiconName,
		},
	}

	// Populate FsrsParametersOverride in response if the deck has custom settings
	if deck.FsrsParamsOverride != nil {
		var params fsrs.Parameters
		if err := json.Unmarshal(deck.FsrsParamsOverride, &params); err != nil {
			return nil, err
		}
		resp.Deck.FsrsParametersOverride = &pb.FsrsParameters{
			Scheduler:        pb.FsrsScheduler_FSRS_SCHEDULER_LONG_TERM,
			RequestRetention: params.RequestRetention,
		}
		if params.EnableShortTerm {
			resp.Deck.FsrsParametersOverride.Scheduler = pb.FsrsScheduler_FSRS_SCHEDULER_SHORT_TERM
		}
	}

	return connect.NewResponse(resp), nil
}

func (s *Server) DeleteDeck(ctx context.Context, req *connect.Request[pb.DeleteDeckRequest]) (
	*connect.Response[pb.DeleteDeckResponse], error) {

	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}

	if req.Msg.Id == 0 {
		return nil, invalidArgError("need a deck")
	}

	cardCount, err := s.Queries.CountCardsInDeck(ctx, models.CountCardsInDeckParams{
		DeckID: req.Msg.Id,
		UserID: int64(user.DBID),
	})
	if err != nil {
		return nil, err
	}

	if cardCount > 0 {
		return nil, invalidArgError("cannot delete deck with cards in it")
	}

	err = s.Queries.DeleteDeck(ctx, models.DeleteDeckParams{
		ID:     req.Msg.Id,
		UserID: int64(user.DBID),
	})
	if err != nil {
		var pgErr *pgconn.PgError
		// foreign_key_violation: a study plan still adds to this deck.
		if errors.As(err, &pgErr) && pgErr.Code == "23503" {
			return nil, invalidArgError("cannot delete a deck with a study plan; delete the plan first")
		}
		return nil, err
	}

	return connect.NewResponse(&pb.DeleteDeckResponse{}), nil
}

// The fsrs library fuzzes only by day, and not at all for intervals under 2.5
// days. It tends to ask questions at the same hour and minute that they were
// asked last, so cards studied together keep coming back together. Add a
// little more randomness to cards scheduled days out. Learning and relearning
// steps (minutes, in short-term mode) are left alone.
func furtherFuzzDueDate(params fsrs.Parameters, now time.Time, card *fsrs.Card) {
	if !params.EnableFuzz || card.State != fsrs.Review {
		return
	}
	// Find a random second in a 21,600-second interval (6 hours) centered
	// around the due date.
	fuzzFactor := 21600 // 6 hours

	if card.Due.Sub(now) > (time.Hour * 720) {
		// Fuzz by 24 hours
		fuzzFactor = 86400
	}

	d := int64(rand.Int32N(int32(fuzzFactor)))
	d -= (int64(fuzzFactor) / 2)

	card.Due = card.Due.Add(time.Duration(d) * time.Second)
}
