package redfish_test

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/breml/redfish-explorer/internal/redfish"
)

// extract runs ExtractLinks over a body with no response headers.
func extract(t *testing.T, body string) (groups []redfish.Group, self string) {
	t.Helper()

	groups, self, err := redfish.ExtractLinks(&redfish.Response{Body: []byte(body)})
	if err != nil {
		t.Fatalf("ExtractLinks: unexpected error: %v", err)
	}

	return groups, self
}

// extractFixture runs ExtractLinks over a fixture file.
func extractFixture(t *testing.T, name string) []redfish.Group {
	t.Helper()

	body, err := os.ReadFile(filepath.Join("testdata", "oem", name))
	if err != nil {
		t.Fatalf("reading fixture %s: %v", name, err)
	}

	groups, _, err := redfish.ExtractLinks(&redfish.Response{Body: body})
	if err != nil {
		t.Fatalf("ExtractLinks(%s): unexpected error: %v", name, err)
	}

	return groups
}

// group returns the named group, failing the test if it is absent.
func group(t *testing.T, groups []redfish.Group, title string) redfish.Group {
	t.Helper()

	for _, g := range groups {
		if g.Title == title {
			return g
		}
	}

	t.Fatalf("group %q not found in %v", title, titles(groups))

	return redfish.Group{}
}

// titles lists the group titles in order.
func titles(groups []redfish.Group) []string {
	out := make([]string, 0, len(groups))
	for _, g := range groups {
		out = append(out, g.Title)
	}

	return out
}

// labels lists a group's link labels in order.
func labels(g redfish.Group) []string {
	out := make([]string, 0, len(g.Links))
	for _, l := range g.Links {
		out = append(out, l.Label)
	}

	return out
}

// find returns the link with the given label, failing the test if absent.
func find(t *testing.T, g redfish.Group, label string) redfish.Link {
	t.Helper()

	for _, l := range g.Links {
		if l.Label == label {
			return l
		}
	}

	t.Fatalf("link %q not found in group %q, have %v", label, g.Title, labels(g))

	return redfish.Link{}
}

// equal compares two string slices.
func equal(a []string, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}

const serviceRootBody = `{
  "@odata.id": "/redfish/v1",
  "@odata.type": "#ServiceRoot.v1_15_0.ServiceRoot",
  "@odata.context": "/redfish/v1/$metadata#ServiceRoot.ServiceRoot",
  "Systems": {"@odata.id": "/redfish/v1/Systems"},
  "Chassis": {"@odata.id": "/redfish/v1/Chassis"},
  "Managers": {"@odata.id": "/redfish/v1/Managers"},
  "Links": {"Sessions": {"@odata.id": "/redfish/v1/SessionService/Sessions"}},
  "Oem": {"Contoso": {"ResourceDirectory": {"@odata.id": "/redfish/v1/ResourceDirectory"}}}
}`

func TestExtractLinksRecordsSelfAndExcludesIt(t *testing.T) {
	t.Parallel()

	groups, self := extract(t, serviceRootBody)

	if self != "/redfish/v1" {
		t.Errorf("self = %q, want %q", self, "/redfish/v1")
	}

	for _, g := range groups {
		for _, l := range g.Links {
			if l.Target == self {
				t.Errorf("self link %q must not appear in group %q", self, g.Title)
			}
		}
	}
}

func TestExtractLinksGroupsAndOrder(t *testing.T) {
	t.Parallel()

	groups, _ := extract(t, serviceRootBody)

	want := []string{"Resource", "Links", "Oem · Contoso"}
	if !equal(titles(groups), want) {
		t.Errorf("groups = %v, want %v", titles(groups), want)
	}

	// Document order is preserved: Redfish services order meaningfully.
	wantResource := []string{"Systems", "Chassis", "Managers"}
	if got := labels(group(t, groups, "Resource")); !equal(got, wantResource) {
		t.Errorf("Resource links = %v, want %v", got, wantResource)
	}
}

func TestExtractLinksMarksOEM(t *testing.T) {
	t.Parallel()

	groups, _ := extract(t, serviceRootBody)

	oem := group(t, groups, "Oem · Contoso")
	if !oem.OEM || oem.Vendor != "Contoso" {
		t.Errorf("group OEM = %t, Vendor = %q, want true and %q", oem.OEM, oem.Vendor, "Contoso")
	}

	link := find(t, oem, "ResourceDirectory")
	if !link.OEM || link.Vendor != "Contoso" {
		t.Errorf("link OEM = %t, Vendor = %q, want true and %q", link.OEM, link.Vendor, "Contoso")
	}

	if link.JSONPath != "Oem.Contoso.ResourceDirectory" {
		t.Errorf("JSONPath = %q, want %q", link.JSONPath, "Oem.Contoso.ResourceDirectory")
	}
}

