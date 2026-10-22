package wordvault

import (
	"context"
	"sort"
	"time"
	// The leaderboard timezone has to load even without system tzdata.
	_ "time/tzdata"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgtype"

	pb "github.com/domino14/word_db_server/api/rpc/wordvault"
	"github.com/domino14/word_db_server/internal/stores"
	"github.com/domino14/word_db_server/internal/stores/models"
)

// Leaderboard days run midnight to midnight US/Pacific, for everyone.
var leaderboardLoc = mustLoadLocation("America/Los_Angeles")

const leaderboardDays = 7

func mustLoadLocation(name string) *time.Location {
	loc, err := time.LoadLocation(name)
	if err != nil {
		panic(err)
	}
	return loc
}

// leaderboardDayStart returns the start of the leaderboard day containing t.
func leaderboardDayStart(t time.Time) time.Time {
	y, m, d := t.In(leaderboardLoc).Date()
	return time.Date(y, m, d, 0, 0, 0, 0, leaderboardLoc)
}

// toPGDate returns the calendar date of a leaderboard day.
func toPGDate(day time.Time) pgtype.Date {
	y, m, d := day.Date()
	return pgtype.Date{Time: time.Date(y, m, d, 0, 0, 0, 0, time.UTC), Valid: true}
}

// studiedSince reports whether a card's latest review (not counting
// Cardbox imports) is at or after t.
func studiedSince(revlog []stores.ReviewLog, t time.Time) bool {
	for i := len(revlog) - 1; i >= 0; i-- {
		if revlog[i].ImportLog != nil {
			continue
		}
		return !revlog[i].Review.Before(t)
	}
	return false
}

// addDailyStudied adds to a user's studied counts for the leaderboard day
// containing now, in the caller's transaction.
func addDailyStudied(ctx context.Context, qtx *models.Queries, userID int64, now time.Time,
	cards, wordwallsCards int) error {

	if cards == 0 && wordwallsCards == 0 {
		return nil
	}
	return qtx.AddDailyStudied(ctx, models.AddDailyStudiedParams{
		Day:            toPGDate(leaderboardDayStart(now)),
		UserID:         userID,
		Cards:          int32(cards),
		WordwallsCards: int32(wordwallsCards),
	})
}

func (s *Server) GetDailyLeaderboard(ctx context.Context, req *connect.Request[pb.GetDailyLeaderboardRequest]) (
	*connect.Response[pb.GetDailyLeaderboardResponse], error) {

	// interceptor still requires login, but we don't check it here  🤔

	today := toPGDate(leaderboardDayStart(s.Nower.Now()))
	rows, err := s.Queries.GetLeaderboard(ctx, models.GetLeaderboardParams{
		FirstDay: today,
		LastDay:  today,
	})
	if err != nil {
		return nil, err
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Cards != rows[j].Cards {
			return rows[i].Cards > rows[j].Cards
		}
		return rows[i].Username.String < rows[j].Username.String
	})

	resp := &pb.GetDailyLeaderboardResponse{
		Items: make([]*pb.GetDailyLeaderboardResponse_LeaderboardItem, len(rows)),
	}
	for i := range rows {
		resp.Items[i] = &pb.GetDailyLeaderboardResponse_LeaderboardItem{
			User:         rows[i].Username.String,
			CardsStudied: rows[i].Cards,
		}
	}
	return connect.NewResponse(resp), nil
}

func (s *Server) GetWeeklyLeaderboard(ctx context.Context, req *connect.Request[pb.GetWeeklyLeaderboardRequest]) (
	*connect.Response[pb.GetWeeklyLeaderboardResponse], error) {

	today := leaderboardDayStart(s.Nower.Now())
	firstDay := today.AddDate(0, 0, -(leaderboardDays - 1))
	rows, err := s.Queries.GetLeaderboard(ctx, models.GetLeaderboardParams{
		FirstDay: toPGDate(firstDay),
		LastDay:  toPGDate(today),
	})
	if err != nil {
		return nil, err
	}

	const layout = "2006-01-02"
	days := make([]string, leaderboardDays)
	dayIdx := map[string]int{}
	for i := range days {
		days[i] = firstDay.AddDate(0, 0, i).Format(layout)
		dayIdx[days[i]] = i
	}

	byUser := map[string]*pb.GetWeeklyLeaderboardResponse_LeaderboardItem{}
	items := []*pb.GetWeeklyLeaderboardResponse_LeaderboardItem{}
	for _, row := range rows {
		idx, ok := dayIdx[row.Day.Time.Format(layout)]
		if !ok {
			continue
		}
		item, ok := byUser[row.Username.String]
		if !ok {
			item = &pb.GetWeeklyLeaderboardResponse_LeaderboardItem{
				User:              row.Username.String,
				CardsStudiedByDay: make([]int32, leaderboardDays),
			}
			byUser[row.Username.String] = item
			items = append(items, item)
		}
		item.CardsStudiedByDay[idx] += row.Cards
		item.CardsStudied += row.Cards
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].CardsStudied != items[j].CardsStudied {
			return items[i].CardsStudied > items[j].CardsStudied
		}
		return items[i].User < items[j].User
	})

	return connect.NewResponse(&pb.GetWeeklyLeaderboardResponse{
		Days:  days,
		Items: items,
	}), nil
}
