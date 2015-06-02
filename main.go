// The caller of the db creator.
package main

import (
	//"database/sql"
	//_ "github.com/mattn/go-sqlite3"
	//"fmt"
	"github.com/domino14/word_db_maker/dbmaker"
)

func main() {
	//db, err := sql.Open("sqlite3", "./"+lexname+".db")
	symbols := []dbmaker.LexiconSymbolDefinition{
		{In: "America", NotIn: "CSW12", Symbol: "$"},
		{In: "CSW12", NotIn: "OWL2", Symbol: "#"},
		{In: "America", NotIn: "OWL2", Symbol: "+"},
	}
	lexiconMap := dbmaker.LexiconMap{
		"OWL2": &dbmaker.LexiconInfo{
			LexiconName:        "OWL2",
			LexiconFilename:    "/Users/cesar/coding/webolith/words/OWL2.txt",
			LexiconIndex:       4,
			DescriptiveName:    "American 06",
			LetterDistribution: dbmaker.EnglishLetterDistribution(),
		},
		"CSW12": &dbmaker.LexiconInfo{
			LexiconName:        "CSW12",
			LexiconFilename:    "/Users/cesar/coding/webolith/words/CSW12.txt",
			LexiconIndex:       6,
			DescriptiveName:    "Collins 12",
			LetterDistribution: dbmaker.EnglishLetterDistribution(),
		},
		"America": &dbmaker.LexiconInfo{
			LexiconName:        "America",
			LexiconFilename:    "/Users/cesar/coding/webolith/words/America.txt",
			LexiconIndex:       7,
			DescriptiveName:    "I am America, and so can you.",
			LetterDistribution: dbmaker.EnglishLetterDistribution(),
		},
	}
	for name, info := range lexiconMap {
		info.Initialize()
		dbmaker.CreateLexiconDatabase(name, info, symbols, lexiconMap)
	}
}
