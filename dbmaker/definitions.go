package dbmaker

import (
	"html"
	"log"
	"regexp"
	"strings"
)

type SingleDefinition struct {
	word         string
	partOfSpeech string
	// declensions looks something like `ORBED, ORBING, ORBS`
	declensions string
	raw         string
	// nopos is the definition without the part of speech and declensions.
	nopospeech  string
	userVisible string
}

type FullDefinition struct {
	word string
	raw  string

	parts []*SingleDefinition
}

const BadString = "__BAD_STRING___BAD__"

const MaxRecursionCount = 10

var linkRe = regexp.MustCompile("{([[:alpha:]]+)=([[:alpha:]]+)}")
var htmlRe = regexp.MustCompile("{([[:alpha:]]+)}")
var rootRe = regexp.MustCompile("<([[:alpha:]]+)=([[:alpha:]]+)>")
var inflectionRe = regexp.MustCompile(`\[([[:alpha:]]+)(\s*[[:ascii:]]*)\]`)

// Expand all the entities, links, etc from the definitions. Return a map
// with just user-readable string definitions.
func expandDefinitions(definitions map[string]*FullDefinition) map[string]string {
	/*
		From Chew:

		but take note of the use of {} to indicate links to significant words
		in definitions but also HTML entities (the words will have a part of
		speech following an = sign), <> to indicate links to root forms of
		inflections, [] to indicate inflections of root forms, / to separate
		different senses of a word, and : to indicate undefined run-on
		entries.
	*/
	userVisibleDefs := make(map[string]string)
	// Now cycle through all the definitions, and recursively follow
	// links, etc.
	for word, fd := range definitions {
		userVisibleDefs[word] = fd.expand(definitions)
	}
	return userVisibleDefs
}

func addToDefinitions(word string, rawdef string, definitions map[string]*FullDefinition) {
	definitions[word] = &FullDefinition{
		raw:  rawdef,
		word: word,
	}
	definitions[word].populateSubdefs()
}

func (fd *FullDefinition) populateSubdefs() {
	parts := strings.Split(fd.raw, " / ")
	fd.parts = make([]*SingleDefinition, len(parts))
	for idx, part := range parts {
		sd := createSingleDefinition(idx, fd.word, part)
		fd.parts[idx] = sd
	}
}

func createSingleDefinition(idx int, word, part string) *SingleDefinition {
	sd := &SingleDefinition{
		word: word,
		raw:  part,
	}
	// Just get the inflection for now.
	inflections := inflectionRe.FindStringSubmatch(part)
	pospeech := ""
	declensions := ""
	if len(inflections) > 1 {
		pospeech = inflections[1]
	}
	if len(inflections) > 2 {
		declensions = strings.TrimSpace(inflections[2])
	}
	sd.partOfSpeech = pospeech
	sd.declensions = declensions
	sd.nopospeech = strings.TrimSpace(inflectionRe.ReplaceAllString(part, ""))
	return sd
}

func (fd *FullDefinition) expand(definitions map[string]*FullDefinition) string {
	expandedParts := []string{}
	for _, part := range fd.parts {
		expanded := expandRaw(part.raw, part.word, definitions, make(map[string]bool),
			false)
		expandedParts = append(expandedParts, expanded)
	}

	return strings.Join(expandedParts, "\n")
}

func ReplaceAllStringSubmatchFunc(re *regexp.Regexp, str string,
	repl func([]string) string) string {

	result := ""
	lastIndex := 0

	for _, v := range re.FindAllSubmatchIndex([]byte(str), -1) {
		groups := []string{}
		for i := 0; i < len(v); i += 2 {
			groups = append(groups, str[v[i]:v[i+1]])
		}

		result += str[lastIndex:v[0]] + repl(groups)
		lastIndex = v[1]
	}

	return result + str[lastIndex:]
}

func expandRaw(rawdef string, word string, definitions map[string]*FullDefinition,
	visitedWords map[string]bool, matchedLink bool) string {

	if visitedWords[word] {
		// This word has already been seen.
		log.Printf("[DEBUG] Word was seen at least once: %v", word)
		return BadString
	}

	visitedWords[word] = true
	rawdef = ReplaceAllStringSubmatchFunc(htmlRe, rawdef, func(groups []string) string {
		return html.UnescapeString("&" + groups[1] + ";")
	})

	// Find {} link submatches
	submatches := linkRe.FindAllStringSubmatch(rawdef, -1)

	def := ""
	if len(submatches) > 0 {
		matchedLink = true
		substrings := linkRe.Split(rawdef, -1)
		def += substrings[0]
		idx := 0

		for _, submatch := range submatches {
			link := submatch[1]
			pospeech := submatch[2]
			idx++
			def += link
			linkText := findLinkText(link, pospeech, definitions, word, false,
				visitedWords, matchedLink)
			if linkText != BadString && linkText != "" {
				def += " (" + linkText + ")"
			}
		}
		def += substrings[idx]
	} else {
		def = rawdef
	}
	rawdef = def
	def = ""

	// Find < > submatches
	submatches = rootRe.FindAllStringSubmatch(rawdef, -1)
	if len(submatches) > 0 {
		substrings := rootRe.Split(rawdef, -1)
		def += substrings[0]
		idx := 0

		for _, submatch := range submatches {
			root := submatch[1]
			pospeech := submatch[2]
			idx++
			linkText := findLinkText(root, pospeech, definitions, word, true,
				visitedWords, matchedLink)
			if linkText != BadString {
				if matchedLink {
					def += linkText
				} else {
					def += strings.ToUpper(root)
					if linkText != "" {
						def += ", " + linkText
					}
				}
			} else {
				def += root
			}
		}
		def += substrings[idx]
	} else {
		def = rawdef
	}

	return def
}

func findLinkText(link string, pospeech string, definitions map[string]*FullDefinition,
	word string, searchDeclensions bool, visitedWords map[string]bool,
	matchedLink bool) string {

	upper := strings.ToUpper(link)

	def := definitions[upper]
	var bestCandidate *SingleDefinition
	for _, sd := range def.parts {
		if sd.partOfSpeech == pospeech {
			if searchDeclensions && strings.Contains(sd.declensions, word) {
				// found it.
				return expandRaw(sd.nopospeech, sd.word, definitions, visitedWords,
					matchedLink)
			}
			if !searchDeclensions {
				// If we're not searching declensions, this must be expanding
				// a link with {} (indicating a relevant word).
				// In this case, prefer finding a link with the same part
				// of speech that doesn't have a <> link (a declension link)
				if rootRe.MatchString(sd.nopospeech) {
					bestCandidate = sd
					continue
				} else {
					return expandRaw(sd.nopospeech, sd.word, definitions, visitedWords,
						matchedLink)
				}
			}
		}
	}
	if bestCandidate != nil {
		return expandRaw(bestCandidate.nopospeech, bestCandidate.word, definitions,
			visitedWords, matchedLink)
	}

	return ""
}
