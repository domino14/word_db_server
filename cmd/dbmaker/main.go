// The caller of the db creator.
package main

import (
	"os"
	"path/filepath"
	"strings"

	mcconfig "github.com/domino14/macondo/config"
	"github.com/namsral/flag"
	"github.com/rs/zerolog/log"

	"github.com/domino14/word_db_server/dbmaker"
)

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

type Config struct {
	MacondoConfig mcconfig.Config

	migrateDB    string
	dbs          string
	forceCreate  bool
	fixDefsOn    string
	fixSymbolsOn string
	outputDir    string
}

// Load loads the configs from the given arguments
func (c *Config) Load(args []string) error {
	fs := flag.NewFlagSet("dbmaker", flag.ContinueOnError)

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

	fs.StringVar(&c.migrateDB, "migratedb", "", "Migrate a DB instead of generating it")
	fs.StringVar(&c.dbs, "dbs", "", "Pass in comma-separated list of dbs to make, instead of all")
	fs.BoolVar(&c.forceCreate, "force", false, "Create DB even if it already exists (overwrite)")
	fs.StringVar(&c.fixDefsOn, "fixdefs", "",
		"Pass in lexicon name to fix definitions on. DB <lexiconname>.db must exist in this dir.")
	fs.StringVar(&c.fixSymbolsOn, "fixsymbols", "",
		"Pass in lexicon name to fix lexicon symbols on. DB <lexiconname>.db must exist in this dir.")
	fs.StringVar(&c.outputDir, "outputdir", ".", "The output directory")

	return fs.Parse(args)

}

func main() {

	cfg := &Config{}
	cfg.Load(os.Args[1:])
	log.Info().Interface("config", cfg).Msg("dbmaker-started")

	// MkdirAll will make any intermediate dirs but fail gracefully if they exist.
	os.MkdirAll(filepath.Join(cfg.MacondoConfig.LexiconPath, "dawg"), os.ModePerm)
	os.MkdirAll(cfg.outputDir, os.ModePerm)
	symbols, lexiconMap := dbmaker.LexiconMappings(&cfg.MacondoConfig)

	if cfg.migrateDB != "" {
		info, ok := lexiconMap[cfg.migrateDB]
		if !ok {
			log.Error().Msg("That lexicon is not supported")
			return
		}
		dbmaker.MigrateLexiconDatabase(cfg.migrateDB, info)
	} else if cfg.fixDefsOn != "" {
		fixDefinitions(cfg.fixDefsOn, lexiconMap)
	} else if cfg.fixSymbolsOn != "" {
		fixSymbols(cfg.fixSymbolsOn, lexiconMap, symbols)
	} else {
		makeDbs(cfg.dbs, lexiconMap, symbols, cfg.outputDir, cfg.forceCreate)
	}
}

func fixDefinitions(dbToFixDefs string, lexiconMap dbmaker.LexiconMap) {
	// open existing databases but new dictionary files/dawgs etc
	// and apply new definitions
	dbmaker.FixDefinitions(dbToFixDefs, lexiconMap)
}

func fixSymbols(dbToFixSymbols string, lexiconMap dbmaker.LexiconMap,
	symbols []dbmaker.LexiconSymbolDefinition) {

	// open existing databases but new dictionary files/dawgs etc
	// and apply lex symbols.
	dbmaker.FixLexiconSymbols(dbToFixSymbols, lexiconMap, symbols)
}

func makeDbs(dbsToMake string, lexiconMap dbmaker.LexiconMap,
	symbols []dbmaker.LexiconSymbolDefinition, outputDir string,
	forceCreation bool) {

	dbs := []string{}
	if dbsToMake != "" {
		dbs = strings.Split(dbsToMake, ",")
	} else {
		for name := range lexiconMap {
			dbs = append(dbs, name)
		}
	}
	for name, info := range lexiconMap {
		if !stringInSlice(name, dbs) {
			log.Info().Msgf("%v was not in list of dbs, skipping...", name)
			continue
		}
		if info.Dawg == nil || info.Dawg.GetAlphabet() == nil {
			log.Info().Msgf("%v was not supplied, skipping...", name)
			continue
		}
		info.Initialize()
		dbmaker.CreateLexiconDatabase(name, info, symbols, lexiconMap,
			outputDir, !forceCreation)
	}
}
