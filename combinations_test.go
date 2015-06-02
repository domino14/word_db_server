package main

import "testing"

type combinationstestpair struct {
	alphagram    string
	combinations uint64
}

var combinationsTests = []combinationstestpair{
	{"AADEEEILMNORSTU", 640342278144},
	{"AAJQQ", 153},
	{"ACEIORT", 2323512},
	{"MMSUUUU", 120},
	{"AIJNORT", 817236},
	{"AEFFGINR", 1077300},
	{"ADEINOPRTTVZ", 860575104},
	{"ABEIPRSTZ", 5669136},
}

func TestCalcCombinations(t *testing.T) {
	lexInfo := LexiconInfo{
		lexiconName:        "OWL2",
		lexiconFilename:    "./blah.txt",
		lexiconIndex:       4,
		descriptiveName:    "American 06",
		letterDistribution: EnglishLetterDistribution()}
	lexInfo.Initialize()

	for _, pair := range combinationsTests {
		combinations := lexInfo.Combinations(pair.alphagram)
		if combinations != pair.combinations {
			t.Error("For", pair.alphagram, "expected", pair.combinations,
				"got", combinations)
		}

	}
}
