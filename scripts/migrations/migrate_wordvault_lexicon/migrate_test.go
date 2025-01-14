package main

import (
	"fmt"
	"testing"

	"github.com/domino14/word_db_server/config"
	"github.com/matryer/is"
)

func TestDeletedUniqueAlphagrams(t *testing.T) {
	is := is.New(t)

	cfg := &config.Config{}
	cfg.Load(nil)
	fmt.Println(cfg.DataPath)

	alphas, err := deletedUniqueAlphagrams(cfg, "CSW24")
	is.NoErr(err)

	alphaMap := map[string]bool{}
	for _, alpha := range alphas {
		alphaMap[alpha] = true
	}

	is.Equal(alphaMap, map[string]bool{
		"AAMNNORSTW": true,
		"AEMNNORSTW": true,
		"AAMNNRST":   true,
		"AFL":        true,
		"AFLS":       true,
	})
}

func TestDeletedSharedAlphagrams(t *testing.T) {
	is := is.New(t)

	cfg := &config.Config{}
	cfg.Load(nil)
	fmt.Println(cfg.DataPath)

	alphas, err := deletedSharedAlphagrams(cfg, "CSW24")
	is.NoErr(err)

	alphaMap := map[string]bool{}
	for _, alpha := range alphas {
		alphaMap[alpha] = true
	}

	is.Equal(alphaMap, map[string]bool{
		"AEMNNRST": true,
	})
}

func TestDeletedUniqueAlphagramsOSPS(t *testing.T) {
	t.Skip()
	is := is.New(t)

	cfg := &config.Config{}
	cfg.Load(nil)
	fmt.Println(cfg.DataPath)

	alphas, err := deletedUniqueAlphagrams(cfg, "OSPS50")
	is.NoErr(err)
	fmt.Println("Deleted unique alphagrams", alphas)

	alphas, err = deletedSharedAlphagrams(cfg, "OSPS50")
	is.NoErr(err)
	fmt.Println("Deleted shared alphagrams", alphas)

	is.True(false)
}
