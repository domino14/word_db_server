package wordvault

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"connectrpc.com/connect"
	"github.com/jackc/pgx/v5"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	searchpb "github.com/domino14/word_db_server/api/rpc/wordsearcher"
	pb "github.com/domino14/word_db_server/api/rpc/wordvault"
	"github.com/domino14/word_db_server/internal/auth"
	"github.com/domino14/word_db_server/internal/querygen"
	"github.com/domino14/word_db_server/internal/searchserver"
	"github.com/domino14/word_db_server/internal/stores/models"
)

const (
	MaxStudyPlanStages        = 20
	MaxStudyPlanNameLength    = 100
	DefaultStudyPlanBlockSize = 500
	// Keeps a progress response to a few thousand blocks.
	MinStudyPlanBlockSize = 10
)

var ErrStudyPlanNotFound = connect.NewError(connect.CodeNotFound, errors.New("study plan not found"))

// studyPlanRow is a stored plan. sqlc makes an identical row type per query;
// they all convert to this one.
type studyPlanRow = models.GetStudyPlanRow

// cleanStudyPlan checks a plan from a request and returns a copy with
// lexicon conditions removed and names trimmed. Unsaved plans (for
// previewing progress) don't need a name.
func (s *Server) cleanStudyPlan(plan *pb.StudyPlan, needName bool) (*pb.StudyPlan, error) {
	if plan == nil {
		return nil, invalidArgError("need a study plan")
	}
	out := &pb.StudyPlan{
		Id:        plan.Id,
		Lexicon:   plan.Lexicon,
		Name:      strings.TrimSpace(plan.Name),
		DeckId:    plan.DeckId,
		BatchUnit: plan.BatchUnit,
		BatchSize: plan.BatchSize,
	}
	if out.Lexicon == "" {
		return nil, invalidArgError("need a lexicon")
	}
	if needName && out.Name == "" {
		return nil, invalidArgError("need a name")
	}
	if len([]rune(out.Name)) > MaxStudyPlanNameLength {
		return nil, invalidArgError(fmt.Sprintf("name can be at most %d characters", MaxStudyPlanNameLength))
	}
	if len(plan.Stages) == 0 {
		return nil, invalidArgError("need at least one stage")
	}
	if len(plan.Stages) > MaxStudyPlanStages {
		return nil, invalidArgError(fmt.Sprintf("a plan can have at most %d stages", MaxStudyPlanStages))
	}
	for i, st := range plan.Stages {
		stage := &pb.StudyPlanStage{
			Name:       strings.TrimSpace(st.Name),
			SortField:  st.SortField,
			Descending: st.Descending,
		}
		if len([]rune(stage.Name)) > MaxStudyPlanNameLength {
			return nil, invalidArgError(fmt.Sprintf("stage %d: name can be at most %d characters",
				i+1, MaxStudyPlanNameLength))
		}
		if _, ok := pb.StudyPlanSortField_name[int32(st.SortField)]; !ok {
			return nil, invalidArgError(fmt.Sprintf("stage %d: unknown sort field", i+1))
		}
		for _, p := range st.SearchParams {
			switch p.Condition {
			case searchpb.SearchRequest_LEXICON:
				continue
			case searchpb.SearchRequest_DELETED_WORD:
				return nil, invalidArgError(fmt.Sprintf("stage %d: cannot search for deleted words here", i+1))
			}
			stage.SearchParams = append(stage.SearchParams, p)
		}
		if len(stage.SearchParams) == 0 {
			return nil, invalidArgError(fmt.Sprintf("stage %d: need at least one search condition besides the lexicon", i+1))
		}
		// A list or limit has to be the last condition for the search to run.
		// All conditions must match, so moving it doesn't change the stage.
		slices.SortStableFunc(stage.SearchParams, func(a, b *searchpb.SearchRequest_SearchParam) int {
			return cmp.Compare(boolInt(querygen.IsMutexCondition(a.Condition)), boolInt(querygen.IsMutexCondition(b.Condition)))
		})
		out.Stages = append(out.Stages, stage)
	}
	switch out.BatchUnit {
	case pb.StudyPlanBatchUnit_STUDY_PLAN_BATCH_WINDOW:
		if out.BatchSize == 0 {
			return nil, invalidArgError("need a window size")
		}
	case pb.StudyPlanBatchUnit_STUDY_PLAN_BATCH_CARDS:
		if out.BatchSize == 0 {
			return nil, invalidArgError("need a number of cards per batch")
		}
		if out.BatchSize > uint32(s.Config.MaxCardsAdd) {
			return nil, invalidArgError(fmt.Sprintf("cannot add more than %d cards at a time", s.Config.MaxCardsAdd))
		}
	default:
		return nil, invalidArgError("need a batch unit")
	}
	return out, nil
}

