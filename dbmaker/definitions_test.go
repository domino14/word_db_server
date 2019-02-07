package dbmaker

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCreateSingleDefinitionComplexDeclensions(t *testing.T) {
	part := `to cast an evil spell upon [v HEXED, HEXES, HEXING]`
	sd := createSingleDefinition(1, part)
	assert.Equal(t, &SingleDefinition{
		raw:          part,
		partOfSpeech: "v",
		declensions:  "HEXED, HEXES, HEXING",
		nopospeech:   "to cast an evil spell upon",
	}, sd)
}

func TestCreateSingleDefinitionNoDeclensions(t *testing.T) {
	part := `used to represent a hiccup [interj]`
	sd := createSingleDefinition(1, part)
	assert.Equal(t, &SingleDefinition{
		raw:          part,
		partOfSpeech: "interj",
		declensions:  "",
		nopospeech:   "used to represent a hiccup",
	}, sd)
}

func TestCreateSingleDefinitionNoPartOfSpeech(t *testing.T) {
	part := ``
	sd := createSingleDefinition(1, part)
	assert.Equal(t, &SingleDefinition{
		raw:          part,
		partOfSpeech: "",
		declensions:  "",
		nopospeech:   "",
	}, sd)
}

// BIS {twice=adv} [adv] / <bi=n> [n]

func TestExpandPartSimpleLinks(t *testing.T) {
	sd := &SingleDefinition{
		raw: `the state of being {hip=adj} [n HIPNESSES]`,
	}
	definitions := map[string]*FullDefinition{}
	definitions["HIP"] = &FullDefinition{
		parts: []*SingleDefinition{
			createSingleDefinition(1, `aware of the most current styles and trends [adj HIPPER, HIPPEST]`),
			createSingleDefinition(2, `to build a type of roof [v HIPPED, HIPPING, HIPS]`),
		},
	}
	def := expandPart(0, sd, definitions)
	assert.Equal(t,
		`the state of being hip (aware of the most current styles and trends) [n HIPNESSES]`,
		def)
}
