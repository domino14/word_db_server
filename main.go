// The caller of the db creator.
package main

import (
	"github.com/domino14/macondo/gaddag"
	"github.com/domino14/macondo/lexicon"
	"github.com/domino14/word_db_maker/dbmaker"
)

func main() {
	//db, err := sql.Open("sqlite3", "./"+lexname+".db")
	symbols := []dbmaker.LexiconSymbolDefinition{
		{In: "America", NotIn: "CSW15", Symbol: "$"},
		{In: "America", NotIn: "OWL2", Symbol: "+"},
		{In: "CSW15", NotIn: "America", Symbol: "#"},
		{In: "CSW15", NotIn: "CSW12", Symbol: "+"},
	}
	lexiconMap := dbmaker.LexiconMap{
		// Pregenerate these gaddags with macondo/gaddag package.
		"OWL2": lexicon.LexiconInfo{
			LexiconName:        "OWL2",
			LexiconFilename:    "/Users/cesar/coding/webolith/words/OWL2.txt",
			Gaddag:             gaddag.LoadGaddag("/Users/cesar/coding/webolith/words/OWL2.gaddag"),
			LexiconIndex:       4,
			DescriptiveName:    "American 06",
			LetterDistribution: lexicon.EnglishLetterDistribution(),
		},
		"CSW12": lexicon.LexiconInfo{
			LexiconName:        "CSW12",
			LexiconFilename:    "/Users/cesar/coding/webolith/words/CSW12.txt",
			Gaddag:             gaddag.LoadGaddag("/Users/cesar/coding/webolith/words/CSW12.gaddag"),
			LexiconIndex:       6,
			DescriptiveName:    "Collins 12",
			LetterDistribution: lexicon.EnglishLetterDistribution(),
		},
		"CSW15": lexicon.LexiconInfo{
			LexiconName:        "CSW12",
			LexiconFilename:    "/Users/cesar/coding/webolith/words/CSW15.txt",
			Gaddag:             gaddag.LoadGaddag("/Users/cesar/coding/webolith/words/CSW15.gaddag"),
			LexiconIndex:       1,
			DescriptiveName:    "Collins 15",
			LetterDistribution: lexicon.EnglishLetterDistribution(),
		},
		"America": lexicon.LexiconInfo{
			LexiconName:        "America",
			LexiconFilename:    "/Users/cesar/coding/webolith/words/America.txt",
			Gaddag:             gaddag.LoadGaddag("/Users/cesar/coding/webolith/words/America.gaddag"),
			LexiconIndex:       7,
			DescriptiveName:    "I am America, and so can you.",
			LetterDistribution: lexicon.EnglishLetterDistribution(),
		},
		"FISE": lexicon.LexiconInfo{
			LexiconName:        "FISE09",
			LexiconFilename:    "/Users/cesar/coding/webolith/words/FISE.txt",
			Gaddag:             gaddag.LoadGaddag("/Users/cesar/coding/webolith/words/FISE.gaddag"),
			LexiconIndex:       8,
			DescriptiveName:    "Federación Internacional de Scrabble en Español",
			LetterDistribution: lexicon.SpanishLetterDistribution(),
		},
	}
	for name, info := range lexiconMap {
		info.Initialize()
		dbmaker.CreateLexiconDatabase(name, info, symbols, lexiconMap)
	}
}
