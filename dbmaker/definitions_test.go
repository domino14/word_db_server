package dbmaker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateSingleDefinitionComplexDeclensions(t *testing.T) {
	part := `to cast an evil spell upon [v HEXED, HEXES, HEXING]`
	sd := createSingleDefinition(1, "HEX", part)
	assert.Equal(t, &SingleDefinition{
		word:         "HEX",
		raw:          part,
		partOfSpeech: "v",
		declensions:  "HEXED, HEXES, HEXING",
		nopospeech:   "to cast an evil spell upon",
	}, sd)
}

func TestCreateSingleDefinitionNoDeclensions(t *testing.T) {
	part := `used to represent a hiccup [interj]`
	sd := createSingleDefinition(1, "HIC", part)
	assert.Equal(t, &SingleDefinition{
		word:         "HIC",
		raw:          part,
		partOfSpeech: "interj",
		declensions:  "",
		nopospeech:   "used to represent a hiccup",
	}, sd)
}

func TestCreateSingleDefinitionNoPartOfSpeech(t *testing.T) {
	part := ``
	sd := createSingleDefinition(1, "", part)
	assert.Equal(t, &SingleDefinition{
		word:         "",
		raw:          part,
		partOfSpeech: "",
		declensions:  "",
		nopospeech:   "",
	}, sd)
}

// BIS {twice=adv} [adv] / <bi=n> [n]

func TestExpandPartSimpleLinks(t *testing.T) {
	sd := createSingleDefinition(1,
		"HIPNESS",
		`the state of being {hip=adj} [n HIPNESSES]`)
	definitions := map[string]*FullDefinition{}
	addToDefinitions(
		"HIP",
		`aware of the most current styles and trends [adj HIPPER, HIPPEST] / to build a type of roof [v HIPPED, HIPPING, HIPS]`,
		definitions)

	def := expandRaw(sd.raw, "HIPNESS", definitions)
	assert.Equal(t,
		`the state of being hip (aware of the most current styles and trends) [n HIPNESSES]`,
		def)
}

func TestExpandPartMultipleLinks(t *testing.T) {
	sd := createSingleDefinition(1,
		"RUM", `{odd=adj} [adj RUMMER, RUMMEST]`)

	definitions := map[string]*FullDefinition{}
	addToDefinitions("ODD",
		`{unusual=adj} [adj ODDER, ODDEST] / one that is odd [n ODDS]`,
		definitions)
	addToDefinitions("UNUSUAL", `not usual [adj]`, definitions)

	def := expandRaw(sd.raw, "RUM", definitions)
	assert.Equal(t,
		`odd (unusual (not usual)) [adj RUMMER, RUMMEST]`,
		def)
}

func TestUnescapeHtmlEntity(t *testing.T) {
	def := `used instead of {ldquo}me?{rdquo} to feign surprise when accused of something [interj]`
	expanded := expandRaw(def, "MOI", nil)
	assert.Equal(t,
		`used instead of “me?” to feign surprise when accused of something [interj]`,
		expanded)
}

func TestExpandRootLink(t *testing.T) {
	// The definition is for EMEERS
	sd := createSingleDefinition(1, "EMEERS", `<emeer=n> [n]`)
	definitions := map[string]*FullDefinition{}
	addToDefinitions("EMEER", `{emir=n} [n EMEERS]`, definitions)
	addToDefinitions("EMIR", `an Arab chieftain or prince [n EMIRS]`, definitions)

	expanded := expandRaw(sd.raw, "EMEERS", definitions)
	assert.Equal(t,
		`EMEER, emir (an Arab chieftain or prince) [n]`,
		expanded)
}

func TestExpandMatchDeclensions(t *testing.T) {
	definitions := map[string]*FullDefinition{}
	addToDefinitions(
		"OS",
		`a bone [n OSSA] / an {esker=n} [n OSAR] / an {orifice=n} [n ORA]`,
		definitions)
	addToDefinitions("ORIFICE", `a mouth or mouthlike opening [n ORIFICES]`,
		definitions)
	addToDefinitions("ESKER", `a narrow ridge of gravel and sand [n ESKERS]`,
		definitions)
	addToDefinitions("OSSA", `<os=n> [n]`, definitions)

	expanded := expandRaw(`<os=n> [n]`, "ORA", definitions)
	assert.Equal(t,
		`OS, an orifice (a mouth or mouthlike opening) [n]`, expanded)

	expanded = expandRaw(`<os=n> [n]`, "OSSA", definitions)
	assert.Equal(t, `OS, a bone [n]`, expanded)

	expanded = expandRaw(`<os=n> [n]`, "OSAR", definitions)
	assert.Equal(t, `OS, an esker (a narrow ridge of gravel and sand) [n]`,
		expanded)
}

func TestExpandDefinitions(t *testing.T) {
	definitions := map[string]*FullDefinition{}
	addToDefinitions(
		"OS",
		`a bone [n OSSA] / an {esker=n} [n OSAR] / an {orifice=n} [n ORA]`,
		definitions)
	addToDefinitions("ORIFICE", `a mouth or mouthlike opening [n ORIFICES]`,
		definitions)
	addToDefinitions("ESKER", `a narrow ridge of gravel and sand [n ESKERS]`,
		definitions)
	addToDefinitions("OSSA", `<os=n> [n]`, definitions)

	userVisibleDefinitions := expandDefinitions(definitions)

	assert.Equal(t, map[string]string{
		"ORIFICE": "a mouth or mouthlike opening [n ORIFICES]",
		"ESKER":   "a narrow ridge of gravel and sand [n ESKERS]",
		"OSSA":    "OS, a bone [n]",
		"OS": "a bone [n OSSA]" + "\n" +
			"an esker (a narrow ridge of gravel and sand) [n OSAR]" + "\n" +
			"an orifice (a mouth or mouthlike opening) [n ORA]",
	}, userVisibleDefinitions)
}
