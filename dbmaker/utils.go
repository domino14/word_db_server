package dbmaker

import (
	"sort"
	"strings"
)

func MakeAlphagram(word string) string {
	letters := strings.Split(word, "")
	sort.Strings(letters)
	return strings.Join(letters, "")
}
