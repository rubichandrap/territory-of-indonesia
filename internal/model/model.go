// Package model defines the administrative levels and the Region type that
// every other package in this repository passes around.
package model

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/paulmach/orb"
)

// Level is an OpenStreetMap admin_level that this project publishes.
type Level int

const (
	Province Level = 4 // provinsi
	Regency  Level = 5 // kabupaten / kota
	District Level = 6 // kecamatan / distrik
	Village  Level = 7 // desa / kelurahan
)

// Levels lists every published level, top down.
func Levels() []Level { return []Level{Province, Regency, District, Village} }

// Valid reports whether l is one of the published levels.
func (l Level) Valid() bool { return l >= Province && l <= Village }

// String is the singular, lower-case English name used in property values.
func (l Level) String() string {
	switch l {
	case Province:
		return "province"
	case Regency:
		return "regency"
	case District:
		return "district"
	case Village:
		return "village"
	}
	return "level" + strconv.Itoa(int(l))
}

// Plural is the English plural used for directory and file names.
func (l Level) Plural() string {
	switch l {
	case Regency:
		return "regencies"
	default:
		return l.String() + "s"
	}
}

// Region is one administrative boundary: an OSM relation with an assembled
// polygon, plus what the build derives from it.
type Region struct {
	OSMID        int64 // OSM relation id, always positive
	Level        Level
	Name         string // OSM "name" (the local name)
	NameEN       string // OSM "name:en"
	OfficialName string // OSM "official_name", usually carries the kabupaten/kota/kecamatan/desa prefix
	Wikidata     string
	ISO3166_2    string // only set on provinces ("ID-JB")
	Code         string // Kemendagri code, dotted ("32.73.01"); empty when OSM has none
	Kind         string // kabupaten, kota, kecamatan, desa, ...; empty when unknown

	Geometry orb.Geometry // orb.Polygon or orb.MultiPolygon, RFC 7946 winding

	// Derived by the build.
	Bound  orb.Bound
	Area   float64 // planar area in square degrees; only used to rank candidates
	Center orb.Point

	// Filled in by the hierarchy step.
	ParentID  int64   // nearest ancestor that exists in the dataset; 0 for provinces
	Ancestors []int64 // all ancestors, root first
}

var codePatterns = map[Level]*regexp.Regexp{
	Province: regexp.MustCompile(`^\d{2}$`),
	Regency:  regexp.MustCompile(`^\d{2}\.\d{2}$`),
	District: regexp.MustCompile(`^\d{2}\.\d{2}\.\d{2}$`),
	Village:  regexp.MustCompile(`^\d{2}\.\d{2}\.\d{2}\.\d{4}$`),
}

// NormalizeCode returns code if it has the shape of a Kemendagri code for the
// given level, and "" otherwise.
func NormalizeCode(l Level, code string) string {
	code = strings.TrimSpace(code)
	if p, ok := codePatterns[l]; ok && p.MatchString(code) {
		return code
	}
	return ""
}

// CodeHasPrefix reports whether child is a descendant code of parent
// ("32.73.01" is under "32.73"). Both must be non-empty.
func CodeHasPrefix(child, parent string) bool {
	return parent != "" && strings.HasPrefix(child, parent+".")
}

var kindByPrefix = map[Level]map[string]string{
	Regency:  {"kabupaten": "kabupaten", "kota": "kota"},
	District: {"kecamatan": "kecamatan", "distrik": "distrik", "kapanewon": "kapanewon", "kemantren": "kemantren"},
	Village: {
		"desa": "desa", "kelurahan": "kelurahan", "kalurahan": "kalurahan",
		"pekon": "pekon", "nagari": "nagari", "gampong": "gampong", "kampung": "kampung",
	},
}

// DeriveKind works out the local-government term for a region (kota vs
// kabupaten, desa vs kelurahan, ...). It prefers the wording in official_name,
// then name, and falls back to the numbering rules of the Kemendagri codes.
// It returns "" when none of them decide.
func DeriveKind(l Level, officialName, name, code string) string {
	words := kindByPrefix[l]
	for _, s := range []string{officialName, name} {
		first, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(s)), " ")
		if k, ok := words[first]; ok {
			return k
		}
	}
	switch l {
	case Regency:
		// Kota codes are NN.71 and up (Kota Administrasi in DKI Jakarta
		// included); kabupaten codes are NN.01 to NN.70.
		if len(code) == 5 {
			if n, err := strconv.Atoi(code[3:]); err == nil && n > 0 {
				if n >= 71 {
					return "kota"
				}
				return "kabupaten"
			}
		}
	case Village:
		// Village codes: first digit of the last block is 1 for a kelurahan
		// and 2 for a desa.
		if len(code) == 13 {
			switch code[9] {
			case '1':
				return "kelurahan"
			case '2':
				return "desa"
			}
		}
	}
	return ""
}
