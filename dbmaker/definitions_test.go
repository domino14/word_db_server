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

	def := expandRaw(sd.raw, "HIPNESS", definitions, make(map[string]bool), false)
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

	def := expandRaw(sd.raw, "RUM", definitions, make(map[string]bool), false)
	assert.Equal(t,
		`odd (unusual (not usual)) [adj RUMMER, RUMMEST]`,
		def)
}

func TestUnescapeHtmlEntity(t *testing.T) {
	def := `used instead of {ldquo}me?{rdquo} to feign surprise when accused of something [interj]`
	expanded := expandRaw(def, "MOI", nil, make(map[string]bool), false)
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

	expanded := expandRaw(sd.raw, "EMEERS", definitions, make(map[string]bool), false)
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

	expanded := expandRaw(`<os=n> [n]`, "ORA", definitions, make(map[string]bool), false)
	assert.Equal(t,
		`OS, an orifice (a mouth or mouthlike opening) [n]`, expanded)

	expanded = expandRaw(`<os=n> [n]`, "OSSA", definitions, make(map[string]bool), false)
	assert.Equal(t, `OS, a bone [n]`, expanded)

	expanded = expandRaw(`<os=n> [n]`, "OSAR", definitions, make(map[string]bool), false)
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

func TestExpandTrickyCase(t *testing.T) {
	definitions := map[string]*FullDefinition{}
	addToDefinitions(
		"BICEPS",
		`<bicep=n> [n] / an arm muscle [n BICEPSES]`,
		definitions)
	addToDefinitions(
		"BICEP",
		`{biceps=n} [n BICEPS]`,
		definitions)

	userVisibleDefinitions := expandDefinitions(definitions)
	assert.Equal(t, map[string]string{
		"BICEPS": "BICEP, biceps [n]\nan arm muscle [n BICEPSES]",
		"BICEP":  "biceps (an arm muscle) [n BICEPS]",
	}, userVisibleDefinitions)
}

func TestExpandAnotherTrickyCase(t *testing.T) {
	definitions := map[string]*FullDefinition{}
	addToDefinitions(
		"PRISSY",
		`excessively or affectedly proper [adj PRISSIER, PRISSIEST] : PRISSILY [adv] / one who is {prissy=n} [n PRISSIES]`,
		definitions,
	)
	userVisibleDefinitions := expandDefinitions(definitions)
	assert.Equal(t, map[string]string{
		"PRISSY": "excessively or affectedly proper [adj PRISSIER, PRISSIEST] : PRISSILY [adv]" + "\n" +
			"one who is prissy [n PRISSIES]",
	}, userVisibleDefinitions)
}

func TestExpandPeperoncini(t *testing.T) {
	definitions := map[string]*FullDefinition{}
	addToDefinitions(
		"PEPERONCINI",
		`{peperoncino=n} [n PEPERONCINIS, PEPERONCINO]`,
		definitions,
	)
	addToDefinitions(
		"PEPERONCINO",
		`<peperoncini=n> [n]`,
		definitions,
	)
	userVisibleDefinitions := expandDefinitions(definitions)
	assert.Equal(t, map[string]string{
		"PEPERONCINI": "peperoncino (peperoncini) [n PEPERONCINIS, PEPERONCINO]",
		"PEPERONCINO": "PEPERONCINI, peperoncino [n]",
	}, userVisibleDefinitions)
}

func TestManspread(t *testing.T) {
	definitions := map[string]*FullDefinition{}
	addToDefinitions(
		"MANSPREADING",
		`the act of {manspreading=v} [n MANSPREADINGS] / <manspread=v> [v]`,
		definitions,
	)
	addToDefinitions(
		"MANSPREAD",
		`[v MANSPREADING, MANSPREADS]`,
		definitions,
	)
	userVisibleDefinitions := expandDefinitions(definitions)
	assert.Equal(t, map[string]string{
		"MANSPREADING": "the act of manspreading [n MANSPREADINGS]" + "\n" +
			"MANSPREAD [v]",
		"MANSPREAD": "[v MANSPREADING, MANSPREADS]",
	}, userVisibleDefinitions)
}

func TestPaver(t *testing.T) {
	definitions := map[string]*FullDefinition{}
	addToDefinitions(
		"PAVER",
		`one that {paves=v} [n PAVERS]`,
		definitions,
	)
	addToDefinitions(
		"PAVES",
		`<pave=v> [v]`,
		definitions,
	)
	addToDefinitions(
		"PAVE",
		`to cover with material that forms a firm, level surface [v PAVED, PAVES, PAVING]`,
		definitions,
	)
	userVisibleDefinitions := expandDefinitions(definitions)
	assert.Equal(t, map[string]string{
		"PAVER": "one that paves (to cover with material that forms a firm, level surface) [n PAVERS]",
		"PAVES": "PAVE, to cover with material that forms a firm, level surface [v]",
		"PAVE":  "to cover with material that forms a firm, level surface [v PAVED, PAVES, PAVING]",
	}, userVisibleDefinitions)
}