// checkPlanDeck checks that the plan's deck is the user's and is in the
// plan's lexicon.
func (s *Server) checkPlanDeck(ctx context.Context, userID int64, plan *pb.StudyPlan) error {
	if plan.DeckId == 0 {
		return nil
	}
	deck, err := s.Queries.GetDeck(ctx, models.GetDeckParams{ID: int64(plan.DeckId), UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return invalidArgError("deck not found")
	} else if err != nil {
		return err
	}
	if deck.LexiconName != plan.Lexicon {
		return invalidArgError(fmt.Sprintf("deck %s is for %s, not %s", deck.Name, deck.LexiconName, plan.Lexicon))
	}
	return nil
}

// checkStages runs every stage's search, so a plan with a search the lexicon
// can't run is rejected when it's saved rather than when it's used.
func (s *Server) checkStages(ctx context.Context, plan *pb.StudyPlan) error {
	_, err := s.planItems(ctx, plan)
	return err
}

func marshalStages(stages []*pb.StudyPlanStage) ([]byte, error) {
	raw := make([]json.RawMessage, len(stages))
	for i, st := range stages {
		bts, err := protojson.Marshal(st)
		if err != nil {
			return nil, err
		}
		raw[i] = bts
	}
	return json.Marshal(raw)
}

func planFromRow(row studyPlanRow) (*pb.StudyPlan, error) {
	var raw []json.RawMessage
	if err := json.Unmarshal(row.Stages, &raw); err != nil {
		return nil, err
	}
	plan := &pb.StudyPlan{
		Id:        row.ID,
		Lexicon:   row.LexiconName,
		Name:      row.Name,
		DeckId:    uint64(row.DeckID),
		BatchUnit: pb.StudyPlanBatchUnit(row.BatchUnit),
		BatchSize: uint32(row.BatchSize),
		Stages:    make([]*pb.StudyPlanStage, len(raw)),
	}
	for i, r := range raw {
		plan.Stages[i] = &pb.StudyPlanStage{}
		// Tolerate fields from a newer server, e.g. during a rollback.
		if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(r, plan.Stages[i]); err != nil {
			return nil, err
		}
	}
	return plan, nil
}

func (s *Server) getStudyPlan(ctx context.Context, userID int64, id int64) (*pb.StudyPlan, error) {
	row, err := s.Queries.GetStudyPlan(ctx, models.GetStudyPlanParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrStudyPlanNotFound
	} else if err != nil {
		return nil, err
	}
	return planFromRow(row)
}

// planItem is one alphagram of a plan, in the stage it belongs to.
type planItem struct {
	alphagram string
	length    int32
	value     int32 // of the stage's sort field
	stage     int
}

func sortValue(a searchserver.RankedAlphagram, field pb.StudyPlanSortField) int32 {
	switch field {
	case pb.StudyPlanSortField_STUDY_PLAN_SORT_PROBABILITY:
		return a.Probability
	}
	// cleanStudyPlan only lets known fields through.
	panic(fmt.Sprintf("unhandled sort field %v", field))
}

// planItems returns the plan's alphagrams in plan order: stage by stage, and
// within a stage by its sort field, then length, then alphagram. An
// alphagram belongs to the first stage that matches it.
func (s *Server) planItems(ctx context.Context, plan *pb.StudyPlan) ([]planItem, error) {
	seen := map[string]bool{}
	var items []planItem
	for si, st := range plan.Stages {
		found, err := s.WordSearchServer.RankedAlphagrams(ctx, plan.Lexicon, st.SearchParams)
		if err != nil {
			var cerr *connect.Error
			if errors.As(err, &cerr) {
				return nil, connect.NewError(cerr.Code(), fmt.Errorf("stage %d: %s", si+1, cerr.Message()))
			}
			return nil, err
		}
		start := len(items)
		for _, a := range found {
			if seen[a.Alphagram] {
				continue
			}
			seen[a.Alphagram] = true
			items = append(items, planItem{a.Alphagram, a.Length, sortValue(a, st.SortField), si})
		}
		if len(items) > s.Config.MaxQueryResults {
			return nil, invalidArgError(fmt.Sprintf(
				"this plan has more than %d alphagrams; please narrow down its stages", s.Config.MaxQueryResults))
		}
		slices.SortFunc(items[start:], func(a, b planItem) int {
			c := cmp.Compare(a.value, b.value)
			if st.Descending {
				c = -c
			}
			return cmp.Or(c, cmp.Compare(a.length, b.length), strings.Compare(a.alphagram, b.alphagram))
		})
	}
	return items, nil
}

// windowOf returns the window of the given size that v falls in. Windows are
// 1–size, size+1–2·size, and so on.
func windowOf(v int32, size uint32) (int32, int32) {
	sz := int64(size)
	lo := floorDiv(int64(v)-1, sz)*sz + 1
	return int32(lo), int32(lo + sz - 1)
}

func floorDiv(a, b int64) int64 {
	q := a / b
	if a%b != 0 && a < 0 {
		q--
	}
	return q
}

func batchItem(it planItem) *pb.StudyPlanBatchItem {
	return &pb.StudyPlanBatchItem{
		Alphagram: it.alphagram,
		Stage:     uint32(it.stage),
		Length:    it.length,
		SortValue: it.value,
	}
}

// nextBatch returns the plan's next batch of alphagrams that aren't in the
// vault, at most maxCards of them, or nil if the plan is complete.
func nextBatch(plan *pb.StudyPlan, items []planItem, owned map[string]bool, maxCards int) *pb.StudyPlanBatch {
	first := slices.IndexFunc(items, func(it planItem) bool { return !owned[it.alphagram] })
	if first < 0 {
		return nil
	}
	b := &pb.StudyPlanBatch{Stage: uint32(items[first].stage)}
	switch plan.BatchUnit {
	case pb.StudyPlanBatchUnit_STUDY_PLAN_BATCH_WINDOW:
		if gap := gaps(items, owned, plan.BatchSize); gap[first] {
			// Fill in every gap up to the first window not yet reached, rather
			// than one window at a time.
			b.Gaps = true
			for i, it := range items[first:] {
				if owned[it.alphagram] {
					continue
				}
				if !gap[first+i] {
					break
				}
				b.NumMissingInWindow++
				if len(b.Items) < maxCards {
					b.Items = append(b.Items, batchItem(it))
				}
			}
			return b
		}
		b.WindowLo, b.WindowHi = windowOf(items[first].value, plan.BatchSize)
		for _, it := range items[first:] {
			if it.stage != items[first].stage {
				break
			}
			if owned[it.alphagram] || it.value < b.WindowLo || it.value > b.WindowHi {
				continue
			}
			b.NumMissingInWindow++
			if len(b.Items) < maxCards {
				b.Items = append(b.Items, batchItem(it))
			}
		}
	case pb.StudyPlanBatchUnit_STUDY_PLAN_BATCH_CARDS:
		n := min(int(plan.BatchSize), maxCards)
		for _, it := range items[first:] {
			if len(b.Items) == n {
				break
			}
			if !owned[it.alphagram] {
				b.Items = append(b.Items, batchItem(it))
			}
		}
	}
	return b
}

// gaps marks the missing alphagrams that come before the last complete
// window of their stage: holes in ground already covered, such as words a
// new lexicon added. Missing alphagrams after it haven't been reached yet. Cards added far ahead of the rest don't
// complete a window, so they don't turn what comes before them into gaps.
func gaps(items []planItem, owned map[string]bool, size uint32) []bool {
	gap := make([]bool, len(items))
	for start := 0; start < len(items); {
		end := start
		for end < len(items) && items[end].stage == items[start].stage {
			end++
		}
		// Items are in window order within a stage. reached is where the
		// stage's last complete window ends.
		reached := start
		for i := start; i < end; {
			lo, _ := windowOf(items[i].value, size)
			j, complete := i, true
			for ; j < end; j++ {
				if wlo, _ := windowOf(items[j].value, size); wlo != lo {
					break
				}
				complete = complete && owned[items[j].alphagram]
			}
			if complete {
				reached = j
			}
			i = j
		}
		for i := start; i < reached; i++ {
			gap[i] = !owned[items[i].alphagram]
		}
		start = end
	}
	return gap
}

func stageProgress(numStages int, items []planItem, owned map[string]bool,
	blockSize uint32) []*pb.StudyPlanStageProgress {

	out := make([]*pb.StudyPlanStageProgress, numStages)
	blocks := make([]map[int32]*pb.StudyPlanBlock, numStages)
	for i := range out {
		out[i] = &pb.StudyPlanStageProgress{}
		blocks[i] = map[int32]*pb.StudyPlanBlock{}
	}
	// Index of each stage's first missing alphagram, or -1.
	gap := make([]int, numStages)
	for i := range gap {
		gap[i] = -1
	}
	for i, it := range items {
		sp := out[it.stage]
		lo, hi := windowOf(it.value, blockSize)
		blk := blocks[it.stage][lo]
		if blk == nil {
			blk = &pb.StudyPlanBlock{Lo: lo, Hi: hi}
			blocks[it.stage][lo] = blk
		}
		sp.Total++
		blk.Total++
		if owned[it.alphagram] {
			sp.Added++
			blk.Added++
		} else if gap[it.stage] < 0 {
			gap[it.stage] = i
		}
	}

	// A stage is done through the last value before its first gap that the
	// gap doesn't share (a 7 and an 8 can have the same probability).
	stageEnd := make([]int, numStages)
	for i, it := range items {
		stageEnd[it.stage] = i + 1
	}
	for si, sp := range out {
		if sp.Total == 0 {
			continue
		}
		end := stageEnd[si]
		stageStart := end - int(sp.Total)
		cut := end
		if gap[si] >= 0 {
			cut = gap[si]
			for cut > stageStart && items[cut-1].value == items[gap[si]].value {
				cut--
			}
		}
		if cut > stageStart {
			v := items[cut-1].value
			sp.DoneThrough = &v
		}

		sp.Blocks = make([]*pb.StudyPlanBlock, 0, len(blocks[si]))
		for _, b := range blocks[si] {
			sp.Blocks = append(sp.Blocks, b)
		}
		slices.SortFunc(sp.Blocks, func(a, b *pb.StudyPlanBlock) int { return cmp.Compare(a.Lo, b.Lo) })
	}
	return out
}

func (s *Server) vaultAlphagrams(ctx context.Context, userID int64, lexicon string) (map[string]bool, error) {
	rows, err := s.Queries.GetVaultAlphagrams(ctx, models.GetVaultAlphagramsParams{
		UserID: userID, LexiconName: lexicon})
	if err != nil {
		return nil, err
	}
	owned := make(map[string]bool, len(rows))
	for _, a := range rows {
		owned[a] = true
	}
	return owned, nil
}

func (s *Server) AddStudyPlan(ctx context.Context, req *connect.Request[pb.AddStudyPlanRequest]) (
	*connect.Response[pb.AddStudyPlanResponse], error) {

	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	plan, err := s.cleanStudyPlan(req.Msg.Plan, true)
	if err != nil {
		return nil, err
	}
	if err := s.checkPlanDeck(ctx, int64(user.DBID), plan); err != nil {
		return nil, err
	}
	if err := s.checkStages(ctx, plan); err != nil {
		return nil, err
	}
	stages, err := marshalStages(plan.Stages)
	if err != nil {
		return nil, err
	}
	row, err := s.Queries.AddStudyPlan(ctx, models.AddStudyPlanParams{
		UserID:      int64(user.DBID),
		LexiconName: plan.Lexicon,
		Name:        plan.Name,
		DeckID:      int64(plan.DeckId),
		BatchUnit:   int16(plan.BatchUnit),
		BatchSize:   int32(plan.BatchSize),
		Stages:      stages,
	})
	if err != nil {
		return nil, err
	}
	saved, err := planFromRow(studyPlanRow(row))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.AddStudyPlanResponse{Plan: saved}), nil
}