func TestExtractLinksLabelsMembersByID(t *testing.T) {
	t.Parallel()

	body := `{
      "@odata.id": "/redfish/v1/Systems",
      "Members@odata.count": 3,
      "Members": [
        {"@odata.id": "/redfish/v1/Systems/1"},
        {"@odata.id": "/redfish/v1/Systems/System.Embedded.1"},
        {"@odata.id": "/redfish/v1/Systems/BMC%20One"}
      ]
    }`

	groups, _ := extract(t, body)

	want := []string{"1", "System.Embedded.1", "BMC One"}
	if got := labels(group(t, groups, "Members")); !equal(got, want) {
		t.Errorf("Members labels = %v, want %v", got, want)
	}
}

func TestExtractLinksLabelsArrayElements(t *testing.T) {
	t.Parallel()

	body := `{
      "@odata.id": "/redfish/v1/Systems/1",
      "Links": {
        "Chassis": [
          {"@odata.id": "/redfish/v1/Chassis/1"},
          {"@odata.id": "/redfish/v1/Chassis/2"}
        ]
      }
    }`

	groups, _ := extract(t, body)

	want := []string{"Chassis[0]", "Chassis[1]"}
	if got := labels(group(t, groups, "Links")); !equal(got, want) {
		t.Errorf("Links labels = %v, want %v", got, want)
	}
}

func TestExtractLinksActions(t *testing.T) {
	t.Parallel()

	body := `{
      "@odata.id": "/redfish/v1/Systems/1",
      "Actions": {
        "#ComputerSystem.Reset": {
          "target": "/redfish/v1/Systems/1/Actions/ComputerSystem.Reset",
          "@Redfish.ActionInfo": "/redfish/v1/Systems/1/ResetActionInfo"
        },
        "#ComputerSystem.Decommission": {
          "target": "/redfish/v1/Systems/1/Actions/ComputerSystem.Decommission"
        },
        "Oem": {
          "#Contoso.SecureErase": {
            "target": "/redfish/v1/Systems/1/Actions/Oem/Contoso.SecureErase"
          }
        }
      }
    }`

	groups, _ := extract(t, body)

	actions := group(t, groups, "Actions")

	want := []string{"#ComputerSystem.Reset", "#ComputerSystem.Decommission"}
	if got := labels(actions); !equal(got, want) {
		t.Errorf("Actions labels = %v, want %v", got, want)
	}

	reset := find(t, actions, "#ComputerSystem.Reset")
	if reset.Kind != redfish.KindAction {
		t.Errorf("Kind = %s, want %s", reset.Kind, redfish.KindAction)
	}

	if reset.ActionInfo != "/redfish/v1/Systems/1/ResetActionInfo" {
		t.Errorf("ActionInfo = %q, want the ResetActionInfo path", reset.ActionInfo)
	}

	if reset.OEM {
		t.Error("a standard action must not be marked OEM")
	}

	// An action without an ActionInfo leaves the field empty rather than guessing.
	if got := find(t, actions, "#ComputerSystem.Decommission").ActionInfo; got != "" {
		t.Errorf("ActionInfo = %q, want it empty", got)
	}

	// Everything below Actions.Oem is an OEM action.
	oemAction := find(t, group(t, groups, "Oem · Contoso"), "#Contoso.SecureErase")
	if !oemAction.OEM || oemAction.Kind != redfish.KindAction {
		t.Errorf("OEM action = %+v, want an OEM action", oemAction)
	}
}

func TestExtractLinksAnnotations(t *testing.T) {
	t.Parallel()

	body := `{
      "@odata.id": "/redfish/v1/Systems/1",
      "@Redfish.Settings": {
        "SettingsObject": {"@odata.id": "/redfish/v1/Systems/1/Settings"}
      },
      "@Redfish.CollectionCapabilities": {
        "Capabilities": [
          {"CapabilitiesObject": {"@odata.id": "/redfish/v1/Systems/Capabilities"}}
        ]
      }
    }`

	groups, _ := extract(t, body)

	want := []string{"SettingsObject", "CapabilitiesObject"}
	if got := labels(group(t, groups, "Annotations")); !equal(got, want) {
		t.Errorf("Annotations labels = %v, want %v", got, want)
	}
}

