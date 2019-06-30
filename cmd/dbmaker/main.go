// The caller of the db creator.
package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/domino14/word_db_server/dbmaker"
)

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func main() {
	// We are going to have a flag to migrate a database. This is due to a
	// legacy issue where alphagram sort order was not deterministic for
	// alphagrams with equal probability, so we need to keep the old
	// sort orders around in order to not mess up alphagrams-by-probability
	// lists.
	var migratedb = flag.String("migratedb", "", "Migrate a DB instead of generating it")
	var createdbs = flag.String("dbs", "", "Pass in comma-separated list of dbs to make, instead of all")
	var forcecreation = flag.Bool("force", false, "Create DB even if it already exists (overwrite)")
	var dbToFixDefs = flag.String("fixdefs", "",
		"Pass in lexicon name to fix definitions on. DB <lexiconname>.db must exist in this dir.")
	var dbToFixSymbols = flag.String("fixsymbols", "",
		"Pass in lexicon name to fix lexicon symbols on. DB <lexiconname>.db must exist in this dir.")

	var outputDirF = flag.String("outputdir", ".", "The output directory")

	flag.Parse()
	dbToMigrate := *migratedb
	dbsToMake := *createdbs
	outputDir := *outputDirF
	force := *forcecreation

	symbols, lexiconMap := dbmaker.LexiconMappings()

	if dbToMigrate != "" {
		info, ok := lexiconMap[dbToMigrate]
		if !ok {
			fmt.Printf("That lexicon is not supported\n")
			return
		}
		dbmaker.MigrateLexiconDatabase(dbToMigrate, info)
	} else if *dbToFixDefs != "" {
		fixDefinitions(*dbToFixDefs, lexiconMap)
	} else if *dbToFixSymbols != "" {
		fixSymbols(*dbToFixSymbols, lexiconMap, symbols)
	} else {
		makeDbs(dbsToMake, lexiconMap, symbols, outputDir, force)
	}
}

func fixDefinitions(dbToFixDefs string, lexiconMap dbmaker.LexiconMap) {
	// open existing databases but new dictionary files/gaddags etc
	// and apply new definitions
	dbmaker.FixDefinitions(dbToFixDefs, lexiconMap)
}

func fixSymbols(dbToFixSymbols string, lexiconMap dbmaker.LexiconMap,
	symbols []dbmaker.LexiconSymbolDefinition) {

	// open existing databases but new dictionary files/gaddags etc
	// and apply lex symbols.
	dbmaker.FixLexiconSymbols(dbToFixSymbols, lexiconMap, symbols)
}

func makeDbs(dbsToMake string, lexiconMap dbmaker.LexiconMap,
	symbols []dbmaker.LexiconSymbolDefinition, outputDir string,
	forceCreation bool) {

	dbs := []string{}
	if dbsToMake != "" {
		dbs = strings.Split(dbsToMake, ",")
	} else {
		for name := range lexiconMap {
			dbs = append(dbs, name)
		}
	}
	for name, info := range lexiconMap {
		if !stringInSlice(name, dbs) {
			fmt.Println(name, "was not in list of dbs, skipping...")
			continue
		}
		if info.Gaddag == nil || info.Gaddag.GetAlphabet() == nil {
			fmt.Println(name, "was not supplied, skipping...")
			continue
		}
		info.Initialize()
		dbmaker.CreateLexiconDatabase(name, info, symbols, lexiconMap,
			outputDir, !forceCreation)
	}
}
