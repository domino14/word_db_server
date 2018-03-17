// Package dbmaker creates SQLITE databases for various lexica, so I can use
// them in my word game empire.
package dbmaker

import (
	"bufio"
	"database/sql"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/domino14/macondo/gaddag"
	"github.com/domino14/macondo/lexicon"
	_ "github.com/mattn/go-sqlite3"
)

type Alphagram struct {
	words        []string
	combinations uint64
	alphagram    string
	wordCount    uint8
}

func (a *Alphagram) String() string {
	return fmt.Sprintf("Alphagram: %s (%d)", a.alphagram, a.combinations)
}

func (a *Alphagram) pointValue(dist lexicon.LetterDistribution) uint8 {
	pts := uint8(0)
	for _, rn := range a.alphagram {
		pts += dist.PointValues[rn]
	}
	return pts
}

func (a *Alphagram) numVowels() uint8 {
	vowels := uint8(0)
	for _, rn := range a.alphagram {
		if rn == 'A' || rn == 'E' || rn == 'I' || rn == 'O' || rn == 'U' {
			vowels += 1
		}
	}
	return vowels
}

type AlphByCombos []Alphagram // used to be []*Alphagram

func (a AlphByCombos) Len() int      { return len(a) }
func (a AlphByCombos) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a AlphByCombos) Less(i, j int) bool {
	// XXX: Existing aerolith dbs don't sort by alphagram to break ties.
	// (It's sort of random unfortunately)
	// The DBs generated by this tool will be slightly off. We must continue
	// to use the old DBs until there is a lexicon update :(
	if a[i].combinations == a[j].combinations {
		return a[i].alphagram < a[j].alphagram
	} else {
		return a[i].combinations > a[j].combinations
	}
}

type LexiconMap map[string]lexicon.LexiconInfo

type LexiconSymbolDefinition struct {
	In     string // The word is in this lexicon
	NotIn  string // The word is not in this lexicon
	Symbol string // The corresponding lexicon symbol
}

const CurrentVersion = 3

// create a sqlite db for this lexicon name.
func createSqliteDb(outputDir string, lexiconName string) string {
	dbName := outputDir + "/" + lexiconName + ".db"
	os.Remove(dbName)
	sqlStmt := `
	CREATE TABLE alphagrams (probability int, alphagram varchar(20),
	    length int, combinations int, num_anagrams int,
	    point_value int, num_vowels int);

	CREATE TABLE words (word varchar(20), alphagram varchar(20),
	    lexicon_symbols varchar(5), definition varchar(512),
	    front_hooks varchar(26), back_hooks varchar(26),
	    inner_front_hook int, inner_back_hook int);

	CREATE INDEX alpha_index on alphagrams(alphagram);
	CREATE INDEX prob_index on alphagrams(probability, length);
	CREATE INDEX word_index on words(word);
	CREATE INDEX alphagram_index on words(alphagram);
	CREATE INDEX length_index on alphagrams(length);

	CREATE INDEX num_anagrams_index on alphagrams(num_anagrams);
	CREATE INDEX point_value_index on alphagrams(point_value);
	CREATE INDEX num_vowels_index on alphagrams(num_vowels);

	CREATE TABLE db_version (version integer);
	`
	db, err := sql.Open("sqlite3", dbName)
	fmt.Println("Opened database file at", dbName)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	_, err = db.Exec(sqlStmt)
	if err != nil {
		log.Fatal(err)
	}
	return dbName
}

