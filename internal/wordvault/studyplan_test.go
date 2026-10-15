package wordvault

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"testing"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/matryer/is"
	"github.com/rs/zerolog/log"

	searchpb "github.com/domino14/word_db_server/api/rpc/wordsearcher"
	pb "github.com/domino14/word_db_server/api/rpc/wordvault"
	"github.com/domino14/word_db_server/internal/auth"
	"github.com/domino14/word_db_server/internal/searchserver"
	"github.com/domino14/word_db_server/internal/stores/models"
)

func TestWindowOf(t *testing.T) {
	is := is.New(t)
	for _, tc := range []struct {
		v      int32
		size   uint32
		lo, hi int32
	}{
		{1, 500, 1, 500},
		{500, 500, 1, 500},
		{501, 500, 501, 1000},
		{3501, 500, 3501, 4000},
		{4000, 500, 3501, 4000},
		{201, 50, 201, 250},
		{7, 1, 7, 7},
		{0, 500, -499, 0},
	} {
		lo, hi := windowOf(tc.v, tc.size)
		is.Equal([2]int32{lo, hi}, [2]int32{tc.lo, tc.hi})
	}
}

// Two stages: the first ascending, where a 7 and an 8 share each value; the
// second descending.
var testPlanItems = []planItem{
	{"A7", 7, 1, 0}, {"A8", 8, 1, 0},
	{"B7", 7, 2, 0},
	{"C7", 7, 3, 0}, {"C8", 8, 3, 0},
	{"D7", 7, 4, 0},
	{"E7", 7, 5, 0},
	{"Z4", 7, 4, 1}, {"Z3", 7, 3, 1}, {"Z1", 7, 1, 1},
}

func ownedSet(alphas ...string) map[string]bool {
	m := map[string]bool{}
	for _, a := range alphas {
		m[a] = true
	}
	return m
}

func batchAlphagrams(b *pb.StudyPlanBatch) []string {
	out := []string{}
	for _, it := range b.Items {
		out = append(out, it.Alphagram)
	}
	return out
}

func TestNextBatchWindow(t *testing.T) {
	is := is.New(t)
	plan := &pb.StudyPlan{BatchUnit: pb.StudyPlanBatchUnit_STUDY_PLAN_BATCH_WINDOW, BatchSize: 2}

	// Nothing owned: the first window of the first stage.
	b := nextBatch(plan, testPlanItems, ownedSet(), 1000)
	is.Equal(b.Stage, uint32(0))
	is.Equal([2]int32{b.WindowLo, b.WindowHi}, [2]int32{1, 2})
	is.Equal(batchAlphagrams(b), []string{"A7", "A8", "B7"})
	is.Equal(b.NumMissingInWindow, uint32(3))

	// A window only takes what's missing, and never reaches into another
	// stage, although Z4 and Z3 are in the same range of values.
	b = nextBatch(plan, testPlanItems, ownedSet("A7", "A8", "B7", "C7"), 1000)
	is.Equal([2]int32{b.WindowLo, b.WindowHi}, [2]int32{3, 4})
	is.Equal(batchAlphagrams(b), []string{"C8", "D7"})

	// A window bigger than the card limit comes in pieces.
	b = nextBatch(plan, testPlanItems, ownedSet("A7", "A8", "B7"), 1)
	is.Equal(batchAlphagrams(b), []string{"C7"})
	is.Equal(b.NumMissingInWindow, uint32(3))

	// A finished stage moves on to the next one, in its order (descending).
	b = nextBatch(plan, testPlanItems, ownedSet("A7", "A8", "B7", "C7", "C8", "D7", "E7"), 1000)
	is.Equal(b.Stage, uint32(1))
	is.Equal([2]int32{b.WindowLo, b.WindowHi}, [2]int32{3, 4})
	is.Equal(batchAlphagrams(b), []string{"Z4", "Z3"})

	// Skipped cards are picked up first, wherever they are.
	b = nextBatch(plan, testPlanItems, ownedSet("A7", "B7", "C7", "C8", "D7"), 1000)
	is.Equal(batchAlphagrams(b), []string{"A8"})

	all := ownedSet()
	for _, it := range testPlanItems {
		all[it.alphagram] = true
	}
	is.True(nextBatch(plan, testPlanItems, all, 1000) == nil)
}

func TestNextBatchCards(t *testing.T) {
	is := is.New(t)
	plan := &pb.StudyPlan{BatchUnit: pb.StudyPlanBatchUnit_STUDY_PLAN_BATCH_CARDS, BatchSize: 3}

	b := nextBatch(plan, testPlanItems, ownedSet("A8", "C7"), 1000)
	is.Equal(b.Stage, uint32(0))
	is.Equal(batchAlphagrams(b), []string{"A7", "B7", "C8"})
	is.Equal(b.WindowLo, int32(0))
	is.Equal(b.NumMissingInWindow, uint32(0))

	// Batches run across stages.
	b = nextBatch(plan, testPlanItems, ownedSet("A7", "A8", "B7", "C7", "C8"), 1000)
	is.Equal(batchAlphagrams(b), []string{"D7", "E7", "Z4"})

	// The card limit wins over the batch size.
	b = nextBatch(plan, testPlanItems, ownedSet(), 2)
	is.Equal(batchAlphagrams(b), []string{"A7", "A8"})
}

