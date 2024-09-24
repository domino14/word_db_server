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

	"github.com/domino14/word_db_server/config"
	"github.com/domino14/word_db_server/internal/auth"
	"github.com/domino14/word_db_server/internal/searchserver"
	"github.com/domino14/word_db_server/internal/stores/models"
	searchpb "github.com/domino14/word_db_server/rpc/api/wordsearcher"
	pb "github.com/domino14/word_db_server/rpc/api/wordvault"
)

type nower interface {
	Now() time.Time
}

type RealNower struct{}

func (r RealNower) Now() time.Time {
	return time.Now()
}

const MaxCardsAdd = 1000

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
	params, err := s.Queries.LoadParams(ctx, int64(user.DBID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No params exist for this user
			log.Debug().Int("userID", user.DBID).Msg("no-params-found")
			params = fsrs.DefaultParam()
			params.EnableShortTerm = true
			params.EnableFuzz = true
		} else {
			return nil, err
		}
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
		cards[i] = &pb.Card{
			Lexicon: req.Msg.Lexicon,
			// Just return the alphagram here. The purpose of this endpoint is for
			// its metadata, not to quiz on any of the cards.
			Alphagram:     &searchpb.Alphagram{Alphagram: req.Msg.Alphagrams[i]},
			LastReviewed:  timestamppb.New(fcard.LastReview),
			NextScheduled: timestamppb.New(rows[i].NextScheduled.Time),
			NumAsked:      int32(fcard.Reps),
			NumLapses:     int32(fcard.Lapses),
			Stability:     fcard.Stability,
			Difficulty:    fcard.Difficulty,
			Status:        pb.Status(fcard.State + 1), // iota starts at 0 (New)
			// Retrievability is computed as of the request.
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
		UserID:      int64(user.DBID),
		LexiconName: req.Msg.Lexicon,
		Limit:       req.Msg.Limit,
	})
	if err != nil {
		return nil, err
	}
	cards := make([]*pb.Card, len(rows))

	alphagrams := make([]string, len(rows))
	for i := range rows {
		alphagrams[i] = rows[i].Alphagram
	}
	expandResponse, err := s.WordSearchServer.Search(ctx, connect.NewRequest(&searchpb.SearchRequest{
		Expand: true,
		Searchparams: []*searchpb.SearchRequest_SearchParam{
			{
				Condition: searchpb.SearchRequest_LEXICON,
				Conditionparam: &searchpb.SearchRequest_SearchParam_Stringvalue{
					Stringvalue: &searchpb.SearchRequest_StringValue{Value: req.Msg.Lexicon},
				}},
			{Condition: searchpb.SearchRequest_ALPHAGRAM_LIST,
				Conditionparam: &searchpb.SearchRequest_SearchParam_Stringarray{
					Stringarray: &searchpb.SearchRequest_StringArray{Values: alphagrams},
				}},
		}}))
	if err != nil {
		return nil, err
	}
	for i := range rows {
		fcard := rows[i].FsrsCard
		cards[i] = &pb.Card{
			Lexicon:       req.Msg.Lexicon,
			Alphagram:     expandResponse.Msg.Alphagrams[i],
			LastReviewed:  timestamppb.New(fcard.LastReview),
			NextScheduled: timestamppb.New(rows[i].NextScheduled.Time),
			NumAsked:      int32(fcard.Reps),
			NumLapses:     int32(fcard.Lapses),
			Stability:     fcard.Stability,
			Difficulty:    fcard.Difficulty,
			Status:        pb.Status(fcard.State + 1), // iota starts at 0 (New)
		}
	}
	return connect.NewResponse(&pb.Cards{Cards: cards}), nil

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

	// Load from user params
	params, err := qtx.LoadParams(ctx, int64(user.DBID))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No params exist for this user
			log.Debug().Int("userID", user.DBID).Msg("no-params-found")
			params = fsrs.DefaultParam()
			params.EnableShortTerm = true
			params.EnableFuzz = true
		} else {
			return nil, err
		}
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
	card = schedulingCards[fsrs.Rating(req.Msg.Score)].Card

	err = qtx.UpdateCard(ctx, models.UpdateCardParams{
		FsrsCard:      card,
		NextScheduled: pgtype.Timestamptz{Time: card.Due, Valid: true},
		UserID:        int64(user.DBID),
		LexiconName:   req.Msg.Lexicon,
		Alphagram:     req.Msg.Alphagram,
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

func (s *Server) AddCard(ctx context.Context, req *connect.Request[pb.AddCardRequest]) (
	*connect.Response[pb.AddCardResponse], error) {
	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	// In the future load these for the user
	// p := fsrs.DefaultParam()
	// p.EnableFuzz = true
	// p.EnableShortTerm = false

	card := fsrs.NewCard()
	now := s.Nower.Now()

	card.Due = now
	err := s.Queries.AddCard(ctx, models.AddCardParams{
		UserID:        int64(user.DBID),
		LexiconName:   req.Msg.Lexicon,
		Alphagram:     req.Msg.Alphagram,
		NextScheduled: pgtype.Timestamptz{Time: now, Valid: true},
		FsrsCard:      card,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewResponse(&pb.AddCardResponse{}), nil
}

func (s *Server) AddCards(ctx context.Context, req *connect.Request[pb.AddCardsRequest]) (
	*connect.Response[pb.AddCardsResponse], error) {
	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	if len(req.Msg.Alphagrams) > MaxCardsAdd {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("cannot add more than %d cards at a time", MaxCardsAdd))
	}
	// Just add the same card for ease for now.
	card := fsrs.NewCard()
	now := s.Nower.Now()

	card.Due = now
	alphagrams := req.Msg.Alphagrams
	nextScheduleds := make([]pgtype.Timestamptz, len(alphagrams))
	for i := range alphagrams {
		nextScheduleds[i] = pgtype.Timestamptz{Time: now, Valid: true}
	}
	bts, err := json.Marshal(card)
	if err != nil {
		return nil, err
	}

	err = s.Queries.AddCards(ctx, models.AddCardsParams{
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
	return connect.NewResponse(&pb.AddCardsResponse{}), nil
}