func CreateLexiconDatabase(lexiconName string, lexiconInfo lexicon.LexiconInfo,
	lexSymbols []LexiconSymbolDefinition, lexMap LexiconMap,
	outputDir string) {
	fmt.Println("Creating lexicon database", lexiconName)
	definitions, alphagrams := populateAlphsDefs(lexiconInfo.LexiconFilename,
		lexiconInfo.Combinations, lexiconInfo.LetterDistribution)
	fmt.Println("Sorting by probability")
	alphs := alphaMapValues(alphagrams)
	sort.Sort(AlphByCombos(alphs))

	var probs [16]uint32
	for i := 0; i < 16; i++ {
		probs[i] = 0
	}

	dbName := createSqliteDb(outputDir, lexiconName)

	alphInsertQuery := `
	INSERT INTO alphagrams(probability, alphagram, length, combinations,
		num_anagrams, point_value, num_vowels)
	VALUES (?, ?, ?, ?, ?, ?, ?)`
	wordInsertQuery := `
	INSERT INTO words (word, alphagram, lexicon_symbols, definition,
		front_hooks, back_hooks, inner_front_hook, inner_back_hook)
	VALUES(?, ?, ?, ?, ?, ?, ?, ?)`

	db, err := sql.Open("sqlite3", dbName)
	if err != nil {
		log.Fatal(err)
	}
	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}

	alphStmt, err := tx.Prepare(alphInsertQuery)
	if err != nil {
		log.Fatal(err)
	}
	wordStmt, err := tx.Prepare(wordInsertQuery)
	if err != nil {
		log.Fatal(err)
	}
	defer alphStmt.Close()
	defer wordStmt.Close()
	gd := lexiconInfo.Gaddag
	for idx, alph := range alphs {
		if idx%10000 == 0 {
			log.Println(idx, "...")
		}
		wl := len([]rune(alph.alphagram))
		if wl <= 15 {
			probs[wl]++
		}
		_, err = alphStmt.Exec(probs[wl], alph.alphagram, wl, alph.combinations,
			len(alph.words), alph.pointValue(lexiconInfo.LetterDistribution),
			alph.numVowels())
		if err != nil {
			log.Fatal(err)
		}
		for _, word := range alph.words {
			if err != nil {
				log.Fatal(err)
			}

			backHooks := sortedHooks(gaddag.FindHooks(gd, word, gaddag.BackHooks),
				lexiconInfo.LetterDistribution)
			frontHooks := sortedHooks(gaddag.FindHooks(gd, word, gaddag.FrontHooks),
				lexiconInfo.LetterDistribution)
			frontInnerHook := 0
			backInnerHook := 0
			if gaddag.FindInnerHook(gd, word, gaddag.BackInnerHook) {
				backInnerHook = 1
			}
			if gaddag.FindInnerHook(gd, word, gaddag.FrontInnerHook) {
				frontInnerHook = 1
			}

			def := definitions[word]
			alphagram := alph.alphagram

			wordStmt.Exec(
				word, alphagram,
				findLexSymbols(word, lexiconName, lexMap, lexSymbols), def,
				frontHooks, backHooks, frontInnerHook, backInnerHook)
		}
	}
	tx.Commit()

	_, err = db.Exec("INSERT INTO db_version(version) VALUES(?)", CurrentVersion)
	if err != nil {
		log.Fatal(err)
	}

}

// FixLexiconDatabase assumes the database has already been created with
// a previous version of this program. At the minimum, the schema looks like:
// sqlStmt := `
// CREATE TABLE alphagrams (probability int, alphagram varchar(20),
//     length int, combinations int, num_anagrams int);

// CREATE TABLE words (word varchar(20), alphagram varchar(20),
//     lexicon_symbols varchar(5), definition varchar(512),
//     front_hooks varchar(26), back_hooks varchar(26),
//     inner_front_hook int, inner_back_hook int);

// CREATE INDEX alpha_index on alphagrams(alphagram);
// CREATE INDEX prob_index on alphagrams(probability, length);
// CREATE INDEX word_index on words(word);
// CREATE INDEX alphagram_index on words(alphagram);
// `
// This function assumes the above schema.
func FixLexiconDatabase(lexiconName string, lexiconInfo lexicon.LexiconInfo) {
	dbName := "./" + lexiconName + ".db"

	db, err := sql.Open("sqlite3", dbName)
	if err != nil {
		log.Fatal(err)
	}
	var version int
	err = db.QueryRow("SELECT version FROM db_version").Scan(&version)
	switch {
	case err == sql.ErrNoRows:
		log.Fatal("There is a version table but it has no values in it")
	case err != nil:
		if err.Error() == "no such table: db_version" {
			log.Printf("No version table, creating one...")
			_, err = db.Exec("CREATE TABLE db_version (version integer)")
			if err != nil {
				log.Fatal(err)
			}
			_, err = db.Exec("INSERT INTO db_version(version) VALUES(?)", 1)
			if err != nil {
				log.Fatal(err)
			}
			version = 1
		} else {
			log.Fatal(err)
		}
	default:
		if version == CurrentVersion {
			fmt.Printf("DB Version is up to date (version %d)\n", version)
		} else {
			fmt.Printf("Version of this table is %d, moving to %d\n", version,
				version+1)
		}
	}

	if version == 1 {
		fmt.Println("Migrating to version 2...")
		migrateToV2(db, lexiconInfo.LetterDistribution)
		fmt.Println("Run again to migrate to version 3")
	}
	if version == 2 {
		fmt.Printf("Migrating to version 3...")
		migrateToV3(db)
	}

}

