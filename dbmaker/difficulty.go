package dbmaker

import (
	"database/sql"
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"

	"github.com/rs/zerolog/log"
)

func createDifficultyMap(lexiconPath string, lexiconName string) map[string]int {
	difficultyPath := filepath.Join(lexiconPath, "difficulty",
		lexiconName)
	dm := map[string]int{}
	// Only difficulty data for 7s and 8s for meow.
	for length := 7; length <= 8; length++ {
		filename := filepath.Join(difficultyPath, strconv.Itoa(length)+".csv")
		f, err := os.Open(filename)
		if err != nil {
			log.Info().Msgf("difficulty map creation: no file named %v found", filename)
			continue
		}
		defer f.Close()
		log.Info().Msgf("using difficulty file: %v", filename)
		lines, err := csv.NewReader(f).ReadAll()
		exitIfError(err)
		header := lines[0]
		qidx := -1
		aidx := -1
		for i, h := range header {
			if h == "Alphagram" {
				aidx = i
			}
			if h == "quantile" {
				qidx = i
			}
		}
		if qidx == -1 || aidx == -1 {
			panic("alphagram or quantile not found in file")
		}
		for _, line := range lines[1:] {
			// Each quantile starts with `q` so remove that from the string
			// before conversion.
			rating, err := strconv.Atoi(line[qidx][1:])
			exitIfError(err)
			// quantiles are 0-based; it's nicer to have a range from 1 to 100 inclusive:
			dm[line[aidx]] = rating + 1
		}
	}
	if len(dm) == 0 {
		return nil
	}
	log.Info().Int("map-size", len(dm)).Int("ACCHNOOS", dm["ACCHNOOS"]).Msg("created difficulty map")
	return dm
}

func alphagramDifficulty(alphagram string, difficulties map[string]int) int {
	// Default to 0 if not specified. This is ok.
	if difficulties == nil {
		return 0
	}
	var diff int
	var ok bool
	if diff, ok = difficulties[alphagram]; !ok {
		return 0
	}
	return diff
}

func loadDifficulty(db *sql.DB, lexInfo *LexiconInfo) {

	rows, err := db.Query(`
		SELECT alphagram FROM alphagrams WHERE length BETWEEN 7 AND 8
	`)
	exitIfError(err)
	defer rows.Close()

	tx, err := db.Begin()
	exitIfError(err)

	updateQuery := `
		UPDATE alphagrams SET difficulty = ? WHERE alphagram = ?
	`
	alphagrams := []Alphagram{}
	for rows.Next() {
		var (
			alph string
		)
		if err := rows.Scan(&alph); err != nil {
			log.Fatal().Err(err).Msg("")
		}
		alphagrams = append(alphagrams, Alphagram{alphagram: alph})
	}
	i := 0
	updateStmt, err := tx.Prepare(updateQuery)
	exitIfError(err)
	for _, alph := range alphagrams {
		d := alphagramDifficulty(alph.alphagram, lexInfo.Difficulties)
		_, err := updateStmt.Exec(d, alph.alphagram)
		exitIfError(err)
		i++
		if i%10000 == 0 {
			log.Debug().Msgf("%d...", i)
		}
	}
	tx.Commit()
}
