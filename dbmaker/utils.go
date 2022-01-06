package dbmaker

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"

	"github.com/domino14/macondo/alphabet"
	mcconfig "github.com/domino14/macondo/config"
	"github.com/domino14/macondo/gaddag"
	"github.com/domino14/macondo/gaddagmaker"
)

const DeletionToken = "X"

func LexiconMappings(cfg *mcconfig.Config) LexiconMap {
	// set LEXICON_PATH to something.
	// For example "/Users/cesar/coding/webolith/words/" on my computer.

	englishLD, err := alphabet.EnglishLetterDistribution(cfg)
	if err != nil {
		panic(err)
	}
	spanishLD, err := alphabet.SpanishLetterDistribution(cfg)
	if err != nil {
		panic(err)
	}
	polishLD, err := alphabet.PolishLetterDistribution(cfg)
	if err != nil {
		panic(err)
	}
	germanLD, err := alphabet.GermanLetterDistribution(cfg)
	if err != nil {
		panic(err)
	}
	frenchLD, err := alphabet.FrenchLetterDistribution(cfg)
	if err != nil {
		panic(err)
	}
	lexiconPath := cfg.LexiconPath

	cswFamily := []*LexiconInfo{
		{
			LexiconName:        "CSW12",
			LexiconFilename:    filepath.Join(lexiconPath, "CSW12.txt"),
			Dawg:               LoadOrMakeDawg(lexiconPath, "CSW12", false),
			RDawg:              LoadOrMakeDawg(lexiconPath, "CSW12", true),
			LexiconIndex:       6,
			DescriptiveName:    "CSW12",
			LetterDistribution: englishLD,
		},
		{
			LexiconName:        "CSW15",
			LexiconFilename:    filepath.Join(lexiconPath, "CSW15.txt"),
			Dawg:               LoadOrMakeDawg(lexiconPath, "CSW15", false),
			RDawg:              LoadOrMakeDawg(lexiconPath, "CSW15", true),
			LexiconIndex:       1,
			DescriptiveName:    "Collins 15",
			LetterDistribution: englishLD,
		},
		{
			LexiconName:        "CSW19",
			LexiconFilename:    filepath.Join(lexiconPath, "CSW19.txt"),
			Dawg:               LoadOrMakeDawg(lexiconPath, "CSW19", false),
			RDawg:              LoadOrMakeDawg(lexiconPath, "CSW19", true),
			LexiconIndex:       12,
			DescriptiveName:    "Collins 2019",
			LetterDistribution: englishLD,
			Difficulties:       createDifficultyMap(lexiconPath, "CSW19"),
		},
		{
			LexiconName:        "CSW21",
			LexiconFilename:    filepath.Join(lexiconPath, "CSW21.txt"),
			Dawg:               LoadOrMakeDawg(lexiconPath, "CSW21", false),
			RDawg:              LoadOrMakeDawg(lexiconPath, "CSW21", true),
			LexiconIndex:       18,
			DescriptiveName:    "Collins 2021",
			LetterDistribution: englishLD,
			Difficulties:       createDifficultyMap(lexiconPath, "CSW21"),
		},
	}

	fiseFamily := []*LexiconInfo{
		{
			LexiconName:        "FISE09",
			LexiconFilename:    filepath.Join(lexiconPath, "FISE09.txt"),
			Dawg:               LoadOrMakeDawg(lexiconPath, "FISE09", false),
			RDawg:              LoadOrMakeDawg(lexiconPath, "FISE09", true),
			LexiconIndex:       8,
			DescriptiveName:    "Federación Internacional de Scrabble en Español",
			LetterDistribution: spanishLD,
		},
		{
			LexiconName:        "FISE2",
			LexiconFilename:    filepath.Join(lexiconPath, "FISE2.txt"),
			Dawg:               LoadOrMakeDawg(lexiconPath, "FISE2", false),
			RDawg:              LoadOrMakeDawg(lexiconPath, "FISE2", true),
			LexiconIndex:       10,
			DescriptiveName:    "Federación Internacional de Scrabble en Español, 2017 Edition",
			LetterDistribution: spanishLD,
		},
	}

	twlFamily := []*LexiconInfo{
		{
			LexiconName:        "OWL2",
			LexiconFilename:    filepath.Join(lexiconPath, "OWL2.txt"),
			Dawg:               LoadOrMakeDawg(lexiconPath, "OWL2", false),
			RDawg:              LoadOrMakeDawg(lexiconPath, "OWL2", true),
			LexiconIndex:       4,
			DescriptiveName:    "OWL2",
			LetterDistribution: englishLD,
		},
		{
			LexiconName:        "America",
			LexiconFilename:    filepath.Join(lexiconPath, "America.txt"),
			Dawg:               LoadOrMakeDawg(lexiconPath, "America", false),
			RDawg:              LoadOrMakeDawg(lexiconPath, "America", true),
			LexiconIndex:       7,
			DescriptiveName:    "America",
			LetterDistribution: englishLD,
		},
		{
			LexiconName:        "NWL18",
			LexiconFilename:    filepath.Join(lexiconPath, "NWL18.txt"),
			Dawg:               LoadOrMakeDawg(lexiconPath, "NWL18", false),
			RDawg:              LoadOrMakeDawg(lexiconPath, "NWL18", true),
			LexiconIndex:       9,
			DescriptiveName:    "NASPA Word List, 2020 Edition",
			LetterDistribution: englishLD,
			Difficulties:       createDifficultyMap(lexiconPath, "NWL18"),
		},
		{
			LexiconName:        "NWL20",
			LexiconFilename:    filepath.Join(lexiconPath, "NWL20.txt"),
			Dawg:               LoadOrMakeDawg(lexiconPath, "NWL20", false),
			RDawg:              LoadOrMakeDawg(lexiconPath, "NWL20", true),
			LexiconIndex:       15,
			DescriptiveName:    "NASPA Word List, 2020 Edition",
			LetterDistribution: englishLD,
			Difficulties:       createDifficultyMap(lexiconPath, "NWL20"),
		},
	}

	ospsFamily := []*LexiconInfo{
		{
			LexiconName:        "OSPS42",
			LexiconFilename:    filepath.Join(lexiconPath, "OSPS42.txt"),
			Dawg:               LoadOrMakeDawg(lexiconPath, "OSPS42", false),
			RDawg:              LoadOrMakeDawg(lexiconPath, "OSPS42", true),
			LexiconIndex:       14,
			DescriptiveName:    "Polska Federacja Scrabble - Update 42",
			LetterDistribution: polishLD,
		},
		{
			LexiconName:        "OSPS44",
			LexiconFilename:    filepath.Join(lexiconPath, "OSPS44.txt"),
			Dawg:               LoadOrMakeDawg(lexiconPath, "OSPS44", false),
			RDawg:              LoadOrMakeDawg(lexiconPath, "OSPS44", true),
			LexiconIndex:       16,
			DescriptiveName:    "Polska Federacja Scrabble - Update 44",
			LetterDistribution: polishLD,
		},
	}

	deutschFamily := []*LexiconInfo{
		{
			LexiconName:        "Deutsch",
			LexiconFilename:    filepath.Join(lexiconPath, "Deutsch.txt"),
			Dawg:               LoadOrMakeDawg(lexiconPath, "Deutsch", false),
			RDawg:              LoadOrMakeDawg(lexiconPath, "Deutsch", true),
			LexiconIndex:       17,
			DescriptiveName:    "Scrabble®-Turnierliste - based on Duden 28th edition",
			LetterDistribution: germanLD,
		},
	}

	frenchFamily := []*LexiconInfo{
		{
			LexiconName:        "FRA20",
			LexiconFilename:    filepath.Join(lexiconPath, "FRA20.txt"),
			Dawg:               LoadOrMakeDawg(lexiconPath, "FRA20", false),
			RDawg:              LoadOrMakeDawg(lexiconPath, "FRA20", true),
			DescriptiveName:    "French 2020 lexicon",
			LetterDistribution: frenchLD,
		},
	}

	lexiconMap := LexiconMap{
		FamilyCSW:     cswFamily,
		FamilyFISE:    fiseFamily,
		FamilyTWL:     twlFamily,
		FamilyOSPS:    ospsFamily,
		FamilyDeutsch: deutschFamily,
		FamilyFrench:  frenchFamily,
	}

	return lexiconMap
}

