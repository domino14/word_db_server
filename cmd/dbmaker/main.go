// The caller of the db creator.
package main

import (
	"os"
	"strings"

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
	MigrateDB    string
	DBs          string
	ForceCreate  bool
	FixDefsOn    string
	FixSymbolsOn string
	OutputDir    string
	DataPath     string
}

// Load loads the configs from the given arguments
func (c *Config) Load(args []string) error {
	fs := flag.NewFlagSet("dbmaker", flag.ContinueOnError)
	// We are going to have a flag to migrate a database. This is due to a
	// legacy issue where alphagram sort order was not deterministic for
	// alphagrams with equal probability, so we need to keep the old
	// sort orders around in order to not mess up alphagrams-by-probability
	// lists.

	fs.StringVar(&c.MigrateDB, "migratedb", "", "Migrate a DB instead of generating it")
	fs.StringVar(&c.DBs, "dbs", "", "Pass in comma-separated list of dbs to make, instead of all")
	fs.BoolVar(&c.ForceCreate, "force", false, "Create DB even if it already exists (overwrite)")
	fs.StringVar(&c.FixDefsOn, "fixdefs", "",
		"Pass in lexicon name to fix definitions on. DB <lexiconname>.db must exist in this dir.")
	fs.StringVar(&c.FixSymbolsOn, "fixsymbols", "",
		"Pass in lexicon name to fix lexicon symbols on. DB <lexiconname>.db must exist in this dir.")
	fs.StringVar(&c.OutputDir, "outputdir", ".", "The output directory")
	fs.StringVar(&c.DataPath, "datapath", os.Getenv("WDB_DATA_PATH"), "The data path")
	return fs.Parse(args)

}

func main() {

	cfg := &Config{}
	cfg.Load(os.Args[1:])
	log.Info().Interface("config", cfg).Msg("dbmaker-started")

	// MkdirAll will make any intermediate dirs but fail gracefully if they exist.
	os.MkdirAll(cfg.OutputDir, os.ModePerm)
	lexiconMap := dbmaker.LexiconMappings(cfg.DataPath)

	if cfg.MigrateDB != "" {
		info, err := lexiconMap.GetLexiconInfo(cfg.MigrateDB)
		if err != nil {
			log.Err(err).Msg("That lexicon is not supported")
			return
		}
		dbmaker.MigrateLexiconDatabase(cfg.MigrateDB, info)
	} else if cfg.FixDefsOn != "" {
		fixDefinitions(cfg.FixDefsOn, lexiconMap)
	} else if cfg.FixSymbolsOn != "" {
		fixSymbols(cfg.FixSymbolsOn, lexiconMap)
	} else {
		makeDbs(cfg.DBs, lexiconMap, cfg.OutputDir, cfg.ForceCreate)
	}
}

func fixDefinitions(dbToFixDefs string, lexiconMap dbmaker.LexiconMap) {
	// open existing databases but new dictionary files/dawgs etc
	// and apply new definitions
	dbmaker.FixDefinitions(dbToFixDefs, lexiconMap)
}

func fixSymbols(dbToFixSymbols string, lexiconMap dbmaker.LexiconMap) {

	// open existing databases but new dictionary files/dawgs etc
	// and apply lex symbols.
	dbmaker.FixLexiconSymbols(dbToFixSymbols, lexiconMap)
}

func makeDbs(dbsToMake string, lexiconMap dbmaker.LexiconMap,
	outputDir string, forceCreation bool) {

	dbs := []string{}
	if dbsToMake != "" {
		dbs = strings.Split(dbsToMake, ",")
	} else {
		panic("must provide a list of dbs to make")
	}

	for _, db := range dbs {
		info, err := lexiconMap.GetLexiconInfo(db)
		if err != nil {
			log.Err(err).Msgf("%v was not in list of dbs, skipping...", db)
			continue
		}
		if info.KWG == nil || info.KWG.GetAlphabet() == nil {
			log.Info().Msgf("%v was not supplied, skipping...", db)
			continue
		}
		info.Initialize()
		dbmaker.CreateLexiconDatabase(db, info, lexiconMap,
			outputDir, !forceCreation)
	}

}
