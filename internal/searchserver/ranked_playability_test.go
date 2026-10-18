package searchserver

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/assert"

	pb "github.com/domino14/word_db_server/api/rpc/wordsearcher"
	"github.com/domino14/word_db_server/config"
	"github.com/domino14/word_db_server/internal/querygen"
)

// playabilityTestServer makes a data path with three small lexicon
// databases: PLAY has playability ranks, NOPLAY was made before version 7
// (no playability column), and NULLPLAY has the column but no data.
func playabilityTestServer(t *testing.T) *Server {
	dir := t.TempDir()
	dbDir := filepath.Join(dir, "lexica", "db")
	assert.Nil(t, os.MkdirAll(dbDir, 0o755))
	for _, tc := range []struct {
		lex    string
		column string
		rows   string
	}{
		{"PLAY", ", playability int", `
			('AEINRST', 7, 10, 1), ('AEEINRT', 7, 6, 2), ('AEEIRST', 7, 28, 3),
			('ADEEINRT', 8, 14, 15), ('ADEINRST', 8, 3, 1)`},
		{"NOPLAY", "", `('AEINRST', 7, 10)`},
		{"NULLPLAY", ", playability int", `('AEINRST', 7, 10, NULL)`},
	} {
		db, err := sql.Open("sqlite3", filepath.Join(dbDir, tc.lex+".db"))
		assert.Nil(t, err)
		_, err = db.Exec(`CREATE TABLE alphagrams (alphagram varchar(20), length int,
			probability int` + tc.column + `);
			INSERT INTO alphagrams VALUES ` + tc.rows)
		assert.Nil(t, err)
		db.Close()
	}
	return &Server{Config: &config.Config{DataPath: dir, MaxQueryResults: 100}}
}

func TestRankedAlphagramsPlayability(t *testing.T) {
	s := playabilityTestServer(t)
	ctx := context.Background()

	// Still in probability order; the rank is the playability.
	ranked, err := s.RankedAlphagrams(ctx, "PLAY", []*pb.SearchRequest_SearchParam{
		SearchDescLength(7, 8)}, querygen.RankByPlayability)
	assert.Nil(t, err)
	assert.Equal(t, []RankedAlphagram{
		{"ADEINRST", 8, 1}, {"AEEINRT", 7, 2}, {"AEINRST", 7, 1},
		{"ADEEINRT", 8, 15}, {"AEEIRST", 7, 3},
	}, ranked)

	ranked, err = s.RankedAlphagrams(ctx, "PLAY", []*pb.SearchRequest_SearchParam{
		SearchDescLength(7, 7), SearchDescPlayabilityRange(2, 3)}, querygen.RankByProbability)
	assert.Nil(t, err)
	assert.Equal(t, []RankedAlphagram{{"AEEINRT", 7, 6}, {"AEEIRST", 7, 28}}, ranked)

	// Probability ranks still work on a database without playability.
	ranked, err = s.RankedAlphagrams(ctx, "NOPLAY", []*pb.SearchRequest_SearchParam{
		SearchDescLength(7, 7)}, querygen.RankByProbability)
	assert.Nil(t, err)
	assert.Equal(t, []RankedAlphagram{{"AEINRST", 7, 10}}, ranked)

	for _, lex := range []string{"NOPLAY", "NULLPLAY"} {
		_, err = s.RankedAlphagrams(ctx, lex, []*pb.SearchRequest_SearchParam{
			SearchDescLength(7, 7)}, querygen.RankByPlayability)
		assert.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err), lex)
		assert.Contains(t, err.Error(), "has no playability data", lex)
	}
}
