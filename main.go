package main

import (
	//"database/sql"
	//_ "github.com/mattn/go-sqlite3"
	"fmt"
)

func main() {
	//db, err := sql.Open("sqlite3", "./"+lexname+".db")
	lexiconMap := map[string]*LexiconInfo{
		"OWL2": &LexiconInfo{
			lexiconName:        "OWL2",
			gaddagFilename:     "./blah.gaddag",
			lexiconIndex:       4,
			descriptiveName:    "American 06",
			letterDistribution: EnglishLetterDistribution(),
		},
		"CSW12": &LexiconInfo{
			lexiconName:        "CSW12",
			gaddagFilename:     "./blah.gaddag",
			lexiconIndex:       6,
			descriptiveName:    "Collins 12",
			letterDistribution: EnglishLetterDistribution(),
		},
		"America": &LexiconInfo{
			lexiconName:        "America",
			gaddagFilename:     "./blah.gaddag",
			lexiconIndex:       7,
			descriptiveName:    "I am America, and so can you.",
			letterDistribution: EnglishLetterDistribution(),
		},
	}
	lexiconMap["OWL2"].Initialize()
	fmt.Println(lexiconMap["OWL2"].Combinations("EMILY"))
}
