package dbmaker

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/rs/zerolog/log"

	"github.com/domino14/macondo/alphabet"
	"github.com/domino14/macondo/gaddag"
	"github.com/domino14/macondo/gaddagmaker"
)

func LexiconMappings(lexiconPath string) ([]LexiconSymbolDefinition, LexiconMap) {
	symbols := []LexiconSymbolDefinition{
		{In: "America", NotIn: "OWL2", Symbol: "+"},
		{In: "NWL18", NotIn: "CSW19", Symbol: "$"},
		{In: "NWL18", NotIn: "America", Symbol: "+"},
		{In: "CSW19", NotIn: "NWL18", Symbol: "#"},
		{In: "FISE2", NotIn: "FISE09", Symbol: "+"},
		{In: "CSW19", NotIn: "CSW15", Symbol: "+"},
	}
	// set LEXICON_PATH to something.
	// For example "/Users/cesar/coding/webolith/words/" on my computer.
	lexiconMap := LexiconMap{
		"CSW15": LexiconInfo{
			LexiconName:        "CSW15",
			LexiconFilename:    filepath.Join(lexiconPath, "CSW15.txt"),
			Dawg:               LoadOrMakeDawg(lexiconPath, "CSW15", false),
			RDawg:              LoadOrMakeDawg(lexiconPath, "CSW15", true),
			LexiconIndex:       1,
			DescriptiveName:    "Collins 15",
			LetterDistribution: alphabet.EnglishLetterDistribution(),
		},
		"CSW19": LexiconInfo{
			LexiconName:        "CSW19",
			LexiconFilename:    filepath.Join(lexiconPath, "CSW19.txt"),
			Dawg:               LoadOrMakeDawg(lexiconPath, "CSW19", false),
			RDawg:              LoadOrMakeDawg(lexiconPath, "CSW19", true),
			LexiconIndex:       12,
			DescriptiveName:    "Collins 2019",
			LetterDistribution: alphabet.EnglishLetterDistribution(),
		},
		"OWL2": LexiconInfo{
			LexiconName:        "OWL2",
			LexiconFilename:    filepath.Join(lexiconPath, "OWL2.txt"),
			Dawg:               LoadOrMakeDawg(lexiconPath, "OWL2", false),
			RDawg:              LoadOrMakeDawg(lexiconPath, "OWL2", true),
			LexiconIndex:       4,
			DescriptiveName:    "OWL2",
			LetterDistribution: alphabet.EnglishLetterDistribution(),
		},
		"America": LexiconInfo{
			LexiconName:        "America",
			LexiconFilename:    filepath.Join(lexiconPath, "America.txt"),
			Dawg:               LoadOrMakeDawg(lexiconPath, "America", false),
			RDawg:              LoadOrMakeDawg(lexiconPath, "America", true),
			LexiconIndex:       7,
			DescriptiveName:    "I am America, and so can you.",
			LetterDistribution: alphabet.EnglishLetterDistribution(),
		},
		"FISE09": LexiconInfo{
			LexiconName:        "FISE09",
			LexiconFilename:    filepath.Join(lexiconPath, "FISE09.txt"),
			Dawg:               LoadOrMakeDawg(lexiconPath, "FISE09", false),
			RDawg:              LoadOrMakeDawg(lexiconPath, "FISE09", true),
			LexiconIndex:       8,
			DescriptiveName:    "Federación Internacional de Scrabble en Español",
			LetterDistribution: alphabet.SpanishLetterDistribution(),
		},
		"FISE2": LexiconInfo{
			LexiconName:        "FISE2",
			LexiconFilename:    filepath.Join(lexiconPath, "FISE2.txt"),
			Dawg:               LoadOrMakeDawg(lexiconPath, "FISE2", false),
			RDawg:              LoadOrMakeDawg(lexiconPath, "FISE2", true),
			LexiconIndex:       10,
			DescriptiveName:    "Federación Internacional de Scrabble en Español, 2017 Edition",
			LetterDistribution: alphabet.SpanishLetterDistribution(),
		},
		"NWL18": LexiconInfo{
			LexiconName:        "NWL18",
			LexiconFilename:    filepath.Join(lexiconPath, "NWL18.txt"),
			Dawg:               LoadOrMakeDawg(lexiconPath, "NWL18", false),
			RDawg:              LoadOrMakeDawg(lexiconPath, "NWL18", true),
			LexiconIndex:       9,
			DescriptiveName:    "NASPA Word List, 2018 Edition",
			LetterDistribution: alphabet.EnglishLetterDistribution(),
			Difficulties:       createDifficultyMap(lexiconPath, "NWL18"),
		},
		"OSPS40": LexiconInfo{
			LexiconName:        "OSPS40",
			LexiconFilename:    filepath.Join(lexiconPath, "OSPS40.txt"),
			Dawg:               LoadOrMakeDawg(lexiconPath, "OSPS40", false),
			RDawg:              LoadOrMakeDawg(lexiconPath, "OSPS40", true),
			LexiconIndex:       11,
			DescriptiveName:    "Polska Federacja Scrabble - Update 40",
			LetterDistribution: alphabet.PolishLetterDistribution(),
		},
	}
	return symbols, lexiconMap
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
