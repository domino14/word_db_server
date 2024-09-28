package wordvault

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		cards[i] = &pb.Card{
			Lexicon: req.Msg.Lexicon,
			// Just return the alphagram here. The purpose of this endpoint is for
			// its metadata, not to quiz on any of the cards.
			Alphagram:      &searchpb.Alphagram{Alphagram: req.Msg.Alphagrams[i]},
			CardJsonRepr:   cardbts,
			Retrievability: f.GetRetrievability(fcard, s.Nower.Now()),
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
		Limit:         req.Msg.Limit,
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
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid score"))
	}
	if req.Msg.Lexicon == "" || req.Msg.Alphagram == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("no such lexicon or alphagram"))
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
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("card with your input parameters was not found"))
		} else {
			return nil, err
		}
	}
	card := cardrow.FsrsCard

	schedulingCards := f.Repeat(card, now)
	rating := fsrs.Rating(req.Msg.Score)
	card = schedulingCards[rating].Card
	rlog := schedulingCards[rating].ReviewLog
	rlogbts, err := json.Marshal(rlog)
	if err != nil {
		return nil, err
	}

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
	return connect.NewResponse(&pb.ScoreCardResponse{
		NextScheduled: timestamppb.New(card.Due),
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
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid score"))
	}
	if req.Msg.Lexicon == "" || req.Msg.Alphagram == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("no such lexicon or alphagram"))
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
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("card with your input parameters was not found"))
		} else {
			return nil, err
		}
	}
	if len(cardrow.ReviewLog) == 0 {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("this card has no review history"))
	}

	card := fsrs.Card{}
	err = json.Unmarshal(req.Msg.LastCardRepr, &card)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("last card was not properly provided"))
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
	return connect.NewResponse(&pb.ScoreCardResponse{
		NextScheduled: timestamppb.New(card.Due),
	}), nil

}

func (s *Server) AddCards(ctx context.Context, req *connect.Request[pb.AddCardsRequest]) (
	*connect.Response[pb.AddCardsResponse], error) {
	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	if len(req.Msg.Alphagrams) > s.Config.MaxCardsAdd {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cannot add more than %d cards at a time", s.Config.MaxCardsAdd))
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
	return connect.NewResponse(&pb.AddCardsResponse{NumCardsAdded: int32(numInserted)}), nil
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
	breakdown := map[string]uint32{}
	if req.Msg.OnlyOverdue {
		ocCount, err := s.Queries.GetOverdueCount(ctx, models.GetOverdueCountParams{
			UserID: int64(user.DBID),
			Now:    toPGTimestamp(s.Nower.Now()),
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
			UserID: int64(user.DBID),
			Now:    toPGTimestamp(s.Nower.Now()),
			Tz:     tz,
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
