package dbmaker

import (
	"database/sql"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/rs/zerolog/log"
)

// createPlayabilityMap reads lexica/playability/<lexiconName>.tsv, the
// ranks.tsv written by macondo's cmd/playability pipeline. It has one row per
// alphagram; `rank` is the alphagram's playability rank within its length,
// 1 being the most playable. It returns nil if there's no file.
func createPlayabilityMap(lexiconPath string, lexiconName string) map[string]int {
	filename := filepath.Join(lexiconPath, "playability", lexiconName+".tsv")
	f, err := os.Open(filename)
	if err != nil {
		log.Info().Msgf("playability map creation: no file named %v found", filename)
		return nil
	}
	defer f.Close()
	log.Info().Msgf("using playability file: %v", filename)
	pm, err := readPlayabilities(f)
	if err != nil {
		log.Fatal().Err(err).Str("file", filename).Msg("bad playability file")
	}
	log.Info().Int("map-size", len(pm)).Int("AEINRST", pm["AEINRST"]).Msg("created playability map")
	return pm
}

func readPlayabilities(r io.Reader) (map[string]int, error) {
	cr := csv.NewReader(r)
	cr.Comma = '\t'
	cr.LazyQuotes = true
	header, err := cr.Read()
	if err != nil {
		return nil, err
	}
	aidx, ridx := -1, -1
	for i, h := range header {
		switch h {
		case "alphagram":
			aidx = i
		case "rank":
			ridx = i
		}
	}
	if aidx == -1 || ridx == -1 {
		return nil, fmt.Errorf("need alphagram and rank columns, got %v", header)
	}
	pm := map[string]int{}
	for {
		line, err := cr.Read()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, err
		}
		rank, err := strconv.Atoi(line[ridx])
		if err != nil {
			return nil, err
		}
		if rank < 1 {
			return nil, fmt.Errorf("%s has rank %d", line[aidx], rank)
		}
		if _, ok := pm[line[aidx]]; ok {
			return nil, fmt.Errorf("%s appears twice", line[aidx])
		}
		pm[line[aidx]] = rank
	}
	return pm, nil
}

// checkPlayabilities makes sure the playability file covers exactly the
// lexicon's alphagrams, so no alphagram is left without a rank.
func checkPlayabilities(alphagrams []string, playabilities map[string]int) error {
	missing := []string{}
	for _, a := range alphagrams {
		if _, ok := playabilities[a]; !ok {
			missing = append(missing, a)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("%d alphagrams have no playability, e.g. %v", len(missing),
			missing[:min(len(missing), 5)])
	}
	if len(playabilities) != len(alphagrams) {
		return fmt.Errorf("playability file has %d alphagrams but the lexicon has %d",
			len(playabilities), len(alphagrams))
	}
	return nil
}

// alphagramPlayability is the value for the playability column: NULL for a
// lexicon without playability data.
func alphagramPlayability(alphagram string, playabilities map[string]int) sql.NullInt32 {
	if playabilities == nil {
		return sql.NullInt32{}
	}
	return sql.NullInt32{Int32: int32(playabilities[alphagram]), Valid: true}
}

func loadPlayability(db *sql.DB, lexInfo *LexiconInfo) {
	if lexInfo.Playabilities == nil {
		log.Info().Msg("no playability data for this lexicon; leaving the column empty")
		return
	}
	rows, err := db.Query(`SELECT alphagram FROM alphagrams`)
	exitIfError(err)
	alphagrams := []string{}
	for rows.Next() {
		var alph string
		exitIfError(rows.Scan(&alph))
		alphagrams = append(alphagrams, alph)
	}
	exitIfError(rows.Err())
	rows.Close()
	exitIfError(checkPlayabilities(alphagrams, lexInfo.Playabilities))

	tx, err := db.Begin()
	exitIfError(err)
	updateStmt, err := tx.Prepare(`UPDATE alphagrams SET playability = ? WHERE alphagram = ?`)
	exitIfError(err)
	for _, alph := range alphagrams {
		_, err := updateStmt.Exec(alphagramPlayability(alph, lexInfo.Playabilities), alph)
		exitIfError(err)
	}
	exitIfError(tx.Commit())
}
