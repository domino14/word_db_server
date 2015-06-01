package main

import (
//"database/sql"
//_ "github.com/mattn/go-sqlite3"
//"fmt"
)

func main() {
	//db, err := sql.Open("sqlite3", "./"+lexname+".db")
	lexInfo := LexiconInfo{
		lexiconName:        "OWL2",
		gaddagFilename:     "./blah.gaddag",
		lexiconIndex:       4,
		descriptiveName:    "American 06",
		letterDistribution: EnglishLetterDistribution()}
	lexInfo.Initialize()
}
