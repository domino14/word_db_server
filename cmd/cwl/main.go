package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/namsral/flag"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"

	mcconfig "github.com/domino14/macondo/config"

	"github.com/domino14/word_db_server/internal/anagramserver"
	pb "github.com/domino14/word_db_server/rpc/wordsearcher"
)

const (
	BritishDict  = "CSW21"
	AmericanDict = "NWL20"
)

// Use more specific env var names here to avoid colliding with other
// env vars user might have on their system. (more so the case for log level)
var LogLevel = os.Getenv("CWL_LOG_LEVEL")

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

type Config struct {
	MacondoConfig mcconfig.Config

	buildMode bool
	showStats bool
	rack      string
}

func (c *Config) Load(args []string) error {
	fs := flag.NewFlagSet("cwl", flag.ContinueOnError)

	fs.BoolVar(&c.MacondoConfig.Debug, "debug", false, "debug logging on")

	fs.StringVar(&c.MacondoConfig.LetterDistributionPath, "letter-distribution-path", "../macondo/data/letterdistributions", "directory holding letter distribution files")
	fs.StringVar(&c.MacondoConfig.StrategyParamsPath, "strategy-params-path", "../macondo/data/strategy", "directory holding strategy files")
	fs.StringVar(&c.MacondoConfig.LexiconPath, "lexicon-path", "../macondo/data/lexica", "directory holding lexicon files")
	fs.StringVar(&c.MacondoConfig.DefaultLexicon, "default-lexicon", "NWL18", "the default lexicon to use")
	fs.StringVar(&c.MacondoConfig.DefaultLetterDistribution, "default-letter-distribution", "English", "the default letter distribution to use. English, EnglishSuper, Spanish, Polish, etc.")

	// We are going to have a flag to migrate a database. This is due to a
	// legacy issue where alphagram sort order was not deterministic for
	// alphagrams with equal probability, so we need to keep the old
	// sort orders around in order to not mess up alphagrams-by-probability
	// lists.

	fs.BoolVar(&c.buildMode, "b", false, "Build mode")
	fs.BoolVar(&c.showStats, "t", false, "Show stats")

	err := fs.Parse(args)
	if err != nil {
		return err
	}
	c.rack = fs.Arg(0)
	return nil
}

func main() {
	cfg := &Config{}
	cfg.Load(os.Args[1:])

	zerolog.SetGlobalLevel(zerolog.WarnLevel)
	if strings.ToLower(LogLevel) == "debug" {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	}

	anagramMode := pb.AnagramRequest_EXACT
	if cfg.buildMode {
		anagramMode = pb.AnagramRequest_BUILD
	}
	log.Debug().Interface("config", cfg).Str("rack", cfg.rack).Bool("build", cfg.buildMode).Msg("input")

	s := &anagramserver.Server{
		MacondoConfig: &cfg.MacondoConfig,
	}

	amResp, err := s.Anagram(context.Background(), &pb.AnagramRequest{
		Lexicon: AmericanDict,
		Letters: cfg.rack,
		Mode:    anagramMode,
		Expand:  true,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}
	britResp, err := s.Anagram(context.Background(), &pb.AnagramRequest{
		Lexicon: BritishDict,
		Letters: cfg.rack,
		Mode:    anagramMode,
		Expand:  true,
	})
	if err != nil {
		log.Fatal().Err(err).Msg("")
	}

	outputWords, amOnly, britOnly := merge(amResp, britResp)
	sort.Sort(ByLonger{outputWords})
	printWords(outputWords)
	if cfg.showStats {
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
