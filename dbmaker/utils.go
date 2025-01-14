package dbmaker

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/domino14/word-golib/config"
	"github.com/domino14/word-golib/kwg"
	"github.com/domino14/word-golib/tilemapping"
	"github.com/rs/zerolog/log"
)

const DeletionToken = "X"

func loadKWG(dataPath, lexName string) *kwg.KWG {
	k, err := kwg.Get(&config.Config{DataPath: dataPath}, lexName)
	if err != nil {
		log.Err(err).Str("lexName", lexName).Msg("unable to load kwg")
	}
	return k
}

func LexiconMappings(dataPath string) LexiconMap {
	cfg := &config.Config{DataPath: dataPath}

	englishLD, err := tilemapping.EnglishLetterDistribution(cfg)
	if err != nil {
		panic(err)
	}
	spanishLD, err := tilemapping.NamedLetterDistribution(cfg, "spanish")
	if err != nil {
		panic(err)
	}
	polishLD, err := tilemapping.NamedLetterDistribution(cfg, "polish")
	if err != nil {
		panic(err)
	}
	germanLD, err := tilemapping.NamedLetterDistribution(cfg, "german")
	if err != nil {
		panic(err)
	}
	frenchLD, err := tilemapping.NamedLetterDistribution(cfg, "french")
	if err != nil {
		panic(err)
	}

	lexiconPath := filepath.Join(dataPath, "lexica")

	cswFamily := []*LexiconInfo{
		{
			LexiconName:        "CSW12",
			LexiconFilename:    filepath.Join(lexiconPath, "CSW12.txt"),
			LexiconIndex:       6,
			KWG:                loadKWG(dataPath, "CSW12"),
			DescriptiveName:    "CSW12",
			LetterDistribution: englishLD,
		},
		{
			LexiconName:        "CSW15",
			LexiconFilename:    filepath.Join(lexiconPath, "CSW15.txt"),
			KWG:                loadKWG(dataPath, "CSW15"),
			LexiconIndex:       1,
			DescriptiveName:    "Collins 15",
			LetterDistribution: englishLD,
		},
		{
			LexiconName:        "CSW19",
			LexiconFilename:    filepath.Join(lexiconPath, "CSW19.txt"),
			KWG:                loadKWG(dataPath, "CSW19"),
			LexiconIndex:       12,
			DescriptiveName:    "Collins 2019",
			LetterDistribution: englishLD,
			Difficulties:       createDifficultyMap(lexiconPath, "CSW19"),
		},
		{
			LexiconName:        "CSW21",
			LexiconFilename:    filepath.Join(lexiconPath, "CSW21.txt"),
			KWG:                loadKWG(dataPath, "CSW21"),
			LexiconIndex:       18,
			DescriptiveName:    "Collins 2021",
			LetterDistribution: englishLD,
			Difficulties:       createDifficultyMap(lexiconPath, "CSW21"),
		},
		{
			LexiconName:        "CSW24",
			LexiconFilename:    filepath.Join(lexiconPath, "CSW24.txt"),
			KWG:                loadKWG(dataPath, "CSW24"),
			LexiconIndex:       25,
			DescriptiveName:    "Collins 2024",
			LetterDistribution: englishLD,
			Difficulties:       createDifficultyMap(lexiconPath, "CSW24"),
		},
	}

	fiseFamily := []*LexiconInfo{
		{
			LexiconName:        "FISE09",
			LexiconFilename:    filepath.Join(lexiconPath, "FISE09.txt"),
			KWG:                loadKWG(dataPath, "FISE09"),
			LexiconIndex:       8,
			DescriptiveName:    "Federación Internacional de Scrabble en Español",
			LetterDistribution: spanishLD,
		},
		{
			LexiconName:        "FISE2",
			LexiconFilename:    filepath.Join(lexiconPath, "FISE2.txt"),
			KWG:                loadKWG(dataPath, "FISE2"),
			LexiconIndex:       10,
			DescriptiveName:    "Federación Internacional de Scrabble en Español, 2017 Edition",
			LetterDistribution: spanishLD,
		},
	}

	twlFamily := []*LexiconInfo{
		{
			LexiconName:        "OWL2",
			LexiconFilename:    filepath.Join(lexiconPath, "OWL2.txt"),
			KWG:                loadKWG(dataPath, "OWL2"),
			LexiconIndex:       4,
			DescriptiveName:    "OWL2",
			LetterDistribution: englishLD,
		},
		{
			LexiconName:        "America",
			LexiconFilename:    filepath.Join(lexiconPath, "America.txt"),
			KWG:                loadKWG(dataPath, "America"),
			LexiconIndex:       7,
			DescriptiveName:    "America",
			LetterDistribution: englishLD,
		},
		{
			LexiconName:        "NWL18",
			LexiconFilename:    filepath.Join(lexiconPath, "NWL18.txt"),
			KWG:                loadKWG(dataPath, "NWL18"),
			LexiconIndex:       9,
			DescriptiveName:    "NASPA Word List, 2020 Edition",
			LetterDistribution: englishLD,
			Difficulties:       createDifficultyMap(lexiconPath, "NWL18"),
		},
		{
			LexiconName:        "NWL20",
			LexiconFilename:    filepath.Join(lexiconPath, "NWL20.txt"),
			KWG:                loadKWG(dataPath, "NWL20"),
			LexiconIndex:       15,
			DescriptiveName:    "NASPA Word List, 2020 Edition",
			LetterDistribution: englishLD,
			Difficulties:       createDifficultyMap(lexiconPath, "NWL20"),
		},
		{
			LexiconName:        "NWL23",
			LexiconFilename:    filepath.Join(lexiconPath, "NWL23.txt"),
			KWG:                loadKWG(dataPath, "NWL23"),
			LexiconIndex:       24,
			DescriptiveName:    "NASPA Word List, 2023 Edition",
			LetterDistribution: englishLD,
			Difficulties:       createDifficultyMap(lexiconPath, "NWL23"),
		},
	}

	ospsFamily := []*LexiconInfo{
		{
			LexiconName:        "OSPS42",
			LexiconFilename:    filepath.Join(lexiconPath, "OSPS42.txt"),
			KWG:                loadKWG(dataPath, "OSPS42"),
			LexiconIndex:       14,
			DescriptiveName:    "Polska Federacja Scrabble - Update 42",
			LetterDistribution: polishLD,
		},
		{
			LexiconName:        "OSPS44",
			LexiconFilename:    filepath.Join(lexiconPath, "OSPS44.txt"),
			KWG:                loadKWG(dataPath, "OSPS44"),
			LexiconIndex:       16,
			DescriptiveName:    "Polska Federacja Scrabble - Update 44",
			LetterDistribution: polishLD,
		},
		{
			LexiconName:        "OSPS46",
			LexiconFilename:    filepath.Join(lexiconPath, "OSPS46.txt"),
			KWG:                loadKWG(dataPath, "OSPS46"),
			LexiconIndex:       20,
			DescriptiveName:    "Polska Federacja Scrabble - Update 46",
			LetterDistribution: polishLD,
		},
		{
			LexiconName:        "OSPS48",
			LexiconFilename:    filepath.Join(lexiconPath, "OSPS48.txt"),
			KWG:                loadKWG(dataPath, "OSPS48"),
			LexiconIndex:       21,
			DescriptiveName:    "Polska Federacja Scrabble - Update 48",
			LetterDistribution: polishLD,
		},
		{
			LexiconName:        "OSPS49",
			LexiconFilename:    filepath.Join(lexiconPath, "OSPS49.txt"),
			KWG:                loadKWG(dataPath, "OSPS49"),
			LexiconIndex:       22,
			DescriptiveName:    "Polska Federacja Scrabble - Update 49",
			LetterDistribution: polishLD,
		},
		{
			LexiconName:        "OSPS50",
			LexiconFilename:    filepath.Join(lexiconPath, "OSPS50.txt"),
			KWG:                loadKWG(dataPath, "OSPS50"),
			LexiconIndex:       26,
			DescriptiveName:    "Polska Federacja Scrabble - Update 50",
			LetterDistribution: polishLD,
		},
	}

	deutschFamily := []*LexiconInfo{
		{
			LexiconName:        "Deutsch",
			LexiconFilename:    filepath.Join(lexiconPath, "Deutsch.txt"),
			KWG:                loadKWG(dataPath, "RD28"),
			LexiconIndex:       17,
			DescriptiveName:    "Scrabble®-Turnierliste - based on Duden 28th edition",
			LetterDistribution: germanLD,
		},
	}

	frenchFamily := []*LexiconInfo{
		{
			LexiconName:        "FRA20",
			LexiconFilename:    filepath.Join(lexiconPath, "FRA20.txt"),
			KWG:                loadKWG(dataPath, "FRA20"),
			DescriptiveName:    "French 2020 lexicon",
			LetterDistribution: frenchLD,
		},
		{
			LexiconName:        "FRA24",
			LexiconFilename:    filepath.Join(lexiconPath, "FRA24.txt"),
			KWG:                loadKWG(dataPath, "FRA24"),
			DescriptiveName:    "French 2024 lexicon",
			LetterDistribution: frenchLD,
			LexiconIndex:       23,
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
