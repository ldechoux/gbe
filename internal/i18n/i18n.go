// Package i18n holds the translations of the user interface.
//
// Each language is a JSON file in locales/, named after its code (en.json,
// fr.json...), embedded in the binary: adding a language only takes a new
// file. Messages missing from a language fall back to English.
package i18n

import (
	"embed"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strings"
)

// Default is the language used when none was chosen.
const Default = "en"

// Locale is one language.
type Locale struct {
	Code string // file name without extension, e.g. "fr"
	Name string // native name shown in the menu, e.g. "Francais"
	msgs map[string]string
}

//go:embed locales/*.json
var files embed.FS

var (
	locales []*Locale // sorted by code
	byCode  = map[string]*Locale{}
)

func init() {
	entries, err := files.ReadDir("locales")
	if err != nil {
		panic(err)
	}
	for _, e := range entries {
		data, err := files.ReadFile("locales/" + e.Name())
		if err != nil {
			panic(err)
		}
		l, err := parse(strings.TrimSuffix(e.Name(), path.Ext(e.Name())), data)
		if err != nil {
			panic(fmt.Sprintf("i18n: %s: %v", e.Name(), err))
		}
		locales = append(locales, l)
		byCode[l.Code] = l
	}
	sort.Slice(locales, func(i, j int) bool { return locales[i].Code < locales[j].Code })
	if byCode[Default] == nil {
		panic("i18n: missing locales/" + Default + ".json")
	}
}

func parse(code string, data []byte) (*Locale, error) {
	var f struct {
		Name     string            `json:"name"`
		Messages map[string]string `json:"messages"`
	}
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	if f.Name == "" {
		return nil, fmt.Errorf("no name")
	}
	return &Locale{Code: code, Name: f.Name, msgs: f.Messages}, nil
}

// Languages returns the available languages, sorted by code.
func Languages() []*Locale { return locales }

// Has reports whether code is an available language.
func Has(code string) bool { return byCode[code] != nil }

// Get returns the language code, or the default one if it does not exist.
func Get(code string) *Locale {
	if l := byCode[code]; l != nil {
		return l
	}
	return byCode[Default]
}

// T returns the message key, formatted with args like fmt.Sprintf. A message
// missing from l is taken from the default language; an unknown key is
// returned as is.
func (l *Locale) T(key string, args ...any) string {
	msg, ok := l.msgs[key]
	if !ok {
		if msg, ok = byCode[Default].msgs[key]; !ok {
			msg = key
		}
	}
	if len(args) > 0 {
		return fmt.Sprintf(msg, args...)
	}
	return msg
}

// Has reports whether l (not the fallback) defines key.
func (l *Locale) Has(key string) bool {
	_, ok := l.msgs[key]
	return ok
}

// Keys returns the keys l defines, sorted.
func (l *Locale) Keys() []string {
	keys := make([]string, 0, len(l.msgs))
	for k := range l.msgs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
