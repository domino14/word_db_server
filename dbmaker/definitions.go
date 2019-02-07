package dbmaker

import (
	"log"
	"regexp"
	"strings"
)

type SingleDefinition struct {
	partOfSpeech string
	// declensions looks something like `ORBED, ORBING, ORBS`
	declensions string
	raw         string
	// nopos is the definition without the part of speech and declensions.
	nopospeech  string
	userVisible string
}

type FullDefinition struct {
	raw string

	parts []*SingleDefinition
}

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
	for _, fd := range definitions {
		// First, split into parts
		parts := strings.Split(fd.raw, " / ")
		fd.parts = make([]*SingleDefinition, len(parts))
		for idx, part := range parts {
			sd := createSingleDefinition(idx, part)
			fd.parts[idx] = sd
		}
	}
	userVisibleDefs := make(map[string]string)
	// Now cycle through all the definitions again, and recursively follow
	// links, etc.
	for word, fd := range definitions {
		userVisibleDefs[word] = expand(fd, definitions)
	}
	return userVisibleDefs
}

func createSingleDefinition(idx int, part string) *SingleDefinition {
	sd := &SingleDefinition{
		raw: part,
	}
	// Just get the inflection for now.
	inflections := inflectionRe.FindStringSubmatch(part)
	pospeech := ""
	declensions := ""
	log.Println("IJF", inflections)
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

func expand(fd *FullDefinition, definitions map[string]*FullDefinition) string {
	expandedParts := []string{}
	for idx, part := range fd.parts {
		expandedParts = append(expandedParts, expandPart(idx, part, definitions))
	}

	return strings.Join(expandedParts, "\n")
}

func expandPart(defnum int, sd *SingleDefinition, definitions map[string]*FullDefinition) string {
	// replaced := linkRe.ReplaceAllStringFunc(sd.raw, func(match string) string {

	// 	log.Println("[DEBUG] MATCH ", match)
	// 	return "FOO"
	// })

	log.Println("Trying to find submatches for linkRe, raw is", sd.raw)
	submatches := linkRe.FindAllStringSubmatch(sd.raw, -1)

	def := ""
	if len(submatches) > 0 {
		substrings := linkRe.Split(sd.raw, -1)
		log.Println("substrings", substrings)
		def += substrings[0]
		idx := 0

		for _, submatch := range submatches {
			link := submatch[1]
			pospeech := submatch[2]
			idx++
			def += link + " (" + findLinkText(link, pospeech, definitions) + ")"
		}
		def += substrings[idx]
	}
	return def
}

func findLinkText(link string, pospeech string, definitions map[string]*FullDefinition) string {
	upper := strings.ToUpper(link)

	def := definitions[upper]

	for _, sd := range def.parts {
		if sd.partOfSpeech == pospeech {
			log.Println("Found sd", sd)
			// && strings.Contains(sd.declensions, upper) {
			// found it.
			return sd.nopospeech
		}
	}
	return ""
}
