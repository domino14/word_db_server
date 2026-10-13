package wordvault

import (
	"os"
	"testing"

	wglconfig "github.com/domino14/word-golib/config"
	"github.com/matryer/is"
)

func TestLexiconAlphagram(t *testing.T) {
	is := is.New(t)
	cfg := &wglconfig.Config{DataPath: os.Getenv("WDB_DATA_PATH")}

	// Polish sorts Ń right after N, not after Z like code point order.
	is.Equal(lexiconAlphagram(cfg, "OSPS49", "ACKOORWŃ"), "ACKŃOORW")
	is.Equal(lexiconAlphagram(cfg, "OSPS49", "ackoorwń"), "ACKŃOORW")
	is.Equal(lexiconAlphagram(cfg, "RD29", "ARÄ"), "AÄR")
	is.Equal(lexiconAlphagram(cfg, "NWL23", "RETINAS"), "AEINRST")
	// Unparseable input is left alone.
	is.Equal(lexiconAlphagram(cfg, "NWL23", "AB3"), "AB3")

	is.Equal(lookupAlphagrams(cfg, "OSPS49", []string{"ACKOORWŃ", "AEINRST"}),
		[]string{"ACKOORWŃ", "ACKŃOORW", "AEINRST"})
}
