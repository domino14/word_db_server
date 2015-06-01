package main

import (
	//"database/sql"
	//_ "github.com/mattn/go-sqlite3"
	"fmt"
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
	fmt.Println(lexInfo.subChooseCombos)
	fmt.Println(lexInfo.Combinations("AADEEEILMNORSTU"))
	fmt.Println(lexInfo.Combinations("AAJQQ"))
	fmt.Println(lexInfo.Combinations("ACEIORT"))
	fmt.Println(lexInfo.Combinations("MMSUUUU"))
	fmt.Println(lexInfo.Combinations("AIJNORT"))
	fmt.Println(lexInfo.Combinations("AEFFGINR"))
	fmt.Println(lexInfo.Combinations("ADEINOPRTTVZ"))
	fmt.Println(lexInfo.Combinations("ABEIPRSTZ"))
}
