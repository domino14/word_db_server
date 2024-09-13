package common

import (
	"os"
	"testing"

	"github.com/domino14/word-golib/config"
	"github.com/domino14/word-golib/tilemapping"
	"github.com/matryer/is"
)

var DefaultConfig = &config.Config{
	DataPath: os.Getenv("WDB_DATA_PATH"),
}

func TestAlphagram(t *testing.T) {
	is := is.New(t)
	englishLD, err := tilemapping.EnglishLetterDistribution(DefaultConfig)
	is.NoErr(err)
	w := InitializeWord("?EMONEN", englishLD)
	is.Equal(w.MakeAlphagram(), "EEMNNO?")
}
