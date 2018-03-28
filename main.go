// The caller of the db creator.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/domino14/macondo/gaddag"
	"github.com/domino14/macondo/lexicon"
	"github.com/domino14/word_db_maker/dbmaker"
)

func main() {
	// We are going to have a flag to "fix" a database. This is due to a
	// legacy issue where alphagram sort order was not deterministic for
	// alphagrams with equal probability, so we need to keep the old
	// sort orders around in order to not mess up alphagrams-by-probability
	// lists.
	var fixdb = flag.String("fixdb", "", "Fix a DB instead of generating it")
	var outputDirF = flag.String("outputdir", ".", "The output directory")
	flag.Parse()
	dbToFix := *fixdb
	outputDir := *outputDirF
	//db, err := sql.Open("sqlite3", "./"+lexname+".db")
	symbols := []dbmaker.LexiconSymbolDefinition{
		{In: "America2016", NotIn: "CSW15", Symbol: "$"},
		{In: "America2016", NotIn: "America", Symbol: "%"},
		{In: "America2016", NotIn: "OWL2", Symbol: "+"},
		{In: "America", NotIn: "CSW15", Symbol: "$"},
		{In: "America", NotIn: "OWL2", Symbol: "+"},
		{In: "CSW15", NotIn: "America2016", Symbol: "#"},
		{In: "CSW15", NotIn: "America", Symbol: "#"},
		{In: "CSW15", NotIn: "CSW12", Symbol: "+"},
	}
	// set LEXICON_PATH to something.
	// For example "/Users/cesar/coding/webolith/words/" on my computer.
	lexiconPrefix := os.Getenv("LEXICON_PATH")
	gaddagPrefix := os.Getenv("GADDAG_PATH")
	lexiconMap := dbmaker.LexiconMap{
		// Pregenerate these gaddags with macondo/gaddag package.
		"OWL2": lexicon.LexiconInfo{
			LexiconName:        "OWL2",
			LexiconFilename:    lexiconPrefix + "OWL2.txt",
			Gaddag:             gaddag.LoadGaddag(gaddagPrefix + "OWL2.gaddag"),
			LexiconIndex:       4,
			DescriptiveName:    "American 06",
			LetterDistribution: lexicon.EnglishLetterDistribution(),
		},
		"CSW12": lexicon.LexiconInfo{
			LexiconName:        "CSW12",
			LexiconFilename:    lexiconPrefix + "CSW12.txt",
			Gaddag:             gaddag.LoadGaddag(gaddagPrefix + "CSW12.gaddag"),
			LexiconIndex:       6,
			DescriptiveName:    "Collins 12",
			LetterDistribution: lexicon.EnglishLetterDistribution(),
		},
		"CSW15": lexicon.LexiconInfo{
			LexiconName:        "CSW15",
			LexiconFilename:    lexiconPrefix + "CSW15.txt",
			Gaddag:             gaddag.LoadGaddag(gaddagPrefix + "CSW15.gaddag"),
			LexiconIndex:       1,
			DescriptiveName:    "Collins 15",
			LetterDistribution: lexicon.EnglishLetterDistribution(),
		},
		"America": lexicon.LexiconInfo{
			LexiconName:        "America",
			LexiconFilename:    lexiconPrefix + "America.txt",
			Gaddag:             gaddag.LoadGaddag(gaddagPrefix + "America.gaddag"),
			LexiconIndex:       7,
			DescriptiveName:    "I am America, and so can you.",
			LetterDistribution: lexicon.EnglishLetterDistribution(),
		},
		"America2016": lexicon.LexiconInfo{
			LexiconName:        "America2016",
			LexiconFilename:    lexiconPrefix + "America2016.txt",
			Gaddag:             gaddag.LoadGaddag(gaddagPrefix + "America2016.gaddag"),
			LexiconIndex:       2,
			DescriptiveName:    "I am Trumperica, and so can you.",
			LetterDistribution: lexicon.EnglishLetterDistribution(),
		},
		"FISE09": lexicon.LexiconInfo{
			LexiconName:        "FISE09",
			LexiconFilename:    lexiconPrefix + "FISE09.txt",
			Gaddag:             gaddag.LoadGaddag(gaddagPrefix + "FISE09.gaddag"),
			LexiconIndex:       8,
			DescriptiveName:    "Federación Internacional de Scrabble en Español",
			LetterDistribution: lexicon.SpanishLetterDistribution(),
		},
	}
	if dbToFix != "" {
		info, ok := lexiconMap[dbToFix]
		if !ok {
			fmt.Printf("That lexicon is not supported\n")
			return
		}
		dbmaker.FixLexiconDatabase(dbToFix, info)
	} else {
		for name, info := range lexiconMap {
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
