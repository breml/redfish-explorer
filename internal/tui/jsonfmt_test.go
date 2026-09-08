package tui_test

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/breml/redfish-explorer/internal/tui"
)

func TestPretty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
		ok   bool
	}{
		{
			name: "indents with two spaces",
			body: `{"a":1}`,
			want: "{\n  \"a\": 1\n}",
			ok:   true,
		},
		{
			name: "nested",
			body: `{"a":{"b":[1,2]}}`,
			want: "{\n  \"a\": {\n    \"b\": [\n      1,\n      2\n    ]\n  }\n}",
			ok:   true,
		},
		{name: "html is not json", body: "<html></html>"},
		{name: "truncated", body: `{"a":`},
		{name: "empty", body: ""},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, ok := tui.Pretty([]byte(test.body))
			if ok != test.ok {
				t.Fatalf("Pretty ok = %t, want %t", ok, test.ok)
			}

			if ok && got != test.want {
				t.Errorf("Pretty() =\n%q\nwant\n%q", got, test.want)
			}
		})
	}
}

func TestPrettyPreservesKeyOrder(t *testing.T) {
	t.Parallel()

	// Redfish services put the @odata keys first and group related properties.
	// Re-marshalling through a map would sort them and destroy that.
	body := `{"@odata.id":"/redfish/v1","Zebra":1,"Alpha":2}`

	got, ok := tui.Pretty([]byte(body))
	if !ok {
		t.Fatal("Pretty: want it to succeed")
	}

	wantOrder := []string{"@odata.id", "Zebra", "Alpha"}

	at := 0
	for _, key := range wantOrder {
		i := strings.Index(got[at:], key)
		if i < 0 {
			t.Fatalf("key %q out of order in:\n%s", key, got)
		}

		at += i
	}
}

// renderedAs reports whether text appears in the colorized document drawn in
// the given style. Comparing against the style's own output avoids hard-coding
// escape sequences that would break on any theme change.
func renderedAs(colorized string, style lipgloss.Style, text string) bool {
	return strings.Contains(colorized, style.Render(`"`+text+`"`))
}

func TestColorizeLeavesTheTextIntact(t *testing.T) {
	t.Parallel()

	pretty, ok := tui.Pretty([]byte(`{"a":1,"b":"x","c":true,"d":null,"e":[1,2],"f":{"g":-1.5e3}}`))
	if !ok {
		t.Fatal("Pretty: want it to succeed")
	}

	got := tui.Colorize(pretty, tui.NewTheme())
	if ansi.Strip(got) != pretty {
		t.Errorf("Colorize changed the text:\n%q\nwant\n%q", ansi.Strip(got), pretty)
	}
}

func TestColorizeHighlightsLinksAndAnnotations(t *testing.T) {
	t.Parallel()

	theme := tui.NewTheme()

	pretty, _ := tui.Pretty([]byte(`{
      "@odata.id": "/redfish/v1/Systems/1",
      "Name": "Server",
      "Description": "not a path"
    }`))
	got := tui.Colorize(pretty, theme)

	if !renderedAs(got, theme.JSONAnnotation, "@odata.id") {
		t.Error("an annotation key must be drawn in the annotation style")
	}

	if !renderedAs(got, theme.JSONKey, "Name") {
		t.Error("an ordinary key must be drawn in the key style")
	}

	if !renderedAs(got, theme.JSONLink, "/redfish/v1/Systems/1") {
		t.Error("a resource path must be drawn as a link")
	}

	if !renderedAs(got, theme.JSONString, "not a path") {
		t.Error("an ordinary string must be drawn in the string style")
	}
}

func TestColorizeMarksEverythingBelowOem(t *testing.T) {
	t.Parallel()

	theme := tui.NewTheme()

	pretty, _ := tui.Pretty([]byte(`{
      "Name": "Server",
      "Oem": {
        "Hpe": {
          "Bay": 3,
          "Nested": {"Deep": "value"}
        }
      }
    }`))
	got := tui.Colorize(pretty, theme)

	// The Oem key itself is drawn like what it contains, so the block reads as
	// one thing however deeply it nests.
	for _, key := range []string{"Oem", "Hpe", "Bay", "Nested", "Deep"} {
		if !renderedAs(got, theme.OEM, key) {
			t.Errorf("%q should be drawn in the OEM colour", key)
		}
	}

	if renderedAs(got, theme.OEM, "Name") {
		t.Error("a standard key must not be drawn in the OEM colour")
	}
}

func TestColorizeHandlesAwkwardStrings(t *testing.T) {
	t.Parallel()

	// A quote, a colon and a brace inside string values must not be mistaken
	// for structure.
	bodies := []string{
		`{"a":"say \"hi\"","b":"c: d","c":"{not an object}"}`,
		`{"a":"back\\slash"}`,
		`{"":"empty key"}`,
		`[]`,
		`[[[1]]]`,
		`{"Oem":{}}`,
	}

	for _, body := range bodies {
		pretty, ok := tui.Pretty([]byte(body))
		if !ok {
			t.Fatalf("Pretty(%s): want it to succeed", body)
		}

		if got := ansi.Strip(tui.Colorize(pretty, tui.NewTheme())); got != pretty {
			t.Errorf("Colorize(%s) changed the text:\n%q\nwant\n%q", body, got, pretty)
		}
	}
}

func TestColorizeDoesNotChangeWidth(t *testing.T) {
	t.Parallel()

	pretty, _ := tui.Pretty([]byte(`{"@odata.id":"/redfish/v1","Oem":{"Hpe":{"Bay":3}}}`))
	got := tui.Colorize(pretty, tui.NewTheme())

	for i, line := range strings.Split(got, "\n") {
		want := lipgloss.Width(strings.Split(pretty, "\n")[i])
		if lipgloss.Width(line) != want {
			t.Errorf("line %d is %d columns, want %d", i, lipgloss.Width(line), want)
		}
	}
}