/*
   GoLang: os.Rename() give error "invalid cross-device link" for Docker
   container with Volumes.
   MoveFile(source, destination) will work moving file between folders
   https://gist.github.com/var23rav/23ae5d0d4d830aff886c3c970b8f6c6b
*/
func MoveFile(sourcePath, destPath string) error {
	inputFile, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("Couldn't open source file: %s", err)
	}
	outputFile, err := os.Create(destPath)
	if err != nil {
		inputFile.Close()
		return fmt.Errorf("Couldn't open dest file: %s", err)
	}
	defer outputFile.Close()
	_, err = io.Copy(outputFile, inputFile)
	inputFile.Close()
	if err != nil {
		return fmt.Errorf("Writing to output file failed: %s", err)
	}
	// The copy was successful, so now delete the original file
	err = os.Remove(sourcePath)
	if err != nil {
		return fmt.Errorf("Failed removing original file: %s", err)
	}
	return nil
}

func LoadOrMakeDawg(prefix, lexiconName string, reverse bool) *gaddag.SimpleDawg {
	dawgfilename := lexiconName + ".dawg"
	if reverse {
		dawgfilename = lexiconName + "-r.dawg"
	}

	possibleDawg := filepath.Join(prefix, "dawg", dawgfilename)

	d, err := gaddag.LoadDawg(possibleDawg)
	if err == nil {
		return d
	}
	// Otherwise, build it.
	lexiconFilename := filepath.Join(prefix, lexiconName+".txt")
	gd := gaddagmaker.GenerateDawg(lexiconFilename, true, true, reverse)
	if gd.Root == nil {
		// Gaddag could not be generated at all, maybe lexicon is missing.
		log.Error().Err(err).Msg("")
		return nil
	}
	// Otherwise, rename file
	err = MoveFile("out.dawg", possibleDawg)
	if err != nil {
		panic(err)
	}
	// It should exist now.
	d, err = gaddag.LoadDawg(possibleDawg)
	if err != nil {
		panic(err)
	}
	return d
}