func migrateToV2(db *sql.DB, dist lexicon.LetterDistribution) {
	// Version 2 has the following improvements:
	// An index on point value, and point value
	// An index on num anagrams, and num anagrams
	// An index on num vowels, and num vowels

	_, err := db.Exec(`
			ALTER TABLE alphagrams ADD COLUMN num_anagrams int;
			ALTER TABLE alphagrams ADD COLUMN point_value int;
			ALTER TABLE alphagrams ADD COLUMN num_vowels int;

			CREATE INDEX num_anagrams_index on alphagrams(num_anagrams);
			CREATE INDEX point_value_index on alphagrams(point_value);
			CREATE INDEX num_vowels_index on alphagrams(num_vowels);
			`)
	if err != nil {
		log.Fatal(err)
	}

	// Read in all the alphagrams.
	rows, err := db.Query(`
			SELECT words.alphagram, count() AS word_ct FROM words
			INNER JOIN alphagrams on words.alphagram = alphagrams.alphagram
			GROUP BY words.alphagram
			`)
	if err != nil {
		log.Fatal(err)
	}
	defer rows.Close()

	tx, err := db.Begin()
	if err != nil {
		log.Fatal(err)
	}

	updateQuery := `
		UPDATE alphagrams SET num_anagrams = ?, point_value = ?, num_vowels = ?
		WHERE alphagram = ?
	`

	alphagrams := []Alphagram{}
	// Read all the rows and update alphagrams.
	for rows.Next() {
		var (
			alph      string
			wordCount int
		)
		if err := rows.Scan(&alph, &wordCount); err != nil {
			log.Fatal(err)
		}
		alphagrams = append(alphagrams, Alphagram{alphagram: alph,
			wordCount: uint8(wordCount)})
	}

	i := 0
	updateStmt, err := tx.Prepare(updateQuery)
	for _, alph := range alphagrams {
		_, err := updateStmt.Exec(alph.wordCount, alph.pointValue(dist),
			alph.numVowels(), alph.alphagram)
		if err != nil {
			log.Fatal(err)
		}
		i += 1
		if i%10000 == 0 {
			log.Printf("%d...", i)
		}
	}
	tx.Commit()

	_, err = db.Exec("UPDATE db_version SET version = ?", 2)
	if err != nil {
		log.Fatal(err)
	}
}

func migrateToV3(db *sql.DB) {
	_, err := db.Exec("CREATE INDEX length_index on alphagrams(length);")
	if err != nil {
		log.Fatal(err)
	}
	_, err = db.Exec("UPDATE db_version SET version = ?", 3)
	if err != nil {
		log.Fatal(err)
	}
}

func sortedHooks(hooks []rune, dist lexicon.LetterDistribution) string {
	w := lexicon.Word{Word: string(hooks), Dist: dist}
	return w.MakeAlphagram()
}

func findLexSymbols(word string, lexiconName string, lexMap LexiconMap,
	lexSymbols []LexiconSymbolDefinition) string {

	symbols := ""

	for _, def := range lexSymbols {
		if lexiconName == def.In {
			lex := lexMap[def.NotIn]
			if lex.Gaddag.GetAlphabet() != nil &&
				!gaddag.FindWord(lex.Gaddag, word) &&
				!strings.Contains(symbols, def.Symbol) {
				symbols += def.Symbol
			}
		}
	}
	return symbols
}

// The values of the map.
func alphaMapValues(theMap map[string]Alphagram) []Alphagram {
	x := make([]Alphagram, len(theMap))
	i := 0
	for _, value := range theMap {
		x[i] = value
		i++
	}
	return x
}

func populateAlphsDefs(filename string, combinations func(string, bool) uint64,
	dist lexicon.LetterDistribution) (
	map[string]string, map[string]Alphagram) {
	definitions := make(map[string]string)
	alphagrams := make(map[string]Alphagram)
	file, _ := os.Open(filename)
	// XXX: Check error
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) > 0 {
			word := lexicon.Word{Word: strings.ToUpper(fields[0]), Dist: dist}
			definition := ""
			if len(fields) > 1 {
				definition = strings.Join(fields[1:], " ")
			}
			definitions[word.Word] = definition
			alphagram := word.MakeAlphagram()
			alph, ok := alphagrams[alphagram]
			if !ok {
				alphagrams[alphagram] = Alphagram{
					[]string{word.Word},
					combinations(alphagram, true),
					alphagram, 0}
			} else {
				alph.words = append(alph.words, word.Word)
				alphagrams[alphagram] = alph
			}
		}
	}
	file.Close()
	return definitions, alphagrams
}
