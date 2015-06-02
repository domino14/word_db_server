// Package dbmaker creates SQLITE databases for various lexica, so I can use
// them in my word game empire.
package dbmaker

import (
	"bufio"
	"fmt"
	"github.com/domino14/macondo/gaddag"
	"os"
	"strings"
)

type Alphagram struct {
	words        []string
	combinations uint64
	alphagram    string
}
type LexiconMap map[string]*LexiconInfo

type LexiconSymbolDefinition struct {
	In     string // The word is in this lexicon
	NotIn  string // The word is not in this lexicon
	Symbol string // The corresponding lexicon symbol
}

func CreateLexiconDatabase(lexiconName string, lexiconInfo *LexiconInfo,
	lexSymbols []LexiconSymbolDefinition, lexMap LexiconMap) {
	fmt.Println("Creating lexicon database", lexiconName, lexiconInfo,
		lexSymbols, lexMap)

	gaddag.GenerateGaddag(lexiconInfo.LexiconFilename, false, false)
	definitions := make(map[string]string)
	alphagrams := make(map[string]*Alphagram)
	file, _ := os.Open(lexiconInfo.LexiconFilename)
	// XXX: Check error
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) > 0 {
			word := fields[0]
			definition := ""
			if len(fields) > 1 {
				definition = strings.Join(fields[1:], " ")
			}
			definitions[word] = definition
			alphagram := MakeAlphagram(word)

			alph, ok := alphagrams[alphagram]
			if !ok {
				alphagrams[alphagram] = &Alphagram{
					[]string{word},
					lexiconInfo.Combinations(alphagram),
					alphagram}
			} else {
				alph.words = append(alph.words, word)
			}
		}
	}
	file.Close()
	fmt.Println("Sorting by probability")
}

/**
*  words := []string{}
   file, err := os.Open(filename)
   if err != nil {
       log.Println("Filename", filename, "not found")
       return nil
   }
   scanner := bufio.NewScanner(file)
   for scanner.Scan() {
       // Split line into spaces.
       fields := strings.Fields(scanner.Text())
       if len(fields) > 0 {
           words = append(words, fields[0])
       }
   }
   file.Close()
   return words
*/
