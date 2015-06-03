// Package dbmaker creates SQLITE databases for various lexica, so I can use
// them in my word game empire.
package dbmaker

import (
	"bufio"
	"fmt"
	"github.com/domino14/macondo/gaddag"
	"os"
	"sort"
	"strings"
)

type Alphagram struct {
	words        []string
	combinations uint64
	alphagram    string
}

func (a *Alphagram) String() string {
	return fmt.Sprintf("Alphagram: %s (%d)", a.alphagram, a.combinations)
}

type AlphByCombos []*Alphagram

func (a AlphByCombos) Len() int      { return len(a) }
func (a AlphByCombos) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a AlphByCombos) Less(i, j int) bool {
	// XXX: Should sort by alphagram if combinations are identical
	// This may result in a different DB than on Aerolith :(
	return a[i].combinations > a[j].combinations
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
	definitions, alphagrams := populateAlphsDefs(lexiconInfo.LexiconFilename,
		lexiconInfo.Combinations)
	fmt.Println("Sorting by probability")
	alphs := alphaMapKeys(&alphagrams)
	sort.Sort(AlphByCombos(alphs))
	fmt.Println(alphs)
	if len(definitions) == 0 {
	}
}

// XXX: Find a more idiomatic way of doing this.
func alphaMapKeys(theMap *map[string]*Alphagram) []*Alphagram {
	x := make([]*Alphagram, len(*theMap))
	i := 0
	for _, value := range *theMap {
		x[i] = value
		i++
	}
	return x
}

func populateAlphsDefs(filename string, combinations func(string) uint64) (
	definitions map[string]string, alphagrams map[string]*Alphagram) {
	definitions = make(map[string]string)
	alphagrams = make(map[string]*Alphagram)
	file, _ := os.Open(filename)
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
					combinations(alphagram),
					alphagram}
			} else {
				alph.words = append(alph.words, word)
			}
		}
	}
	file.Close()
	return
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
