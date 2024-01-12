package common

import (
	"os"
	"testing"

	"github.com/domino14/word-golib/tilemapping"
	"github.com/domino14/word_db_server/config"
	"github.com/matryer/is"
)

var DefaultConfig = &config.Config{
	DataPath: os.Getenv("WDB_DATA_PATH"),
}

func TestAlphagram(t *testing.T) {
	is := is.New(t)
	cfg := map[string]any{"data-path": DefaultConfig.DataPath}
	englishLD, err := tilemapping.EnglishLetterDistribution(cfg)
	is.NoErr(err)
	w := InitializeWord("?EMONEN", englishLD)
	is.Equal(w.MakeAlphagram(), "EEMNNO?")
}
