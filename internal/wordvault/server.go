package wordvault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"sort"
	"time"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/open-spaced-repetition/go-fsrs/v3"
	"github.com/rs/zerolog/log"
	"google.golang.org/protobuf/types/known/timestamppb"

	searchpb "github.com/domino14/word_db_server/api/rpc/wordsearcher"
	pb "github.com/domino14/word_db_server/api/rpc/wordvault"
	"github.com/domino14/word_db_server/config"
	"github.com/domino14/word_db_server/internal/auth"
	"github.com/domino14/word_db_server/internal/searchserver"
	"github.com/domino14/word_db_server/internal/stores/models"
)

var ErrNeedMembership = errors.New("adding these cards would put you over your limit; please upgrade your account to add more cards <3")

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

func (s *Server) GetCardInformation(ctx context.Context, req *connect.Request[pb.GetCardInfoRequest]) (
	*connect.Response[pb.Cards], error) {

	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}

	// Load from user params
	params, err := s.fsrsParams(ctx, int64(user.DBID))
	if err != nil {
		return nil, err
	}
	f := fsrs.NewFSRS(params) // cache this later!

	rows, err := s.Queries.GetCards(ctx, models.GetCardsParams{
		UserID:      int64(user.DBID),
		LexiconName: req.Msg.Lexicon,
		Alphagrams:  req.Msg.Alphagrams,
	})
	if err != nil {
		return nil, err
	}

	cards := make([]*pb.Card, len(rows))
	for i := range rows {
		fcard := rows[i].FsrsCard
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
			Alphagram:      &searchpb.Alphagram{Alphagram: req.Msg.Alphagrams[i]},
			CardJsonRepr:   cardbts,
			Retrievability: f.GetRetrievability(fcard, s.Nower.Now()),
			ReviewLog:      revlogbts,
		}
	}
	return connect.NewResponse(&pb.Cards{Cards: cards}), nil
}

func (s *Server) GetNextScheduled(ctx context.Context, req *connect.Request[pb.GetNextScheduledRequest]) (
	*connect.Response[pb.Cards], error) {
	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	rows, err := s.Queries.GetNextScheduled(ctx, models.GetNextScheduledParams{
		UserID:        int64(user.DBID),
		LexiconName:   req.Msg.Lexicon,
		Limit:         int32(req.Msg.Limit),
		NextScheduled: toPGTimestamp(s.Nower.Now()),
	})
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
	row, err := s.Queries.GetSingleNextScheduled(ctx, models.GetSingleNextScheduledParams{
		UserID:        int64(user.DBID),
		LexiconName:   req.Msg.Lexicon,
		NextScheduled: toPGTimestamp(s.Nower.Now()),
	})
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

	resp := &pb.GetSingleNextScheduledResponse{
		Card: &pb.Card{
			Lexicon:   req.Msg.Lexicon,
			Alphagram: expandResponse.Msg.Alphagrams[0],
			// sqlc can't detect that row.FsrsCard is of type fsrs.Card
			// because of the way the query is written, so we just pass
			// the raw bytes as they are.
			CardJsonRepr: row.FsrsCard,
		},
		OverdueCount: uint32(row.TotalCount),
	}

	return connect.NewResponse(resp), nil
}