func (s *Server) GetStudyPlans(ctx context.Context, req *connect.Request[pb.GetStudyPlansRequest]) (
	*connect.Response[pb.GetStudyPlansResponse], error) {

	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	rows, err := s.Queries.GetStudyPlans(ctx, int64(user.DBID))
	if err != nil {
		return nil, err
	}
	resp := &pb.GetStudyPlansResponse{Plans: make([]*pb.StudyPlan, len(rows))}
	for i, row := range rows {
		resp.Plans[i], err = planFromRow(studyPlanRow(row))
		if err != nil {
			return nil, err
		}
	}
	return connect.NewResponse(resp), nil
}

func (s *Server) EditStudyPlan(ctx context.Context, req *connect.Request[pb.EditStudyPlanRequest]) (
	*connect.Response[pb.EditStudyPlanResponse], error) {

	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	if req.Msg.Plan == nil || req.Msg.Plan.Id == 0 {
		return nil, invalidArgError("need a study plan")
	}
	existing, err := s.getStudyPlan(ctx, int64(user.DBID), req.Msg.Plan.Id)
	if err != nil {
		return nil, err
	}
	if req.Msg.Plan.Lexicon != "" && req.Msg.Plan.Lexicon != existing.Lexicon {
		return nil, invalidArgError("cannot change a study plan's lexicon")
	}
	edited := proto.Clone(req.Msg.Plan).(*pb.StudyPlan)
	edited.Lexicon = existing.Lexicon
	plan, err := s.cleanStudyPlan(edited, true)
	if err != nil {
		return nil, err
	}
	if err := s.checkPlanDeck(ctx, int64(user.DBID), plan); err != nil {
		return nil, err
	}
	if err := s.checkStages(ctx, plan); err != nil {
		return nil, err
	}
	stages, err := marshalStages(plan.Stages)
	if err != nil {
		return nil, err
	}
	row, err := s.Queries.EditStudyPlan(ctx, models.EditStudyPlanParams{
		ID:        plan.Id,
		UserID:    int64(user.DBID),
		Name:      plan.Name,
		DeckID:    int64(plan.DeckId),
		BatchUnit: int16(plan.BatchUnit),
		BatchSize: int32(plan.BatchSize),
		Stages:    stages,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrStudyPlanNotFound
	} else if err != nil {
		return nil, err
	}
	saved, err := planFromRow(studyPlanRow(row))
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.EditStudyPlanResponse{Plan: saved}), nil
}

