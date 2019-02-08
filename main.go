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
	"github.com/domino14/word_db_maker/dbmaker"
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
		{In: "CSW15", NotIn: "America", Symbol: "#"},
		{In: "CSW15", NotIn: "CSW12", Symbol: "+"},
	}
	// set LEXICON_PATH to something.
	// For example "/Users/cesar/coding/webolith/words/" on my computer.
	lexiconPrefix := os.Getenv("LEXICON_PATH")
	gaddagPrefix := os.Getenv("GADDAG_PATH")
	lexiconMap := dbmaker.LexiconMap{
		// Pregenerate these gaddags with macondo/gaddag package.
		"CSW12": dbmaker.LexiconInfo{
			LexiconName:        "CSW12",
			LexiconFilename:    lexiconPrefix + "CSW12.txt",
			Gaddag:             gaddag.LoadGaddag(gaddagPrefix + "CSW12.gaddag"),
			LexiconIndex:       1,
			DescriptiveName:    "Collins 12",
			LetterDistribution: alphabet.EnglishLetterDistribution(),
		},
		"CSW15": dbmaker.LexiconInfo{
			LexiconName:        "CSW15",
			LexiconFilename:    lexiconPrefix + "CSW15.txt",
			Gaddag:             gaddag.LoadGaddag(gaddagPrefix + "CSW15.gaddag"),
			LexiconIndex:       1,
			DescriptiveName:    "Collins 15",
			LetterDistribution: alphabet.EnglishLetterDistribution(),
		},
		"America": dbmaker.LexiconInfo{
			LexiconName:        "America",
			LexiconFilename:    lexiconPrefix + "America.txt",
			Gaddag:             gaddag.LoadGaddag(gaddagPrefix + "America.gaddag"),
			LexiconIndex:       7,
			DescriptiveName:    "I am America, and so can you.",
			LetterDistribution: alphabet.EnglishLetterDistribution(),
		},
		"FISE09": dbmaker.LexiconInfo{
			LexiconName:        "FISE09",
			LexiconFilename:    lexiconPrefix + "FISE09.txt",
			Gaddag:             gaddag.LoadGaddag(gaddagPrefix + "FISE09.gaddag"),
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
	}
	if dbToMigrate != "" {
		info, ok := lexiconMap[dbToMigrate]
		if !ok {
			fmt.Printf("That lexicon is not supported\n")
			return
		}
		dbmaker.MigrateLexiconDatabase(dbToMigrate, info)
	} else {
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
			if info.Gaddag.GetAlphabet() == nil {
				fmt.Println(name, "was not supplied, skipping...")
				continue
			}
			info.Initialize()
			dbmaker.CreateLexiconDatabase(name, info, symbols, lexiconMap,
				outputDir)
		}
	}
}
