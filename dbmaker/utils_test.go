package dbmaker

import "testing"

type alphagramtestpair struct {
	word      string
	alphagram string
}

var utilsTests = []alphagramtestpair{
	{"FIREFANG", "AEFFGINR"},
	{"QAJAQ", "AAJQQ"},
	{"EROTICA", "ACEIORT"},
	{"MUUMUUS", "MMSUUUU"},
	{"PRIVATDOZENT", "ADEINOPRTTVZ"},
	{"DEUTERANOMALIES", "AADEEEILMNORSTU"},
}

func TestAlphagram(t *testing.T) {
	for _, pair := range utilsTests {
		alphagram := MakeAlphagram(pair.word)
		if alphagram != pair.alphagram {
			t.Error("For", pair.word, "expected", pair.alphagram,
				"got", alphagram)
		}

	}
}