func (s *Server) DeleteStudyPlan(ctx context.Context, req *connect.Request[pb.DeleteStudyPlanRequest]) (
	*connect.Response[pb.DeleteStudyPlanResponse], error) {

	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	n, err := s.Queries.DeleteStudyPlan(ctx, models.DeleteStudyPlanParams{
		ID: req.Msg.Id, UserID: int64(user.DBID)})
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrStudyPlanNotFound
	}
	return connect.NewResponse(&pb.DeleteStudyPlanResponse{}), nil
}

func (s *Server) GetStudyPlanProgress(ctx context.Context, req *connect.Request[pb.GetStudyPlanProgressRequest]) (
	*connect.Response[pb.GetStudyPlanProgressResponse], error) {

	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	var plan *pb.StudyPlan
	var err error
	switch w := req.Msg.Which.(type) {
	case *pb.GetStudyPlanProgressRequest_PlanId:
		plan, err = s.getStudyPlan(ctx, int64(user.DBID), w.PlanId)
	case *pb.GetStudyPlanProgressRequest_Plan:
		plan, err = s.cleanStudyPlan(w.Plan, false)
	default:
		err = invalidArgError("need a study plan")
	}
	if err != nil {
		return nil, err
	}

	blockSize := req.Msg.BlockSize
	if blockSize == 0 {
		blockSize = DefaultStudyPlanBlockSize
		if plan.BatchUnit == pb.StudyPlanBatchUnit_STUDY_PLAN_BATCH_WINDOW {
			blockSize = plan.BatchSize
		}
	}
	blockSize = max(blockSize, MinStudyPlanBlockSize)

	items, err := s.planItems(ctx, plan)
	if err != nil {
		return nil, err
	}
	owned, err := s.vaultAlphagrams(ctx, int64(user.DBID), plan.Lexicon)
	if err != nil {
		return nil, err
	}
	resp := &pb.GetStudyPlanProgressResponse{
		Stages:    stageProgress(len(plan.Stages), items, owned, blockSize),
		NextBatch: nextBatch(plan, items, owned, s.Config.MaxCardsAdd),
		BlockSize: blockSize,
	}
	for _, sp := range resp.Stages {
		resp.Total += sp.Total
		resp.Added += sp.Added
	}

	if req.Msg.IncludeDeckCounts {
		inDeck, err := s.Queries.GetVaultAlphagramsInDeck(ctx, models.GetVaultAlphagramsInDeckParams{
			UserID: int64(user.DBID), LexiconName: plan.Lexicon, DeckID: int64(plan.DeckId)})
		if err != nil {
			return nil, err
		}
		inPlan := make(map[string]bool, len(items))
		for _, it := range items {
			inPlan[it.alphagram] = true
		}
		dc := &pb.StudyPlanDeckCounts{CardsInDeck: uint32(len(inDeck))}
		for _, a := range inDeck {
			if inPlan[a] {
				dc.CardsInDeckInPlan++
			}
		}
		resp.DeckCounts = dc
	}
	return connect.NewResponse(resp), nil
}

