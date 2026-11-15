package osmium

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// Relation is the part of an OSM relation the pipeline cares about.
type Relation struct {
	ID         int64
	Tags       map[string]string
	SubareaIDs []int64 // member relations with role "subarea"
}

// ParseRelations reads OPL text (`osmium cat -f opl`) and returns its
// relations. Lines for other object types are ignored.
//
// OPL escapes characters that would break its syntax as %hex%, which is
// undone for tag keys and values.
func ParseRelations(r io.Reader) ([]Relation, error) {
	var out []Relation
	br := bufio.NewReaderSize(r, 1<<20)
	for {
		line, err := br.ReadString('\n')
		if len(line) > 0 && line[0] == 'r' {
			if rel, ok := parseRelation(strings.TrimRight(line, "\r\n")); ok {
				out = append(out, rel)
			}
		}
		if err == io.EOF {
			return out, nil
		}
		if err != nil {
			return out, err
		}
	}
}

func parseRelation(line string) (Relation, bool) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return Relation{}, false
	}
	id, err := strconv.ParseInt(fields[0][1:], 10, 64)
	if err != nil {
		return Relation{}, false
	}
	rel := Relation{ID: id, Tags: map[string]string{}}
	for _, f := range fields[1:] {
		switch f[0] {
		case 'T':
			if len(f) == 1 {
				continue
			}
			for _, kv := range strings.Split(f[1:], ",") {
				k, v, _ := strings.Cut(kv, "=")
				rel.Tags[unescape(k)] = unescape(v)
			}
		case 'M':
			for _, m := range strings.Split(f[1:], ",") {
				if m == "" || m[0] != 'r' {
					continue
				}
				ref, role, _ := strings.Cut(m[1:], "@")
				if role != "subarea" {
					continue
				}
				if n, err := strconv.ParseInt(ref, 10, 64); err == nil {
					rel.SubareaIDs = append(rel.SubareaIDs, n)
				}
			}
		}
	}
	return rel, true
}

// unescape decodes OPL %hex% escapes, where hex is the Unicode code point.
func unescape(s string) string {
	if !strings.Contains(s, "%") {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '%' {
			b.WriteByte(s[i])
			continue
		}
		end := strings.IndexByte(s[i+1:], '%')
		if end < 0 {
			b.WriteString(s[i:])
			break
		}
		if cp, err := strconv.ParseInt(s[i+1:i+1+end], 16, 32); err == nil {
			b.WriteRune(rune(cp))
			i += end + 1
			continue
		}
		b.WriteByte('%')
	}
	return b.String()
}
