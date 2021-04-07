package dawg

import (
	"strings"

	"github.com/domino14/macondo/alphabet"
	mcconfig "github.com/domino14/macondo/config"
	"github.com/domino14/macondo/gaddag"
)

type dawgInfo struct {
	dawg *gaddag.SimpleDawg
	dist *alphabet.LetterDistribution
}

func (di *dawgInfo) GetDawg() *gaddag.SimpleDawg {
	return di.dawg
}

func (di *dawgInfo) GetDist() *alphabet.LetterDistribution {
	return di.dist
}

// GetDawgInfo gets a dawg and letter distribution. The letter distribution
// is deduced from the lexicon name. A better version of this function in the
// future might have a letter distribution name as an argument.
func GetDawgInfo(cfg *mcconfig.Config, lexicon string) (*dawgInfo, error) {
	var distName string
	switch {
	case strings.Contains(lexicon, "FISE"):
		distName = "spanish"
	case strings.Contains(lexicon, "OSPS"):
		distName = "polish"
	case strings.Contains(lexicon, "Deutsch"):
		distName = "german"
	default:
		distName = "english"
	}

	dist, err := alphabet.Get(cfg, distName)
	if err != nil {
		return nil, err
	}

	dawg, err := gaddag.GetDawg(cfg, lexicon)
	if err != nil {
		return nil, err
	}

	return &dawgInfo{
		dawg: dawg,
		dist: dist,
	}, nil
}
