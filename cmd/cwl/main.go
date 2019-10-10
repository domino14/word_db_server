package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	"github.com/domino14/word_db_server/internal/anagramserver"
	pb "github.com/domino14/word_db_server/rpc/wordsearcher"
)

const (
	BritishDict  = "CSW19"
	AmericanDict = "NWL18"
)

// Use more specific env var names here to avoid colliding with other
// env vars user might have on their system. (more so the case for log level)
var LogLevel = os.Getenv("CWL_LOG_LEVEL")
var LexiconPath = os.Getenv("CWL_LEXICON_PATH")

type outputWords []*pb.Word

func (ws outputWords) Len() int      { return len(ws) }
func (ws outputWords) Swap(i, j int) { ws[i], ws[j] = ws[j], ws[i] }

type ByLonger struct{ outputWords }

func (ws ByLonger) Less(i, j int) bool {
	if len(ws.outputWords[i].Word) != len(ws.outputWords[j].Word) {
		return len(ws.outputWords[i].Word) < len(ws.outputWords[j].Word)
	}
	return ws.outputWords[i].Word < ws.outputWords[j].Word
}

func main() {
	var build = flag.Bool("b", false, "Build mode")
	var stats = flag.Bool("t", false, "Show stats")
	flag.Parse()

	if flag.NArg() != 1 {
		fmt.Println("Usage: ./cwl [-b] [-t] letters")
		fmt.Println("use -b for build mode; default is exact anagram")
		fmt.Println("-t shows stats (number of words in each lexicon")
		os.Exit(-1)
	}
	zerolog.SetGlobalLevel(zerolog.WarnLevel)
	if strings.ToLower(LogLevel) == "debug" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	anagramMode := pb.AnagramRequest_EXACT
	if *build {
		anagramMode = pb.AnagramRequest_BUILD
	}
	rack := flag.Arg(0)

	s := &anagramserver.Server{
		LexiconPath: LexiconPath,
	}

	s.Initialize() // load dawgs

	amResp, err := s.Anagram(context.Background(), &pb.AnagramRequest{
		Lexicon: AmericanDict,
		Letters: rack,
		Mode:    anagramMode,
		Expand:  true,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}
	britResp, err := s.Anagram(context.Background(), &pb.AnagramRequest{
		Lexicon: BritishDict,
		Letters: rack,
		Mode:    anagramMode,
		Expand:  true,
	})

	outputWords, amOnly, britOnly := merge(amResp, britResp)
	sort.Sort(ByLonger{outputWords})
	printWords(outputWords)
	if *stats {
		fmt.Printf("\u001b[32mTotal: %v -- In %v: %v -- In %v: %v -- British-only: %v -- American-only: %v\033[0m",
			len(outputWords), AmericanDict, len(outputWords)-britOnly, BritishDict,
			len(outputWords)-amOnly, britOnly, amOnly)
	}
}

func merge(american *pb.AnagramResponse, british *pb.AnagramResponse) (outputWords, int, int) {
	// build a set for both
	amerWordMap := mapifyWords(american)
	britWordMap := mapifyWords(british)
	words := []*pb.Word{}
	amerOnly := 0
	britOnly := 0
	addedWords := map[string]bool{}
	for wordStr, word := range amerWordMap {

		if strings.Contains(word.LexiconSymbols, "$") {
			amerOnly++
		}
		addedWords[wordStr] = true
		words = append(words, word)
	}

	for wordStr, word := range britWordMap {
		if strings.Contains(word.LexiconSymbols, "#") {
			britOnly++
		}
		if _, ok := addedWords[wordStr]; !ok {
			addedWords[wordStr] = true
			words = append(words, word)
		}
	}
	return words, amerOnly, britOnly
}

// turn the response into a map of string word to pb word
func mapifyWords(resp *pb.AnagramResponse) map[string]*pb.Word {
	m := map[string]*pb.Word{}
	for _, word := range resp.Words {
		m[word.Word] = word
	}
	return m
}

func printWords(words []*pb.Word) {
	var Reset = "\033[0m"
	var Red = "\u001b[31m"
	var Blue = "\u001b[34m"

	for idx, word := range words {
		color := ""
		reset := ""
		if strings.Contains(word.LexiconSymbols, "#") {
			color = Red
			reset = Reset
		} else if strings.Contains(word.LexiconSymbols, "$") {
			color = Blue
			reset = Reset
		}
		def := strings.Replace(word.Definition, "\n", " / ", -1)
		fmt.Printf("%v%v%v: %v%v", color, word.Word, word.LexiconSymbols,
			def, reset)
		if idx != len(words)-1 {
			fmt.Printf("\n")
		}
	}
}
