// The caller of the db creator.
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/domino14/macondo/alphabet"
	"github.com/domino14/macondo/gaddag"
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
	var dbToFixDefs = flag.String("fixdefs", "",
		"Pass in lexicon name to fix definitions on. DB <lexiconname>.db must exist in this dir.")
	var dbToFixSymbols = flag.String("fixsymbols", "",
		"Pass in lexicon name to fix lexicon symbols on. DB <lexiconname>.db must exist in this dir.")

	var outputDirF = flag.String("outputdir", ".", "The output directory")

	flag.Parse()
	dbToMigrate := *migratedb
	dbsToMake := *createdbs
	outputDir := *outputDirF
	//db, err := sql.Open("sqlite3", "./"+lexname+".db")
	symbols := []dbmaker.LexiconSymbolDefinition{
		{In: "NWL18", NotIn: "CSW15", Symbol: "$"},
		{In: "NWL18", NotIn: "America", Symbol: "+"},
		{In: "CSW15", NotIn: "NWL18", Symbol: "#"},
		{In: "FISE2", NotIn: "FISE09", Symbol: "+"},
		{In: "CSW15", NotIn: "CSW12", Symbol: "+"},
	}
	// set LEXICON_PATH to something.
	// For example "/Users/cesar/coding/webolith/words/" on my computer.
	lexiconPrefix := os.Getenv("LEXICON_PATH")
	gaddagPrefix := filepath.Join(lexiconPrefix, "gaddag")
	lexiconMap := dbmaker.LexiconMap{
		// Pregenerate these gaddags with macondo/gaddag package.
		"CSW12": dbmaker.LexiconInfo{
			LexiconName:        "CSW12",
			LexiconFilename:    filepath.Join(lexiconPrefix, "CSW12.txt"),
			Gaddag:             gaddag.LoadGaddag(filepath.Join(gaddagPrefix, "CSW12.gaddag")),
			LexiconIndex:       1,
			DescriptiveName:    "Collins 12",
			LetterDistribution: alphabet.EnglishLetterDistribution(),
		},
		"CSW15": dbmaker.LexiconInfo{
			LexiconName:        "CSW15",
			LexiconFilename:    filepath.Join(lexiconPrefix, "CSW15.txt"),
			Gaddag:             gaddag.LoadGaddag(filepath.Join(gaddagPrefix, "CSW15.gaddag")),
			LexiconIndex:       1,
			DescriptiveName:    "Collins 15",
			LetterDistribution: alphabet.EnglishLetterDistribution(),
		},
		"America": dbmaker.LexiconInfo{
			LexiconName:        "America",
			LexiconFilename:    filepath.Join(lexiconPrefix, "America.txt"),
			Gaddag:             gaddag.LoadGaddag(filepath.Join(gaddagPrefix, "America.gaddag")),
			LexiconIndex:       7,
			DescriptiveName:    "I am America, and so can you.",
			LetterDistribution: alphabet.EnglishLetterDistribution(),
		},
		"FISE09": dbmaker.LexiconInfo{
			LexiconName:        "FISE09",
			LexiconFilename:    filepath.Join(lexiconPrefix, "FISE09.txt"),
			Gaddag:             gaddag.LoadGaddag(filepath.Join(gaddagPrefix, "FISE09.gaddag")),
			LexiconIndex:       8,
			DescriptiveName:    "Federación Internacional de Scrabble en Español",
			LetterDistribution: alphabet.SpanishLetterDistribution(),
		},
		"FISE2": dbmaker.LexiconInfo{
			LexiconName:        "FISE2",
			LexiconFilename:    filepath.Join(lexiconPrefix, "FISE2.txt"),
			Gaddag:             gaddag.LoadGaddag(filepath.Join(gaddagPrefix, "FISE2.gaddag")),
			LexiconIndex:       10,
			DescriptiveName:    "Federación Internacional de Scrabble en Español, 2017 Edition",
			LetterDistribution: alphabet.SpanishLetterDistribution(),
		},
		"NWL18": dbmaker.LexiconInfo{
			LexiconName:        "NWL18",
			LexiconFilename:    filepath.Join(lexiconPrefix, "NWL18.txt"),
			Gaddag:             gaddag.LoadGaddag(filepath.Join(gaddagPrefix, "NWL18.gaddag")),
			LexiconIndex:       9,
			DescriptiveName:    "NASPA Word List, 2018 Edition",
			LetterDistribution: alphabet.EnglishLetterDistribution(),
		},
		"OSPS40": dbmaker.LexiconInfo{
			LexiconName:        "OSPS40",
			LexiconFilename:    filepath.Join(lexiconPrefix, "OSPS40.txt"),
			Gaddag:             gaddag.LoadGaddag(filepath.Join(gaddagPrefix, "OSPS40.gaddag")),
			LexiconIndex:       11,
			DescriptiveName:    "Polska Federacja Scrabble - Update 40",
			LetterDistribution: alphabet.PolishLetterDistribution(),
		},
	}
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
		makeDbs(dbsToMake, lexiconMap, symbols, outputDir)
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
	symbols []dbmaker.LexiconSymbolDefinition, outputDir string) {

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
			outputDir)
	}
}