func (s *Server) fsrsParams(ctx context.Context, dbid int64) (fsrs.Parameters, error) {
	// Load from user params
	params, err := s.Queries.LoadParams(ctx, dbid)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No params exist for this user
			log.Debug().Int64("userID", dbid).Msg("no-params-found")
			params = fsrs.DefaultParam()
			params.EnableShortTerm = false
			params.EnableFuzz = true
			params.MaximumInterval = 365 * 5 // Default is 100 years, which is a bit optimistic
		} else {
			return fsrs.Parameters{}, err
		}
	}
	return params, nil
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

	tx, err := s.DBPool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	params, err := s.fsrsParams(ctx, int64(user.DBID))
	if err != nil {
		return nil, err
	}

	f := fsrs.NewFSRS(params) // cache this later!

	cardrow, err := qtx.GetCard(ctx, models.GetCardParams{
		UserID:      int64(user.DBID),
		LexiconName: req.Msg.Lexicon,
		Alphagram:   req.Msg.Alphagram})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, invalidArgError("card with your input parameters was not found")
		} else {
			return nil, err
		}
	}
	card := cardrow.FsrsCard
	revlog := cardrow.ReviewLog
	if len(revlog) > 0 && s.Nower.Now().Sub(revlog[len(revlog)-1].Review) < JustReviewedInterval {
		return nil, invalidArgError("this card was just reviewed")
	}

	// It seems from reading the code that card.ElapsedDays gets updated by
	// the below function, so it doesn't need to be recalculated upon
	// db load. However, the db version of this variable is useless. It
	// should be a local variable and not stored in the db.
	schedulingCards := f.Repeat(card, now)
	rating := fsrs.Rating(req.Msg.Score)
	card = schedulingCards[rating].Card
	rlog := schedulingCards[rating].ReviewLog
	rlogbts, err := json.Marshal(rlog)
	if err != nil {
		return nil, err
	}
	furtherFuzzDueDate(params, now, &card)
	err = qtx.UpdateCard(ctx, models.UpdateCardParams{
		FsrsCard:      card,
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

	tx, err := s.DBPool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	qtx := s.Queries.WithTx(tx)

	// Load from user params
	params, err := s.fsrsParams(ctx, int64(user.DBID))
	if err != nil {
		return nil, err
	}
	f := fsrs.NewFSRS(params) // cache this later!

	cardrow, err := qtx.GetCard(ctx, models.GetCardParams{
		UserID:      int64(user.DBID),
		LexiconName: req.Msg.Lexicon,
		Alphagram:   req.Msg.Alphagram})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, invalidArgError("card with your input parameters was not found")
		} else {
			return nil, err
		}
	}
	if len(cardrow.ReviewLog) == 0 {
		return nil, invalidArgError("this card has no review history")
	}

	card := fsrs.Card{}
	err = json.Unmarshal(req.Msg.LastCardRepr, &card)
	if err != nil {
		return nil, invalidArgError("last card was not properly provided")
	}

	// And re-schedule the card.
	schedulingCards := f.Repeat(card, now)
	rating := fsrs.Rating(req.Msg.NewScore)
	card = schedulingCards[rating].Card
	newrlog := schedulingCards[rating].ReviewLog
	rlogbts, err := json.Marshal(newrlog)
	if err != nil {
		return nil, err
	}
	furtherFuzzDueDate(params, now, &card)
	// Overwrite last log with this new log.
	err = qtx.UpdateCardReplaceLastLog(ctx, models.UpdateCardReplaceLastLogParams{
		FsrsCard:      card,
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
	if len(req.Msg.Alphagrams) == 0 {
		return nil, invalidArgError("need to add at least one card")
	}
	if len(req.Msg.Alphagrams) > s.Config.MaxCardsAdd {
		return nil, invalidArgError(fmt.Sprintf("cannot add more than %d cards at a time", s.Config.MaxCardsAdd))
	}
	if !user.Member {
		rows, err := s.Queries.GetNumCardsInVault(ctx, int64(user.DBID))
		if err != nil {
			return nil, err
		}
		total := 0
		for i := range rows {
			total += int(rows[i].CardCount)
		}
		if total+len(req.Msg.Alphagrams) > s.Config.MaxNonmemberCards {
			return nil, connect.NewError(connect.CodeInvalidArgument, ErrNeedMembership)
		}
	}
	// if len(req.Msg.Alphagrams)

	// Just add the same card to all rows for ease for now.
	card := fsrs.NewCard()
	now := s.Nower.Now()

	card.Due = now
	alphagrams := req.Msg.Alphagrams
	nextScheduleds := make([]pgtype.Timestamptz, len(alphagrams))
	for i := range alphagrams {
		nextScheduleds[i] = toPGTimestamp(now)
	}
	bts, err := json.Marshal(card)
	if err != nil {
		return nil, err
	}

	numInserted, err := s.Queries.AddCards(ctx, models.AddCardsParams{
		UserID:      int64(user.DBID),
		LexiconName: req.Msg.Lexicon,
		// sqlc compiler can't detect this is a special type. It's ok.
		FsrsCard:       bts,
		Alphagrams:     alphagrams,
		NextScheduleds: nextScheduleds,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(&pb.AddCardsResponse{NumCardsAdded: uint32(numInserted)}), nil
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

	params, err := s.fsrsParams(ctx, int64(user.DBID))
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
		postponements[i].card = card
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

func (s *Server) Delete(ctx context.Context, req *connect.Request[pb.DeleteRequest]) (
	*connect.Response[pb.DeleteResponse], error) {

	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}

	if req.Msg.Lexicon == "" {
		return nil, invalidArgError("need a lexicon")
	}
	if req.Msg.OnlyNewQuestions && len(req.Msg.Alphagrams) > 0 {
		return nil, invalidArgError("cannot delete only new questions and a list of alphagrams")
	}
	var err error
	var deletedRows int64
	if req.Msg.OnlyNewQuestions {
		deletedRows, err = s.Queries.DeleteNewCards(ctx, models.DeleteNewCardsParams{
			UserID:      int64(user.DBID),
			LexiconName: req.Msg.Lexicon,
		})
	} else if len(req.Msg.Alphagrams) == 0 {
		// delete them all!
		deletedRows, err = s.Queries.DeleteCards(ctx, models.DeleteCardsParams{
			UserID: int64(user.DBID), LexiconName: req.Msg.Lexicon,
		})
	} else {
		deletedRows, err = s.Queries.DeleteCardsWithAlphagrams(ctx, models.DeleteCardsWithAlphagramsParams{
			UserID:      int64(user.DBID),
			LexiconName: req.Msg.Lexicon,
			Alphagrams:  req.Msg.Alphagrams,
		})
	}
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.DeleteResponse{NumDeleted: uint32(deletedRows)}), nil

}

// The fsrs library fuzzes only by day. It tends to ask questions at the same
// hour and minute that they were asked last. We want to add a little bit of a fuzz
// to allow for more randomness.
func furtherFuzzDueDate(params fsrs.Parameters, now time.Time, card *fsrs.Card) {
	if !params.EnableFuzz || params.EnableShortTerm {
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
