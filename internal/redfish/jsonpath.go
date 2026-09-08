package redfish

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// step is one element of a JSON path: either an object key or an array index.
type step struct {
	name    string
	index   int
	isIndex bool
}

// jsonPath locates a value inside a response body.
type jsonPath []step

// String renders the path the way it is shown in the footer, for example
// "Links.Chassis[0]" or "Oem.Hpe.Thermal".
func (p jsonPath) String() string {
	var b strings.Builder

	for _, s := range p {
		if s.isIndex {
			b.WriteByte('[')
			b.WriteString(strconv.Itoa(s.index))
			b.WriteByte(']')

			continue
		}

		if b.Len() > 0 {
			b.WriteByte('.')
		}

		b.WriteString(s.name)
	}

	return b.String()
}

// child returns the path of the named member of the value at p.
func (p jsonPath) child(name string) jsonPath {
	return append(p.clone(), step{name: name})
}

// element returns the path of the i-th element of the array at p.
func (p jsonPath) element(i int) jsonPath {
	return append(p.clone(), step{index: i, isIndex: true})
}

// clone copies p so that appending to it cannot disturb a sibling branch of the
// walk that shares the same backing array.
func (p jsonPath) clone() jsonPath {
	cloned := make(jsonPath, len(p))
	copy(cloned, p)

	return cloned
}

// label is the short name shown in the link pane: the last key, carrying any
// array indices that follow it.
func (p jsonPath) label() string {
	last := -1

	for i, s := range p {
		if !s.isIndex {
			last = i
		}
	}

	if last < 0 {
		return p.String()
	}

	return p[last:].String()
}

// firstName returns the outermost object key, or the empty string.
func (p jsonPath) firstName() string {
	for _, s := range p {
		if !s.isIndex {
			return s.name
		}
	}

	return ""
}

// indexOfKey returns the position of the first step with the given key name, or
// -1 when the path does not traverse it.
func (p jsonPath) indexOfKey(name string) int {
	for i, s := range p {
		if !s.isIndex && s.name == name {
			return i
		}
	}

	return -1
}

// nameAfter returns the first key name that follows position i.
func (p jsonPath) nameAfter(i int) string {
	for _, s := range p[i+1:] {
		if !s.isIndex {
			return s.name
		}
	}

	return ""
}

// member is one key and value of a JSON object, in document order.
type member struct {
	key   string
	value json.RawMessage
}

// objectMembers decodes raw as a JSON object, preserving the order in which the
// service wrote its keys. Redfish services order their properties meaningfully,
// and decoding into a map would throw that away.
func objectMembers(raw json.RawMessage) ([]member, bool) {
	dec := json.NewDecoder(bytes.NewReader(raw))

	opening, err := dec.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, false
	}

	var members []member

	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return nil, false
		}

		name, ok := key.(string)
		if !ok {
			return nil, false
		}

		var value json.RawMessage

		err = dec.Decode(&value)
		if err != nil {
			return nil, false
		}

		members = append(members, member{key: name, value: value})
	}

	return members, true
}

// arrayElements decodes raw as a JSON array.
func arrayElements(raw json.RawMessage) ([]json.RawMessage, bool) {
	var elements []json.RawMessage

	err := json.Unmarshal(raw, &elements)
	if err != nil {
		return nil, false
	}

	return elements, true
}

// stringValue decodes raw as a JSON string.
func stringValue(raw json.RawMessage) (string, bool) {
	var s string

	err := json.Unmarshal(raw, &s)
	if err != nil {
		return "", false
	}

	return s, true
}
