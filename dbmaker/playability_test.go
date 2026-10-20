package dbmaker

import (
	"strings"
	"testing"

	"github.com/matryer/is"
)

const testRanks = "len\trank\talphagram\tplayability\twords\n" +
	"7\t1\tAEINRST\t51\tRETAINS,STAINER\n" +
	"7\t2\tAEEINRT\t40\tRETINAE,TRAINEE\n" +
	"2\t1\tIQ\t4480\tQI\n"

func TestReadPlayabilities(t *testing.T) {
	is := is.New(t)
	pm, err := readPlayabilities(strings.NewReader(testRanks))
	is.NoErr(err)
	is.Equal(pm, map[string]int{"AEINRST": 1, "AEEINRT": 2, "IQ": 1})

	_, err = readPlayabilities(strings.NewReader(testRanks + "7\t3\tAEINRST\t1\tRETAINS\n"))
	is.True(err != nil) // duplicate alphagram

	_, err = readPlayabilities(strings.NewReader("alphagram\tplayability\nAEINRST\t51\n"))
	is.True(err != nil) // no rank column

	_, err = readPlayabilities(strings.NewReader("rank\talphagram\n0\tAEINRST\n"))
	is.True(err != nil) // ranks start at 1
}

func TestCheckPlayabilities(t *testing.T) {
	is := is.New(t)
	pm := map[string]int{"AEINRST": 1, "AEEINRT": 2}
	is.NoErr(checkPlayabilities([]string{"AEEINRT", "AEINRST"}, pm))

	err := checkPlayabilities([]string{"AEEINRT", "AEINRST", "ADEINRST"}, pm)
	is.True(err != nil) // ADEINRST has no playability
	is.True(strings.Contains(err.Error(), "ADEINRST"))

	err = checkPlayabilities([]string{"AEINRST"}, pm)
	is.True(err != nil) // the file has an alphagram the lexicon doesn't
}

func TestAlphagramPlayability(t *testing.T) {
	is := is.New(t)
	is.True(!alphagramPlayability("AEINRST", nil).Valid)
	p := alphagramPlayability("AEEINRT", map[string]int{"AEEINRT": 2})
	is.True(p.Valid)
	is.Equal(p.Int32, int32(2))
}