func TestStageProgress(t *testing.T) {
	is := is.New(t)

	// C8 is missing, and C7 has the same value, so stage 1 is done through 2
	// rather than 3. Z4, the first card of the descending stage, is there.
	ps := stageProgress(2, testPlanItems, ownedSet("A7", "A8", "B7", "C7", "E7", "Z4", "Z1"), 2)
	is.Equal(len(ps), 2)
	is.Equal(ps[0].Total, uint32(7))
	is.Equal(ps[0].Added, uint32(5))
	is.Equal(*ps[0].DoneThrough, int32(2))
	blocks := func(sp *pb.StudyPlanStageProgress) [][4]int32 {
		out := [][4]int32{}
		for _, b := range sp.Blocks {
			out = append(out, [4]int32{b.Lo, b.Hi, int32(b.Total), int32(b.Added)})
		}
		return out
	}
	is.Equal(blocks(ps[0]), [][4]int32{{1, 2, 3, 3}, {3, 4, 3, 1}, {5, 6, 1, 1}})

	is.Equal(ps[1].Total, uint32(3))
	is.Equal(ps[1].Added, uint32(2))
	is.Equal(*ps[1].DoneThrough, int32(4))
	// Blocks are in ascending order even for a descending stage.
	is.Equal(blocks(ps[1]), [][4]int32{{1, 2, 1, 1}, {3, 4, 2, 1}})

	// Not started, even with cards added further on.
	ps = stageProgress(2, testPlanItems, ownedSet("B7", "Z3"), 2)
	is.True(ps[0].DoneThrough == nil)
	is.True(ps[1].DoneThrough == nil)

	// Complete.
	all := ownedSet()
	for _, it := range testPlanItems {
		all[it.alphagram] = true
	}
	ps = stageProgress(2, testPlanItems, all, 2)
	is.Equal(*ps[0].DoneThrough, int32(5))
	is.Equal(*ps[1].DoneThrough, int32(1))

	// A stage that matches nothing (everything went to earlier stages).
	ps = stageProgress(3, testPlanItems, all, 2)
	is.Equal(ps[2].Total, uint32(0))
	is.True(ps[2].DoneThrough == nil)
	is.Equal(len(ps[2].Blocks), 0)
}

func planStage(name string, params ...*searchpb.SearchRequest_SearchParam) *pb.StudyPlanStage {
	return &pb.StudyPlanStage{Name: name, SearchParams: params}
}

func windowPlan(lexicon, name string, size uint32, deckID uint64, stages ...*pb.StudyPlanStage) *pb.StudyPlan {
	return &pb.StudyPlan{
		Lexicon: lexicon, Name: name, DeckId: deckID, Stages: stages,
		BatchUnit: pb.StudyPlanBatchUnit_STUDY_PLAN_BATCH_WINDOW, BatchSize: size,
	}
}

func TestCleanStudyPlan(t *testing.T) {
	is := is.New(t)
	s := &Server{Config: DefaultConfig}
	sevens := searchserver.SearchDescLength(7, 7)

	plan, err := s.cleanStudyPlan(&pb.StudyPlan{
		Lexicon: "NWL23", Name: "  7s  ",
		Stages: []*pb.StudyPlanStage{planStage(" Top ", searchserver.SearchDescLexicon("CSW21"), sevens,
			searchserver.SearchDescProbRange(1, 5000))},
		BatchUnit: pb.StudyPlanBatchUnit_STUDY_PLAN_BATCH_CARDS, BatchSize: 100,
	}, true)
	is.NoErr(err)
	is.Equal(plan.Name, "7s")
	is.Equal(plan.Stages[0].Name, "Top")
	// The lexicon condition is dropped; the plan's lexicon is what counts.
	is.Equal(len(plan.Stages[0].SearchParams), 2)
	is.Equal(plan.Stages[0].SearchParams[0].Condition, searchpb.SearchRequest_LENGTH)

	for _, tc := range []struct {
		name string
		plan *pb.StudyPlan
	}{
		{"no plan", nil},
		{"no lexicon", windowPlan("", "x", 500, 0, planStage("", sevens))},
		{"no name", windowPlan("NWL23", " ", 500, 0, planStage("", sevens))},
		{"no stages", windowPlan("NWL23", "x", 500, 0)},
		{"only a lexicon", windowPlan("NWL23", "x", 500, 0, planStage("", searchserver.SearchDescLexicon("NWL23")))},
		{"deleted words", windowPlan("NWL23", "x", 500, 0, planStage("", searchserver.SearchDescDeleted()))},
		{"no window size", windowPlan("NWL23", "x", 0, 0, planStage("", sevens))},
		{"no batch unit", &pb.StudyPlan{Lexicon: "NWL23", Name: "x", BatchSize: 5,
			Stages: []*pb.StudyPlanStage{planStage("", sevens)}}},
		{"too many cards", &pb.StudyPlan{Lexicon: "NWL23", Name: "x", Stages: []*pb.StudyPlanStage{planStage("", sevens)},
			BatchUnit: pb.StudyPlanBatchUnit_STUDY_PLAN_BATCH_CARDS, BatchSize: uint32(DefaultConfig.MaxCardsAdd + 1)}},
		{"unknown sort field", windowPlan("NWL23", "x", 500, 0,
			&pb.StudyPlanStage{SearchParams: []*searchpb.SearchRequest_SearchParam{sevens}, SortField: 99})},
	} {
		_, err := s.cleanStudyPlan(tc.plan, true)
		if err == nil {
			t.Errorf("%s: expected an error", tc.name)
		}
	}

	stages := make([]*pb.StudyPlanStage, MaxStudyPlanStages+1)
	for i := range stages {
		stages[i] = planStage("", sevens)
	}
	_, err = s.cleanStudyPlan(windowPlan("NWL23", "x", 500, 0, stages...), true)
	is.True(err != nil)

	// An unsaved plan, previewed while editing, doesn't need a name.
	_, err = s.cleanStudyPlan(windowPlan("NWL23", "", 500, 0, planStage("", sevens)), false)
	is.NoErr(err)
}