func (s *Server) AddStudyPlanBatch(ctx context.Context, req *connect.Request[pb.AddStudyPlanBatchRequest]) (
	*connect.Response[pb.AddStudyPlanBatchResponse], error) {

	user := auth.UserFromContext(ctx)
	if user == nil {
		return nil, unauthenticated("user not authenticated")
	}
	plan, err := s.getStudyPlan(ctx, int64(user.DBID), req.Msg.PlanId)
	if err != nil {
		return nil, err
	}
	items, err := s.planItems(ctx, plan)
	if err != nil {
		return nil, err
	}
	owned, err := s.vaultAlphagrams(ctx, int64(user.DBID), plan.Lexicon)
	if err != nil {
		return nil, err
	}
	batch := nextBatch(plan, items, owned, s.Config.MaxCardsAdd)
	if batch == nil {
		return connect.NewResponse(&pb.AddStudyPlanBatchResponse{}), nil
	}
	alphagrams := make([]string, len(batch.Items))
	for i, it := range batch.Items {
		alphagrams[i] = it.Alphagram
	}
	added, err := s.addCards(ctx, user, plan.Lexicon, alphagrams, plan.DeckId)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&pb.AddStudyPlanBatchResponse{
		NumCardsAdded: added.NumCardsAdded,
		Batch:         batch,
	}), nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
