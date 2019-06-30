package dbmaker

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/domino14/macondo/alphabet"
	"github.com/domino14/macondo/gaddag"
	"github.com/domino14/macondo/gaddagmaker"
)

var LexiconPath = os.Getenv("LEXICON_PATH")

func LexiconMappings() ([]LexiconSymbolDefinition, LexiconMap) {
	symbols := []LexiconSymbolDefinition{
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
			LexiconFilename:    filepath.Join(LexiconPath, "CSW15.txt"),
			Gaddag:             LoadOrMakeGaddag(LexiconPath, "CSW15"),
			LexiconIndex:       1,
			DescriptiveName:    "Collins 15",
			LetterDistribution: alphabet.EnglishLetterDistribution(),
		},
		"CSW19": LexiconInfo{
			LexiconName:        "CSW19",
			LexiconFilename:    filepath.Join(LexiconPath, "CSW19.txt"),
			Gaddag:             LoadOrMakeGaddag(LexiconPath, "CSW19"),
			LexiconIndex:       12,
			DescriptiveName:    "Collins 2019",
			LetterDistribution: alphabet.EnglishLetterDistribution(),
		},
		"America": LexiconInfo{
			LexiconName:        "America",
			LexiconFilename:    filepath.Join(LexiconPath, "America.txt"),
			Gaddag:             LoadOrMakeGaddag(LexiconPath, "America"),
			LexiconIndex:       7,
			DescriptiveName:    "I am America, and so can you.",
			LetterDistribution: alphabet.EnglishLetterDistribution(),
		},
		"FISE09": LexiconInfo{
			LexiconName:        "FISE09",
			LexiconFilename:    filepath.Join(LexiconPath, "FISE09.txt"),
			Gaddag:             LoadOrMakeGaddag(LexiconPath, "FISE09"),
			LexiconIndex:       8,
			DescriptiveName:    "Federación Internacional de Scrabble en Español",
			LetterDistribution: alphabet.SpanishLetterDistribution(),
		},
		"FISE2": LexiconInfo{
			LexiconName:        "FISE2",
			LexiconFilename:    filepath.Join(LexiconPath, "FISE2.txt"),
			Gaddag:             LoadOrMakeGaddag(LexiconPath, "FISE2"),
			LexiconIndex:       10,
			DescriptiveName:    "Federación Internacional de Scrabble en Español, 2017 Edition",
			LetterDistribution: alphabet.SpanishLetterDistribution(),
		},
		"NWL18": LexiconInfo{
			LexiconName:        "NWL18",
			LexiconFilename:    filepath.Join(LexiconPath, "NWL18.txt"),
			Gaddag:             LoadOrMakeGaddag(LexiconPath, "NWL18"),
			LexiconIndex:       9,
			DescriptiveName:    "NASPA Word List, 2018 Edition",
			LetterDistribution: alphabet.EnglishLetterDistribution(),
		},
		"OSPS40": LexiconInfo{
			LexiconName:        "OSPS40",
			LexiconFilename:    filepath.Join(LexiconPath, "OSPS40.txt"),
			Gaddag:             LoadOrMakeGaddag(LexiconPath, "OSPS40"),
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

func LoadOrMakeGaddag(prefix, lexiconName string) *gaddag.SimpleGaddag {
	possibleGaddag := filepath.Join(prefix, "gaddag", lexiconName+".gaddag")
	sg := gaddag.LoadGaddag(possibleGaddag)
	if sg != nil {
		return sg
	}
	// Otherwise, build it.
	lexiconFilename := filepath.Join(prefix, lexiconName+".txt")
	gd := gaddagmaker.GenerateGaddag(lexiconFilename, false, true)
	if gd.Root == nil {
		// Gaddag could not be generated at all, maybe lexicon is missing.
		return nil
	}
	// Otherwise, rename file
	err := MoveFile("out.gaddag", possibleGaddag)
	if err != nil {
		panic(err)
	}
	// It should exist now.
	return gaddag.LoadGaddag(possibleGaddag)
}

func LoadOrMakeDawg(prefix, lexiconName string) *gaddag.SimpleGaddag {
	possibleDawg := filepath.Join(prefix, "dawg", lexiconName+".dawg")
	sg := gaddag.LoadGaddag(possibleDawg)
	if sg != nil {
		return sg
	}
	// Otherwise, build it.
	lexiconFilename := filepath.Join(prefix, lexiconName+".txt")
	gd := gaddagmaker.GenerateDawg(lexiconFilename, true, true)
	if gd.Root == nil {
		// Gaddag could not be generated at all, maybe lexicon is missing.
		return nil
	}
	// Otherwise, rename file
	err := MoveFile("out.gaddag", possibleDawg)
	if err != nil {
		panic(err)
	}
	// It should exist now.
	return gaddag.LoadGaddag(possibleDawg)
}
