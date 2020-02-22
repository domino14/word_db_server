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
		lines, err := csv.NewReader(f).ReadAll()
		if err != nil {
			panic(err)
		}
		// Skip the header row; start at lines[1:]
		for _, line := range lines[1:] {
			rating, err := strconv.Atoi(line[2])
			if err != nil {
				panic(err)
			}
			dm[line[0]] = rating
		}
	}
	if len(dm) == 0 {
		return nil
	}
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

func loadDifficulty(db *sql.DB, lexInfo LexiconInfo) {

	rows, err := db.Query(`
		SELECT alphagram FROM alphagrams WHERE length BETWEEN 7 AND 8
	`)
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}
	defer rows.Close()

	tx, err := db.Begin()
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

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
	for _, alph := range alphagrams {
		d := alphagramDifficulty(alph.alphagram, lexInfo.Difficulties)
		_, err := updateStmt.Exec(d, alph.alphagram)
		if err != nil {
			log.Fatal().Err(err).Msg("")
		}
		i++
		if i%10000 == 0 {
			log.Debug().Msgf("%d...", i)
		}
	}
	tx.Commit()
}
