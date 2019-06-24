// The caller of the db creator.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/domino14/macondo/gaddagmaker"

	"github.com/domino14/macondo/gaddag"

	"github.com/domino14/macondo/alphabet"
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

var LexiconPrefix = os.Getenv("LEXICON_PATH")

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
	//db, err := sql.Open("sqlite3", "./"+lexname+".db")
	symbols := []dbmaker.LexiconSymbolDefinition{
		{In: "NWL18", NotIn: "CSW19", Symbol: "$"},
		{In: "NWL18", NotIn: "America", Symbol: "+"},
		{In: "CSW19", NotIn: "NWL18", Symbol: "#"},
		{In: "FISE2", NotIn: "FISE09", Symbol: "+"},
		{In: "CSW19", NotIn: "CSW15", Symbol: "+"},
	}
	// set LEXICON_PATH to something.
	// For example "/Users/cesar/coding/webolith/words/" on my computer.
	lexiconMap := dbmaker.LexiconMap{
		// Pregenerate these gaddags with macondo/gaddag package.
		"CSW12": dbmaker.LexiconInfo{
			LexiconName:        "CSW12",
			LexiconFilename:    filepath.Join(LexiconPrefix, "CSW12.txt"),
			Gaddag:             loadOrMakeGaddag("CSW12"),
			LexiconIndex:       1,
			DescriptiveName:    "Collins 12",
			LetterDistribution: alphabet.EnglishLetterDistribution(),
		},
		"CSW15": dbmaker.LexiconInfo{
			LexiconName:        "CSW15",
			LexiconFilename:    filepath.Join(LexiconPrefix, "CSW15.txt"),
			Gaddag:             loadOrMakeGaddag("CSW15"),
			LexiconIndex:       1,
			DescriptiveName:    "Collins 15",
			LetterDistribution: alphabet.EnglishLetterDistribution(),
		},
		"CSW19": dbmaker.LexiconInfo{
			LexiconName:        "CSW19",
			LexiconFilename:    filepath.Join(LexiconPrefix, "CSW19.txt"),
			Gaddag:             loadOrMakeGaddag("CSW19"),
			LexiconIndex:       12,
			DescriptiveName:    "Collins 2019",
			LetterDistribution: alphabet.EnglishLetterDistribution(),
		},
		"America": dbmaker.LexiconInfo{
			LexiconName:        "America",
			LexiconFilename:    filepath.Join(LexiconPrefix, "America.txt"),
			Gaddag:             loadOrMakeGaddag("America"),
			LexiconIndex:       7,
			DescriptiveName:    "I am America, and so can you.",
			LetterDistribution: alphabet.EnglishLetterDistribution(),
		},
		"FISE09": dbmaker.LexiconInfo{
			LexiconName:        "FISE09",
			LexiconFilename:    filepath.Join(LexiconPrefix, "FISE09.txt"),
			Gaddag:             loadOrMakeGaddag("FISE09"),
			LexiconIndex:       8,
			DescriptiveName:    "Federación Internacional de Scrabble en Español",
			LetterDistribution: alphabet.SpanishLetterDistribution(),
		},
		"FISE2": dbmaker.LexiconInfo{
			LexiconName:        "FISE2",
			LexiconFilename:    filepath.Join(LexiconPrefix, "FISE2.txt"),
			Gaddag:             loadOrMakeGaddag("FISE2"),
			LexiconIndex:       10,
			DescriptiveName:    "Federación Internacional de Scrabble en Español, 2017 Edition",
			LetterDistribution: alphabet.SpanishLetterDistribution(),
		},
		"NWL18": dbmaker.LexiconInfo{
			LexiconName:        "NWL18",
			LexiconFilename:    filepath.Join(LexiconPrefix, "NWL18.txt"),
			Gaddag:             loadOrMakeGaddag("NWL18"),
			LexiconIndex:       9,
			DescriptiveName:    "NASPA Word List, 2018 Edition",
			LetterDistribution: alphabet.EnglishLetterDistribution(),
		},
		"OSPS40": dbmaker.LexiconInfo{
			LexiconName:        "OSPS40",
			LexiconFilename:    filepath.Join(LexiconPrefix, "OSPS40.txt"),
			Gaddag:             loadOrMakeGaddag("OSPS40"),
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

/*
   GoLang: os.Rename() give error "invalid cross-device link" for Docker
   container with Volumes.
   MoveFile(source, destination) will work moving file between folders
   https://gist.github.com/var23rav/23ae5d0d4d830aff886c3c970b8f6c6b
*/
func MoveFile(sourcePath, destPath string) error {
	inputFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("Couldn't open source file: %s", err)
	}
	outputFile, err := os.Create(destPath)
	if err != nil {
		inputFile.Close()
		return fmt.Errorf("Couldn't open dest file: %s", err)
	}
	defer outputFile.Close()
	_, err = io.Copy(outputFile, inputFile)
	inputFile.Close()
	if err != nil {
		return fmt.Errorf("Writing to output file failed: %s", err)
	}
	// The copy was successful, so now delete the original file
	err = os.Remove(sourcePath)
	if err != nil {
		return fmt.Errorf("Failed removing original file: %s", err)
	}
	return nil
}

func loadOrMakeGaddag(lexiconName string) *gaddag.SimpleGaddag {
	possibleGaddag := filepath.Join(LexiconPrefix, "gaddag", lexiconName+".gaddag")
	sg := gaddag.LoadGaddag(possibleGaddag)
	if sg != nil {
		return sg
	}
	// Otherwise, build it.
	lexiconFilename := filepath.Join(LexiconPrefix, lexiconName+".txt")
	gd := gaddagmaker.GenerateGaddag(lexiconFilename, false, true)
	if gd.Root == nil {
		// Gaddag could not be generated at all, maybe lexicon is missing.
		return nil
	}
	// Otherwise, rename file
	err := MoveFile("out.gaddag", possibleGaddag)
	if err != nil {
		panic(err)
	}
	// It should exist now.
	return gaddag.LoadGaddag(possibleGaddag)
}