func TestExtractLinksPathLikeStrings(t *testing.T) {
	t.Parallel()

	body := `{
      "@odata.id": "/redfish/v1/Systems/1",
      "SmartStorageUri": "/redfish/v1/Systems/1/SmartStorage",
      "UpperCaseUri": "/redfish/V1/Systems/1/Odd",
      "NotAPath": "Systems/1",
      "Description": "See /redfish/v1 for details"
    }`

	groups, _ := extract(t, body)

	want := []string{"SmartStorageUri", "UpperCaseUri"}
	if got := labels(group(t, groups, "Resource")); !equal(got, want) {
		t.Errorf("Resource labels = %v, want %v", got, want)
	}

	if got := find(t, group(t, groups, "Resource"), "SmartStorageUri").Kind; got != redfish.KindURI {
		t.Errorf("Kind = %s, want %s", got, redfish.KindURI)
	}
}

func TestExtractLinksSuppressesShadowedURIs(t *testing.T) {
	t.Parallel()

	// The same target reached both ways is listed once, as a proper link.
	body := `{
      "@odata.id": "/redfish/v1/Systems/1",
      "BiosUri": "/redfish/v1/Systems/1/Bios",
      "Bios": {"@odata.id": "/redfish/v1/Systems/1/Bios"}
    }`

	groups, _ := extract(t, body)

	resource := group(t, groups, "Resource")
	if len(resource.Links) != 1 {
		t.Fatalf("Resource links = %v, want exactly one", labels(resource))
	}

	if resource.Links[0].Kind != redfish.KindResource {
		t.Errorf("Kind = %s, want %s", resource.Links[0].Kind, redfish.KindResource)
	}
}

func TestExtractLinksKeepsTheSameTargetInDifferentGroups(t *testing.T) {
	t.Parallel()

	// Where a link was found is part of what the user is exploring.
	body := `{
      "@odata.id": "/redfish/v1/Systems/1",
      "Thermal": {"@odata.id": "/redfish/v1/Chassis/1/Thermal"},
      "Oem": {"Hpe": {"Thermal": {"@odata.id": "/redfish/v1/Chassis/1/Thermal"}}}
    }`

	groups, _ := extract(t, body)

	if len(group(t, groups, "Resource").Links) != 1 {
		t.Error("want the standard Thermal link")
	}

	if len(group(t, groups, "Oem · Hpe").Links) != 1 {
		t.Error("want the OEM Thermal link kept as well")
	}
}

func TestExtractLinksDedupesWithinAGroup(t *testing.T) {
	t.Parallel()

	body := `{
      "@odata.id": "/redfish/v1/Systems",
      "Members": [
        {"@odata.id": "/redfish/v1/Systems/1"},
        {"@odata.id": "/redfish/v1/Systems/1"}
      ]
    }`

	groups, _ := extract(t, body)

	if got := len(group(t, groups, "Members").Links); got != 1 {
		t.Errorf("Members links = %d, want 1", got)
	}
}

func TestExtractLinksFromHeaders(t *testing.T) {
	t.Parallel()

	header := http.Header{}
	header.Set("Location", "https://10.0.0.5/redfish/v1/TaskService/Tasks/1")
	header.Set("Content-Location", "/redfish/v1/Systems/1")

	resp := &redfish.Response{
		Header: header,
		Body:   []byte(`{"@odata.id": "/redfish/v1/Systems/1/Actions"}`),
	}

	groups, _, err := redfish.ExtractLinks(resp)
	if err != nil {
		t.Fatalf("ExtractLinks: unexpected error: %v", err)
	}

	headers := group(t, groups, "Headers")

	want := []string{"Location", "Content-Location"}
	if got := labels(headers); !equal(got, want) {
		t.Fatalf("Headers labels = %v, want %v", got, want)
	}

	location := find(t, headers, "Location")
	if location.Kind != redfish.KindHeader {
		t.Errorf("Kind = %s, want %s", location.Kind, redfish.KindHeader)
	}

	// An absolute Location is reduced to its resource path.
	if location.Target != "/redfish/v1/TaskService/Tasks/1" {
		t.Errorf("Target = %q, want the resource path", location.Target)
	}
}

func TestExtractLinksIgnoresNonRedfishHeaders(t *testing.T) {
	t.Parallel()

	header := http.Header{}
	header.Set("Location", "https://10.0.0.5/login.html")

	groups, _, err := redfish.ExtractLinks(&redfish.Response{Header: header, Body: []byte(`{}`)})
	if err != nil {
		t.Fatalf("ExtractLinks: unexpected error: %v", err)
	}

	if len(groups) != 0 {
		t.Errorf("groups = %v, want none", titles(groups))
	}
}

