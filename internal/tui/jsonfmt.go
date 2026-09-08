package tui

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/breml/redfish-explorer/internal/redfish"
)

const (
	// jsonIndent is the indentation of the pretty-printed body.
	jsonIndent = "  "

	// oemKey is the object below which everything is a vendor extension.
	oemKey = "Oem"

	// annotationPrefix introduces an OData or Redfish annotation key.
	annotationPrefix = "@"
)

// Pretty indents a JSON document and reports whether it could be read at all.
//
// It uses json.Indent rather than decoding and re-encoding, because that
// preserves the order in which the service wrote its keys, and Redfish services
// order them meaningfully: the "@odata" keys come first, and related properties
// are grouped. Re-marshalling through a map would sort everything alphabetically
// and destroy that.
func Pretty(body []byte) (string, bool) {
	var out bytes.Buffer

	err := json.Indent(&out, body, "", jsonIndent)
	if err != nil {
		return "", false
	}

	return out.String(), true
}

// Colorize styles pretty-printed JSON so that the things worth finding stand
// out: the links, and anything a vendor added.
func Colorize(indented string, theme Theme) string {
	s := &jsonScanner{src: indented, theme: theme}
	s.run()

	return s.out.String()
}

// jsonScanner walks pretty-printed JSON, tracking enough structure to know
// which keys sit below an "Oem" object.
type jsonScanner struct {
	src   string
	pos   int
	out   strings.Builder
	theme Theme

	// containers holds the key that introduced each open object or array,
	// innermost last.
	containers []string
	// key is the most recent key seen at the current level.
	key string
}

// run scans the whole document.
func (s *jsonScanner) run() {
	for s.pos < len(s.src) {
		switch c := s.src[s.pos]; {
		case c == '"':
			s.scanString()

		case c == '{' || c == '[':
			s.openContainer(c)

		case c == '}' || c == ']':
			s.closeContainer(c)

		case c == '-' || (c >= '0' && c <= '9'):
			s.scanNumber()

		default:
			s.scanLiteralOrByte()
		}
	}
}

// scanString handles a quoted string, which may be a key or a value.
func (s *jsonScanner) scanString() {
	start := s.pos
	end := s.stringEnd()
	raw := s.src[start:end]
	s.pos = end

	text := unquote(raw)

	if s.isKey() {
		s.key = text
		s.out.WriteString(s.keyStyle(text).Render(raw))

		return
	}

	s.out.WriteString(s.valueStyle(text).Render(raw))
}

// stringEnd returns the index just past the closing quote of the string
// starting at s.pos, honouring backslash escapes.
func (s *jsonScanner) stringEnd() int {
	for i := s.pos + 1; i < len(s.src); i++ {
		switch s.src[i] {
		case '\\':
			i++

		case '"':
			return i + 1

		default:
			// An ordinary character inside the string.
		}
	}

	return len(s.src)
}

// isKey reports whether the string just scanned is followed by a colon.
func (s *jsonScanner) isKey() bool {
	for i := s.pos; i < len(s.src); i++ {
		switch s.src[i] {
		case ' ', '\t', '\n', '\r':
			continue

		case ':':
			return true

		default:
			return false
		}
	}

	return false
}

// keyStyle picks the colour for an object key.
func (s *jsonScanner) keyStyle(text string) lipgloss.Style {
	// The Oem key itself is drawn like what it contains, so the block reads as
	// one thing.
	if s.insideOEM() || text == oemKey {
		return s.theme.OEM
	}

	if strings.HasPrefix(text, annotationPrefix) {
		return s.theme.JSONAnnotation
	}

	return s.theme.JSONKey
}

// valueStyle picks the colour for a string value.
func (s *jsonScanner) valueStyle(text string) lipgloss.Style {
	if redfish.IsResourcePath(text) {
		return s.theme.JSONLink
	}

	if s.insideOEM() {
		return s.theme.OEM
	}

	return s.theme.JSONString
}

// openContainer records which key introduced the object or array being opened.
func (s *jsonScanner) openContainer(c byte) {
	s.containers = append(s.containers, s.key)
	s.key = ""
	s.out.WriteString(s.theme.JSONPunct.Render(string(c)))
	s.pos++
}

// closeContainer leaves the innermost object or array.
func (s *jsonScanner) closeContainer(c byte) {
	if len(s.containers) > 0 {
		s.key = s.containers[len(s.containers)-1]
		s.containers = s.containers[:len(s.containers)-1]
	}

	s.out.WriteString(s.theme.JSONPunct.Render(string(c)))
	s.pos++
}

// scanNumber handles a numeric literal.
func (s *jsonScanner) scanNumber() {
	start := s.pos
	for s.pos < len(s.src) && isNumberByte(s.src[s.pos]) {
		s.pos++
	}

	s.out.WriteString(s.theme.JSONNumber.Render(s.src[start:s.pos]))
}

// scanLiteralOrByte handles true, false and null, and passes anything else —
// punctuation and whitespace — through.
func (s *jsonScanner) scanLiteralOrByte() {
	rest := s.src[s.pos:]

	for _, literal := range []string{"true", "false"} {
		if strings.HasPrefix(rest, literal) {
			s.out.WriteString(s.theme.JSONBool.Render(literal))
			s.pos += len(literal)

			return
		}
	}

	if strings.HasPrefix(rest, "null") {
		s.out.WriteString(s.theme.JSONNull.Render("null"))
		s.pos += len("null")

		return
	}

	c := s.src[s.pos]
	if c == ':' || c == ',' {
		s.out.WriteString(s.theme.JSONPunct.Render(string(c)))
	} else {
		s.out.WriteByte(c)
	}

	s.pos++
}

// insideOEM reports whether the scan is below an "Oem" object.
func (s *jsonScanner) insideOEM() bool {
	return slices.Contains(s.containers, oemKey)
}

// isNumberByte reports whether c can appear in a JSON number.
func isNumberByte(c byte) bool {
	return c == '-' || c == '+' || c == '.' || c == 'e' || c == 'E' || (c >= '0' && c <= '9')
}

// unquote returns the text of a quoted JSON string, falling back to the raw
// form when it cannot be decoded.
func unquote(raw string) string {
	var text string

	err := json.Unmarshal([]byte(raw), &text)
	if err != nil {
		return strings.Trim(raw, `"`)
	}

	return text
}
