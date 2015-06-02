package main

import (
//"database/sql"
//_ "github.com/mattn/go-sqlite3"
//"fmt"
)

func main() {
	//db, err := sql.Open("sqlite3", "./"+lexname+".db")
	symbols := []LexiconSymbolDefinition{
		{in: "America", notin: "CSW12", symbol: "$"},
		{in: "CSW12", notin: "OWL2", symbol: "#"},
		{in: "America", notin: "OWL2", symbol: "+"},
	}
	lexiconMap := LexiconMap{
		"OWL2": &LexiconInfo{
			lexiconName:        "OWL2",
			lexiconFilename:    "/Users/cesar/coding/webolith/words/OWL2.txt",
			lexiconIndex:       4,
			descriptiveName:    "American 06",
			letterDistribution: EnglishLetterDistribution(),
		},
		"CSW12": &LexiconInfo{
			lexiconName:        "CSW12",
			lexiconFilename:    "/Users/cesar/coding/webolith/words/CSW12.txt",
			lexiconIndex:       6,
			descriptiveName:    "Collins 12",
			letterDistribution: EnglishLetterDistribution(),
		},
		"America": &LexiconInfo{
			lexiconName:        "America",
			lexiconFilename:    "/Users/cesar/coding/webolith/words/America.txt",
			lexiconIndex:       7,
			descriptiveName:    "I am America, and so can you.",
			letterDistribution: EnglishLetterDistribution(),
		},
	}
	for name, info := range lexiconMap {
		info.Initialize()
		CreateLexiconDatabase(name, info, symbols, lexiconMap)
	}
}