// lexiconQuery returns the alphagrams of the test lexicon matching a WHERE
// clause, in probability order. It's the independent check on what plans
// pick: plain SQL against the lexicon file.
func lexiconQuery(t *testing.T, lexicon, where string, args ...any) []string {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(DefaultConfig.DataPath, "lexica", "db", lexicon+".db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	rows, err := db.Query("SELECT alphagram FROM alphagrams WHERE "+where+" ORDER BY probability, length", args...)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			t.Fatal(err)
		}
		out = append(out, a)
	}
	return out
}

func lexiconInt(t *testing.T, lexicon, query string, args ...any) int32 {
	t.Helper()
	db, err := sql.Open("sqlite3", filepath.Join(DefaultConfig.DataPath, "lexica", "db", lexicon+".db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int32
	if err := db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func without(alphas []string, drop ...string) []string {
	return slices.DeleteFunc(slices.Clone(alphas), func(a string) bool { return slices.Contains(drop, a) })
}

func sorted(alphas []string) []string {
	return slices.Sorted(slices.Values(alphas))
}

func addInChunks(ctx context.Context, t *testing.T, s *Server, lexicon string, deckID uint64, alphas []string) {
	t.Helper()
	for chunk := range slices.Chunk(alphas, s.Config.MaxCardsAdd) {
		_, err := s.AddCards(ctx, connect.NewRequest(&pb.AddCardsRequest{
			Lexicon: lexicon, Alphagrams: chunk, DeckId: deckID}))
		if err != nil {
			t.Fatal(err)
		}
	}
}

func deckOf(ctx context.Context, t *testing.T, pool *pgxpool.Pool, userID int, alphagram string) int64 {
	t.Helper()
	var deck int64
	err := pool.QueryRow(ctx, `SELECT COALESCE(deck_id, 0) FROM wordvault_cards
		WHERE user_id = $1 AND lexicon_name = 'NWL23' AND alphagram = $2`, userID, alphagram).Scan(&deck)
	if err != nil {
		t.Fatal(err)
	}
	return deck
}

func studyPlanTestServer(t *testing.T) (*Server, *pgxpool.Pool) {
	if err := RecreateTestDB(); err != nil {
		t.Fatal(err)
	}
	dbPool, err := pgxpool.New(context.Background(), testDBURI(true))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(dbPool.Close)
	return NewServer(DefaultConfig, dbPool, models.New(dbPool), &searchserver.Server{Config: DefaultConfig}), dbPool
}

func memberCtx(userID int, username string) context.Context {
	ctx := log.Logger.WithContext(context.Background())
	return auth.StoreUserInContext(ctx, userID, username, true)
}

// TestStudyPlanPicksUpWhereYouLeftOff is the main use case: a user who has
// been adding cards by hand creates a plan matching what they were doing,
// links it to their deck, and adds the next batches.
func TestStudyPlanPicksUpWhereYouLeftOff(t *testing.T) {
	is := is.New(t)
	s, pool := studyPlanTestServer(t)
	ctx := memberCtx(42, "cesar")

	const stage1Where = "length IN (7, 8) AND probability <= 3000 AND num_anagrams >= 3"
	const stage2Where = "length IN (7, 8) AND probability BETWEEN ? AND ? AND num_anagrams < 3"
	stage1 := lexiconQuery(t, "NWL23", stage1Where)
	stage2 := lexiconQuery(t, "NWL23", stage2Where, 1, 3000)
	is.True(len(stage1) > 0)

	// What they added by hand: all of stage 1, stage 2 through probability
	// 1,000, and a few strays: one of stage 2 further on, one in another deck
	// (which still counts as done), and a 9 that isn't in the plan.
	addInChunks(ctx, t, s, "NWL23", 0, stage1)
	addInChunks(ctx, t, s, "NWL23", 0, lexiconQuery(t, "NWL23", "length IN (7, 8) AND probability <= 1000"))
	stray := lexiconQuery(t, "NWL23", stage2Where, 1700, 1750)[0]
	strayOther := lexiconQuery(t, "NWL23", stage2Where, 1200, 1250)[0]
	nine := lexiconQuery(t, "NWL23", "length = 9 AND probability = 1")[0]
	addInChunks(ctx, t, s, "NWL23", 0, []string{stray, nine})
	other, err := s.AddDeck(ctx, connect.NewRequest(&pb.AddDeckRequest{Name: "Other", Lexicon: "NWL23"}))
	is.NoErr(err)
	addInChunks(ctx, t, s, "NWL23", uint64(other.Msg.Deck.Id), []string{strayOther})

	plan := windowPlan("NWL23", "7s & 8s", 500, 0,
		// Overlaps with stage 2: an alphagram belongs to the first stage
		// that matches it.
		planStage("Several anagrams", searchserver.SearchDescLength(7, 8), searchserver.SearchDescNumAnagrams(3, 100),
			searchserver.SearchDescProbRange(1, 3000)),
		planStage("Everything else", searchserver.SearchDescLength(7, 8), searchserver.SearchDescProbRange(1, 3000)),
	)

	// Before saving: the editor previews the plan against the vault.
	preview, err := s.GetStudyPlanProgress(ctx, connect.NewRequest(&pb.GetStudyPlanProgressRequest{
		Which: &pb.GetStudyPlanProgressRequest_Plan{Plan: plan}, IncludeDeckCounts: true}))
	is.NoErr(err)
	stage2Done := len(lexiconQuery(t, "NWL23", stage2Where, 1, 1000))
	defaultDeck := len(stage1) + stage2Done + 2
	is.Equal(preview.Msg.DeckCounts.CardsInDeck, uint32(defaultDeck))
	is.Equal(preview.Msg.DeckCounts.CardsInDeckInPlan, uint32(defaultDeck-1)) // not the 9

	saved, err := s.AddStudyPlan(ctx, connect.NewRequest(&pb.AddStudyPlanRequest{Plan: plan}))
	is.NoErr(err)
	planID := saved.Msg.Plan.Id
	is.True(planID > 0)
	is.Equal(saved.Msg.Plan.Name, "7s & 8s")
	is.Equal(len(saved.Msg.Plan.Stages), 2)
	is.Equal(len(saved.Msg.Plan.Stages[1].SearchParams), 2)

	progress, err := s.GetStudyPlanProgress(ctx, connect.NewRequest(&pb.GetStudyPlanProgressRequest{
		Which: &pb.GetStudyPlanProgressRequest_PlanId{PlanId: planID}}))
	is.NoErr(err)
	p := progress.Msg
	is.Equal(p.BlockSize, uint32(500))
	is.Equal(p.Stages[0].Total, uint32(len(stage1)))
	is.Equal(p.Stages[0].Added, uint32(len(stage1)))
	is.Equal(*p.Stages[0].DoneThrough, lexiconInt(t, "NWL23", "SELECT MAX(probability) FROM alphagrams WHERE "+stage1Where))
	is.Equal(p.Stages[1].Total, uint32(len(stage2)))
	is.Equal(p.Stages[1].Added, uint32(stage2Done+2))
	is.Equal(*p.Stages[1].DoneThrough, lexiconInt(t, "NWL23",
		"SELECT MAX(probability) FROM alphagrams WHERE "+stage2Where, 1, 1000))
	is.Equal(p.Total, uint32(len(stage1)+len(stage2)))
	is.Equal(p.Added, uint32(len(stage1)+stage2Done+2))
	is.True(p.DeckCounts == nil)

	// The progress map: blocks of 500, the strays showing up as partly
	// added.
	var stage2Blocks [][2]int32
	for _, b := range p.Stages[1].Blocks {
		stage2Blocks = append(stage2Blocks, [2]int32{b.Lo, int32(b.Added)})
		if b.Hi <= 1000 {
			is.Equal(b.Added, b.Total)
		}
	}
	is.Equal(len(stage2Blocks), 6)
	is.Equal(stage2Blocks[2:], [][2]int32{{1001, 1}, {1501, 1}, {2001, 0}, {2501, 0}})

	// Next: stage 2, 1,001–1,500, without the card already in the other
	// deck.
	want := without(lexiconQuery(t, "NWL23", stage2Where, 1001, 1500), strayOther)
	is.Equal(p.NextBatch.Stage, uint32(1))
	is.Equal([2]int32{p.NextBatch.WindowLo, p.NextBatch.WindowHi}, [2]int32{1001, 1500})
	is.Equal(sorted(batchAlphagrams(p.NextBatch)), sorted(want))
	is.Equal(p.NextBatch.NumMissingInWindow, uint32(len(want)))

	added, err := s.AddStudyPlanBatch(ctx, connect.NewRequest(&pb.AddStudyPlanBatchRequest{PlanId: planID}))
	is.NoErr(err)
	is.Equal(added.Msg.NumCardsAdded, uint32(len(want)))
	is.Equal(added.Msg.Batch.WindowLo, int32(1001))
	is.Equal(sorted(batchAlphagrams(added.Msg.Batch)), sorted(want))
	is.Equal(deckOf(ctx, t, pool, 42, want[0]), int64(0))
	// The stray stays in its deck.
	is.Equal(deckOf(ctx, t, pool, 42, strayOther), other.Msg.Deck.Id)

	// Keep going until the plan is done.
	for _, lo := range []int32{1501, 2001, 2501} {
		want := without(lexiconQuery(t, "NWL23", stage2Where, lo, lo+499), stray)
		added, err := s.AddStudyPlanBatch(ctx, connect.NewRequest(&pb.AddStudyPlanBatchRequest{PlanId: planID}))
		is.NoErr(err)
		is.Equal(added.Msg.Batch.WindowLo, lo)
		is.Equal(sorted(batchAlphagrams(added.Msg.Batch)), sorted(want))
		is.Equal(added.Msg.NumCardsAdded, uint32(len(want)))
	}
	added, err = s.AddStudyPlanBatch(ctx, connect.NewRequest(&pb.AddStudyPlanBatchRequest{PlanId: planID}))
	is.NoErr(err)
	is.Equal(added.Msg.NumCardsAdded, uint32(0))
	is.True(added.Msg.Batch == nil)

	progress, err = s.GetStudyPlanProgress(ctx, connect.NewRequest(&pb.GetStudyPlanProgressRequest{
		Which: &pb.GetStudyPlanProgressRequest_PlanId{PlanId: planID}, BlockSize: 1000}))
	is.NoErr(err)
	is.Equal(progress.Msg.Added, progress.Msg.Total)
	is.True(progress.Msg.NextBatch == nil)
	is.Equal(progress.Msg.BlockSize, uint32(1000))
	is.Equal(len(progress.Msg.Stages[1].Blocks), 3)
}

// A window bigger than the per-request card limit is added over several
// presses.
func TestStudyPlanWindowOverCardLimit(t *testing.T) {
	is := is.New(t)
	s, _ := studyPlanTestServer(t)
	cfg := *DefaultConfig
	cfg.MaxCardsAdd = 100
	s.Config = &cfg
	ctx := memberCtx(42, "cesar")

	saved, err := s.AddStudyPlan(ctx, connect.NewRequest(&pb.AddStudyPlanRequest{Plan: windowPlan("NWL23", "7s", 250, 0,
		planStage("", searchserver.SearchDescLength(7, 7)))}))
	is.NoErr(err)
	id := saved.Msg.Plan.Id

	window := lexiconQuery(t, "NWL23", "length = 7 AND probability BETWEEN 1 AND 250")
	var got []string
	for _, n := range []int{100, 100, 50} {
		added, err := s.AddStudyPlanBatch(ctx, connect.NewRequest(&pb.AddStudyPlanBatchRequest{PlanId: id}))
		is.NoErr(err)
		is.Equal(added.Msg.Batch.WindowLo, int32(1))
		is.Equal(int(added.Msg.NumCardsAdded), n)
		is.Equal(int(added.Msg.Batch.NumMissingInWindow), len(window)-len(got))
		got = append(got, batchAlphagrams(added.Msg.Batch)...)
	}
	// In plan order.
	is.Equal(got, window)

	added, err := s.AddStudyPlanBatch(ctx, connect.NewRequest(&pb.AddStudyPlanBatchRequest{PlanId: id}))
	is.NoErr(err)
	is.Equal(added.Msg.Batch.WindowLo, int32(251))
}

// The other ways people study: the 8s from least to most likely, 100 at a
// time, in their own deck.
func TestStudyPlanDescendingByCards(t *testing.T) {
	is := is.New(t)
	s, pool := studyPlanTestServer(t)
	ctx := memberCtx(43, "someone")

	deck, err := s.AddDeck(ctx, connect.NewRequest(&pb.AddDeckRequest{Name: "8s", Lexicon: "NWL23"}))
	is.NoErr(err)
	deckID := uint64(deck.Msg.Deck.Id)
	saved, err := s.AddStudyPlan(ctx, connect.NewRequest(&pb.AddStudyPlanRequest{Plan: &pb.StudyPlan{
		Lexicon: "NWL23", Name: "8s backwards", DeckId: deckID,
		Stages: []*pb.StudyPlanStage{{
			SearchParams: []*searchpb.SearchRequest_SearchParam{
				searchserver.SearchDescLength(8, 8), searchserver.SearchDescProbRange(1, 1000)},
			Descending: true,
		}},
		BatchUnit: pb.StudyPlanBatchUnit_STUDY_PLAN_BATCH_CARDS, BatchSize: 100,
	}}))
	is.NoErr(err)
	id := saved.Msg.Plan.Id

	eights := lexiconQuery(t, "NWL23", "length = 8 AND probability BETWEEN 1 AND 1000")
	slices.Reverse(eights)

	added, err := s.AddStudyPlanBatch(ctx, connect.NewRequest(&pb.AddStudyPlanBatchRequest{PlanId: id}))
	is.NoErr(err)
	is.Equal(added.Msg.NumCardsAdded, uint32(100))
	is.Equal(batchAlphagrams(added.Msg.Batch), eights[:100])
	is.Equal(added.Msg.Batch.Items[0].SortValue, int32(1000))
	is.Equal(deckOf(ctx, t, pool, 43, eights[0]), int64(deckID))

	// A deleted card comes back with the next batch.
	_, err = s.DeleteFromDeck(ctx, connect.NewRequest(&pb.DeleteFromDeckRequest{
		Lexicon: "NWL23", OnlyAlphagrams: []string{eights[5]}, DeckId: deckID}))
	is.NoErr(err)
	added, err = s.AddStudyPlanBatch(ctx, connect.NewRequest(&pb.AddStudyPlanBatchRequest{PlanId: id}))
	is.NoErr(err)
	is.Equal(batchAlphagrams(added.Msg.Batch), append([]string{eights[5]}, eights[100:199]...))

	progress, err := s.GetStudyPlanProgress(ctx, connect.NewRequest(&pb.GetStudyPlanProgressRequest{
		Which: &pb.GetStudyPlanProgressRequest_PlanId{PlanId: id}}))
	is.NoErr(err)
	// 100, less the deleted card, plus 100 with it back.
	is.Equal(progress.Msg.Stages[0].Added, uint32(199))
	is.Equal(progress.Msg.BlockSize, uint32(DefaultStudyPlanBlockSize))
	// Done down to the last card added (the 8s have no ties).
	is.Equal(*progress.Msg.Stages[0].DoneThrough, added.Msg.Batch.Items[99].SortValue)
	is.Equal(batchAlphagrams(progress.Msg.NextBatch)[0], eights[199])
}

func TestStudyPlanCRUD(t *testing.T) {
	is := is.New(t)
	s, _ := studyPlanTestServer(t)
	ctx := memberCtx(42, "cesar")
	sevens := planStage("", searchserver.SearchDescLength(7, 7))

	_, err := s.AddStudyPlan(context.Background(), connect.NewRequest(&pb.AddStudyPlanRequest{
		Plan: windowPlan("NWL23", "7s", 500, 0, sevens)}))
	is.Equal(connect.CodeOf(err), connect.CodeUnauthenticated)

	first, err := s.AddStudyPlan(ctx, connect.NewRequest(&pb.AddStudyPlanRequest{
		Plan: windowPlan("NWL23", "7s", 500, 0, sevens)}))
	is.NoErr(err)

	// One plan per deck, the default deck included.
	_, err = s.AddStudyPlan(ctx, connect.NewRequest(&pb.AddStudyPlanRequest{
		Plan: windowPlan("NWL23", "More 7s", 500, 0, sevens)}))
	is.Equal(connect.CodeOf(err), connect.CodeInvalidArgument)
	// But each lexicon has its own default deck.
	_, err = s.AddStudyPlan(ctx, connect.NewRequest(&pb.AddStudyPlanRequest{
		Plan: windowPlan("CSW21", "CSW 7s", 500, 0, sevens)}))
	is.NoErr(err)

	cswDeck, err := s.AddDeck(ctx, connect.NewRequest(&pb.AddDeckRequest{Name: "CSW", Lexicon: "CSW21"}))
	is.NoErr(err)
	_, err = s.AddStudyPlan(ctx, connect.NewRequest(&pb.AddStudyPlanRequest{
		Plan: windowPlan("NWL23", "x", 500, uint64(cswDeck.Msg.Deck.Id), sevens)}))
	is.Equal(connect.CodeOf(err), connect.CodeInvalidArgument) // deck is for another lexicon

	theirDeck, err := s.AddDeck(memberCtx(43, "someone"), connect.NewRequest(&pb.AddDeckRequest{Name: "Mine", Lexicon: "NWL23"}))
	is.NoErr(err)
	_, err = s.AddStudyPlan(ctx, connect.NewRequest(&pb.AddStudyPlanRequest{
		Plan: windowPlan("NWL23", "x", 500, uint64(theirDeck.Msg.Deck.Id), sevens)}))
	is.Equal(connect.CodeOf(err), connect.CodeInvalidArgument)

	// A search the lexicon can't run is caught when the plan is saved.
	_, err = s.AddStudyPlan(ctx, connect.NewRequest(&pb.AddStudyPlanRequest{
		Plan: windowPlan("NWL23", "x", 500, 0, planStage("", &searchpb.SearchRequest_SearchParam{
			Condition: searchpb.SearchRequest_LENGTH}))}))
	is.Equal(connect.CodeOf(err), connect.CodeInvalidArgument)
	_, err = s.AddStudyPlan(ctx, connect.NewRequest(&pb.AddStudyPlanRequest{
		Plan: windowPlan("NOSUCHLEX", "x", 500, 0, sevens)}))
	is.True(err != nil)

	// Move the first plan to a deck of its own and change how it batches.
	nwlDeck, err := s.AddDeck(ctx, connect.NewRequest(&pb.AddDeckRequest{Name: "7s", Lexicon: "NWL23"}))
	is.NoErr(err)
	edit := windowPlan("", "Sevens", 50, uint64(nwlDeck.Msg.Deck.Id),
		planStage("Top", searchserver.SearchDescLength(7, 7), searchserver.SearchDescProbRange(1, 1000)))
	edit.Id = first.Msg.Plan.Id
	edited, err := s.EditStudyPlan(ctx, connect.NewRequest(&pb.EditStudyPlanRequest{Plan: edit}))
	is.NoErr(err)
	is.Equal(edited.Msg.Plan.Lexicon, "NWL23")
	is.Equal(edited.Msg.Plan.Name, "Sevens")
	is.Equal(edited.Msg.Plan.BatchSize, uint32(50))
	is.Equal(edited.Msg.Plan.DeckId, uint64(nwlDeck.Msg.Deck.Id))
	is.Equal(edited.Msg.Plan.Stages[0].Name, "Top")

	// That frees up the default deck.
	_, err = s.AddStudyPlan(ctx, connect.NewRequest(&pb.AddStudyPlanRequest{
		Plan: windowPlan("NWL23", "More 7s", 500, 0, sevens)}))
	is.NoErr(err)

	edit.Lexicon = "CSW21"
	_, err = s.EditStudyPlan(ctx, connect.NewRequest(&pb.EditStudyPlanRequest{Plan: edit}))
	is.Equal(connect.CodeOf(err), connect.CodeInvalidArgument)

	plans, err := s.GetStudyPlans(ctx, connect.NewRequest(&pb.GetStudyPlansRequest{}))
	is.NoErr(err)
	is.Equal(len(plans.Msg.Plans), 3)
	is.Equal(plans.Msg.Plans[0].Name, "Sevens")
	is.Equal(plans.Msg.Plans[0].Stages[0].SearchParams[1].GetMinmax().Max, int32(1000))

	// Other users can't see, use, change or delete the plan.
	them := memberCtx(43, "someone")
	plans, err = s.GetStudyPlans(them, connect.NewRequest(&pb.GetStudyPlansRequest{}))
	is.NoErr(err)
	is.Equal(len(plans.Msg.Plans), 0)
	_, err = s.GetStudyPlanProgress(them, connect.NewRequest(&pb.GetStudyPlanProgressRequest{
		Which: &pb.GetStudyPlanProgressRequest_PlanId{PlanId: first.Msg.Plan.Id}}))
	is.Equal(connect.CodeOf(err), connect.CodeNotFound)
	_, err = s.AddStudyPlanBatch(them, connect.NewRequest(&pb.AddStudyPlanBatchRequest{PlanId: first.Msg.Plan.Id}))
	is.Equal(connect.CodeOf(err), connect.CodeNotFound)
	_, err = s.EditStudyPlan(them, connect.NewRequest(&pb.EditStudyPlanRequest{Plan: edit}))
	is.Equal(connect.CodeOf(err), connect.CodeNotFound)
	_, err = s.DeleteStudyPlan(them, connect.NewRequest(&pb.DeleteStudyPlanRequest{Id: first.Msg.Plan.Id}))
	is.Equal(connect.CodeOf(err), connect.CodeNotFound)

	// A deck with a plan can't be deleted until the plan is.
	_, err = s.DeleteDeck(ctx, connect.NewRequest(&pb.DeleteDeckRequest{Id: nwlDeck.Msg.Deck.Id}))
	is.Equal(connect.CodeOf(err), connect.CodeInvalidArgument)
	_, err = s.DeleteStudyPlan(ctx, connect.NewRequest(&pb.DeleteStudyPlanRequest{Id: first.Msg.Plan.Id}))
	is.NoErr(err)
	_, err = s.DeleteDeck(ctx, connect.NewRequest(&pb.DeleteDeckRequest{Id: nwlDeck.Msg.Deck.Id}))
	is.NoErr(err)
	_, err = s.DeleteStudyPlan(ctx, connect.NewRequest(&pb.DeleteStudyPlanRequest{Id: first.Msg.Plan.Id}))
	is.Equal(connect.CodeOf(err), connect.CodeNotFound)
}

// Word-level conditions (hooks, definitions) go through a different query.
func TestStudyPlanWordConditions(t *testing.T) {
	is := is.New(t)
	s, _ := studyPlanTestServer(t)
	ctx := memberCtx(42, "cesar")

	plan := windowPlan("NWL23", "", 100, 0, planStage("", searchserver.SearchDescLength(7, 7),
		searchserver.SearchDescProbRange(1, 2000),
		&searchpb.SearchRequest_SearchParam{
			Condition: searchpb.SearchRequest_DEFINITION_CONTAINS,
			Conditionparam: &searchpb.SearchRequest_SearchParam_Stringvalue{
				Stringvalue: &searchpb.SearchRequest_StringValue{Value: "plant"}},
		}))
	progress, err := s.GetStudyPlanProgress(ctx, connect.NewRequest(&pb.GetStudyPlanProgressRequest{
		Which: &pb.GetStudyPlanProgressRequest_Plan{Plan: plan}}))
	is.NoErr(err)

	want := lexiconQuery(t, "NWL23", `length = 7 AND probability BETWEEN 1 AND 2000 AND alphagram IN (
		SELECT alphagram FROM words WHERE definition LIKE '%plant%')`)
	is.True(len(want) > 0)
	is.Equal(progress.Msg.Total, uint32(len(want)))
	firstWindow := lexiconQuery(t, "NWL23", `length = 7 AND probability BETWEEN ? AND ? AND alphagram IN (
		SELECT alphagram FROM words WHERE definition LIKE '%plant%')`,
		progress.Msg.NextBatch.WindowLo, progress.Msg.NextBatch.WindowHi)
	is.Equal(batchAlphagrams(progress.Msg.NextBatch), firstWindow)
}

// A probability limit picks the stage's alphagrams: here the 1000 most
// probable 7s with at least 4 vowels, which run well past probability 1000.
// Batches then go through them 100 at a time.
func TestStudyPlanProbabilityLimit(t *testing.T) {
	is := is.New(t)
	s, _ := studyPlanTestServer(t)
	ctx := memberCtx(42, "cesar")

	saved, err := s.AddStudyPlan(ctx, connect.NewRequest(&pb.AddStudyPlanRequest{Plan: &pb.StudyPlan{
		Lexicon: "NWL23", Name: "vowel-heavy 7s",
		Stages: []*pb.StudyPlanStage{planStage("", searchserver.SearchDescLength(7, 7),
			searchserver.SearchDescNumVowels(4, 7), searchserver.SearchDescProbLimit(1, 1000))},
		BatchUnit: pb.StudyPlanBatchUnit_STUDY_PLAN_BATCH_CARDS, BatchSize: 100,
	}}))
	is.NoErr(err)
	id := saved.Msg.Plan.Id

	want := lexiconQuery(t, "NWL23", "length = 7 AND num_vowels >= 4")[:1000]
	// A range of 1-1000 would find far fewer.
	is.True(len(lexiconQuery(t, "NWL23", "length = 7 AND num_vowels >= 4 AND probability <= 1000")) < 500)

	for i := range 10 {
		added, err := s.AddStudyPlanBatch(ctx, connect.NewRequest(&pb.AddStudyPlanBatchRequest{PlanId: id}))
		is.NoErr(err)
		is.Equal(added.Msg.NumCardsAdded, uint32(100))
		is.Equal(batchAlphagrams(added.Msg.Batch), want[i*100:(i+1)*100])
	}
	progress, err := s.GetStudyPlanProgress(ctx, connect.NewRequest(&pb.GetStudyPlanProgressRequest{
		Which: &pb.GetStudyPlanProgressRequest_PlanId{PlanId: id}}))
	is.NoErr(err)
	is.Equal(progress.Msg.Total, uint32(1000))
	is.Equal(progress.Msg.Added, uint32(1000))
	is.True(progress.Msg.NextBatch == nil)
	is.True(*progress.Msg.Stages[0].DoneThrough > 1000)
}

func TestStudyPlanProbabilityList(t *testing.T) {
	is := is.New(t)
	s, _ := studyPlanTestServer(t)
	ctx := memberCtx(42, "cesar")

	// Windows of 10 skip the probabilities that aren't on the list.
	plan := windowPlan("NWL23", "", 10, 0, planStage("", searchserver.SearchDescLength(7, 7),
		searchserver.SearchDescProbabilityList([]int32{3, 5, 25, 27})))
	progress, err := s.GetStudyPlanProgress(ctx, connect.NewRequest(&pb.GetStudyPlanProgressRequest{
		Which: &pb.GetStudyPlanProgressRequest_Plan{Plan: plan}}))
	is.NoErr(err)
	is.Equal(progress.Msg.Total, uint32(4))
	is.Equal(progress.Msg.NextBatch.WindowLo, int32(1))
	is.Equal(batchAlphagrams(progress.Msg.NextBatch),
		lexiconQuery(t, "NWL23", "length = 7 AND probability IN (3, 5)"))

	// The query generator can't combine a limit with a list, so such a
	// stage is refused when it's saved.
	_, err = s.AddStudyPlan(ctx, connect.NewRequest(&pb.AddStudyPlanRequest{Plan: windowPlan("NWL23", "x", 10, 0,
		planStage("", searchserver.SearchDescAlphagramList([]string{"AEINRST", "ADEINRS"}),
			searchserver.SearchDescProbLimit(1, 1)))}))
	is.Equal(connect.CodeOf(err), connect.CodeInvalidArgument)
}