func TestExtractLinksHandlesOddBodies(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantErr    error
		wantGroups int
	}{
		{name: "empty body", body: ""},
		{name: "whitespace only", body: "   \n"},
		{name: "empty object", body: `{}`},
		{name: "html error page", body: "<html><body>nope</body></html>", wantErr: redfish.ErrNotJSON},
		{name: "truncated json", body: `{"@odata.id": `, wantErr: redfish.ErrNotJSON},
		{
			name:       "array at the root",
			body:       `[{"@odata.id": "/redfish/v1/Systems/1"}]`,
			wantGroups: 1,
		},
		{name: "odata.id is not a string", body: `{"Bios": {"@odata.id": 42}}`},
		{name: "odata.id is null", body: `{"Bios": {"@odata.id": null}}`},
		{name: "json null", body: `null`},
		{name: "bare number", body: `42`},
		{
			name:       "oem nested inside links",
			body:       `{"Links": {"Oem": {"Ami": {"Deep": {"@odata.id": "/redfish/v1/X"}}}}}`,
			wantGroups: 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			groups, _, err := redfish.ExtractLinks(&redfish.Response{Body: []byte(test.body)})
			if test.wantErr != nil {
				if !errors.Is(err, test.wantErr) {
					t.Fatalf("error = %v, want %v", err, test.wantErr)
				}

				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(groups) != test.wantGroups {
				t.Errorf("groups = %v, want %d", titles(groups), test.wantGroups)
			}
		})
	}
}

func TestExtractLinksHandlesANilResponse(t *testing.T) {
	t.Parallel()

	groups, self, err := redfish.ExtractLinks(nil)
	if err != nil || groups != nil || self != "" {
		t.Errorf("ExtractLinks(nil) = %v, %q, %v; want nil, \"\", nil", groups, self, err)
	}
}

func TestExtractLinksHPE(t *testing.T) {
	t.Parallel()

	groups := extractFixture(t, "hpe-system.json")

	oem := group(t, groups, "Oem · Hpe")
	if oem.Vendor != "Hpe" {
		t.Errorf("Vendor = %q, want %q", oem.Vendor, "Hpe")
	}

	want := []string{
		"#HpeComputerSystemExt.PowerButton",
		"#HpeComputerSystemExt.SecureSystemErase",
		"PCIDevices",
		"SmartStorage",
		"Memory",
		"Thermal",
	}
	if got := labels(oem); !equal(got, want) {
		t.Errorf("Oem · Hpe labels = %v, want %v", got, want)
	}

	// The OEM Links block sits below Oem, so it is an OEM group, not "Links".
	pci := find(t, oem, "PCIDevices")
	if pci.JSONPath != "Oem.Hpe.Links.PCIDevices" {
		t.Errorf("JSONPath = %q, want %q", pci.JSONPath, "Oem.Hpe.Links.PCIDevices")
	}
}

func TestExtractLinksDell(t *testing.T) {
	t.Parallel()

	groups := extractFixture(t, "dell-system.json")

	oem := group(t, groups, "Oem · Dell")

	want := []string{"DellSystem", "DellNumericSensorCollection"}
	if got := labels(oem); !equal(got, want) {
		t.Errorf("Oem · Dell labels = %v, want %v", got, want)
	}

	reset := find(t, group(t, groups, "Actions"), "#ComputerSystem.Reset")
	if !strings.HasSuffix(reset.ActionInfo, "/ResetActionInfo") {
		t.Errorf("ActionInfo = %q, want it to end in /ResetActionInfo", reset.ActionInfo)
	}
}

func TestExtractLinksLenovo(t *testing.T) {
	t.Parallel()

	groups := extractFixture(t, "lenovo-system.json")

	oem := group(t, groups, "Oem · Lenovo")

	// Uri and href are plain strings, not navigation links, but still reachable.
	want := []string{"Uri", "Processors", "href"}
	if got := labels(oem); !equal(got, want) {
		t.Errorf("Oem · Lenovo labels = %v, want %v", got, want)
	}

	if got := find(t, oem, "Uri").Kind; got != redfish.KindURI {
		t.Errorf("Kind = %s, want %s", got, redfish.KindURI)
	}
}

func TestExtractLinksMultipleVendors(t *testing.T) {
	t.Parallel()

	groups := extractFixture(t, "multivendor.json")

	// Ami is reached both through Links.Oem and through Oem; the two merge into
	// a single group, because the user wants everything Ami together.
	want := []string{"Resource", "Annotations", "Oem · Ami"}
	if got := titles(groups); !equal(got, want) {
		t.Fatalf("groups = %v, want %v", got, want)
	}

	wantAmi := []string{"DeepLink", "Fans"}
	if got := labels(group(t, groups, "Oem · Ami")); !equal(got, wantAmi) {
		t.Errorf("Oem · Ami labels = %v, want %v", got, wantAmi)
	}

	// Supermicro contributes no links, so it must not appear as an empty group.
	for _, g := range groups {
		if g.Title == "Oem · Supermicro" {
			t.Error("Oem · Supermicro has no links and must not be shown")
		}
	}
}
