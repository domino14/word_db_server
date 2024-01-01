package dawg

import (
	"strings"
	"sync"

	mcconfig "github.com/domino14/macondo/config"
	"github.com/domino14/macondo/gaddag"
	"github.com/domino14/macondo/tilemapping"
)

var DaPool = sync.Pool{
	New: func() interface{} {
		return &gaddag.DawgAnagrammer{}
	},
}

type dawgInfo struct {
	dawg *gaddag.SimpleDawg
	dist *tilemapping.LetterDistribution
}

func (di *dawgInfo) GetDawg() *gaddag.SimpleDawg {
	return di.dawg
}

func (di *dawgInfo) GetDist() *tilemapping.LetterDistribution {
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

	dist, err := tilemapping.GetDistribution(cfg, distName)
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
