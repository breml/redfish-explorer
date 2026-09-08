package redfish_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/breml/redfish-explorer/internal/redfish"
)

func TestOEMTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want []string
	}{
		{
			name: "types below Oem are collected",
			body: `{
              "@odata.type": "#ComputerSystem.v1_22_0.ComputerSystem",
              "Oem": {"Hpe": {"@odata.type": "#HpeComputerSystemExt.v2_11_0.HpeComputerSystemExt"}}
            }`,
			want: []string{"#HpeComputerSystemExt.v2_11_0.HpeComputerSystemExt"},
		},
		{
			name: "standard types are ignored",
			body: `{
              "@odata.type": "#ComputerSystem.v1_22_0.ComputerSystem",
              "Bios": {"@odata.type": "#Bios.v1_2_0.Bios"}
            }`,
		},
		{
			name: "nested and repeated types are reported once, sorted",
			body: `{
              "Oem": {
                "Dell": {
                  "@odata.type": "#DellOem.v1_3_0.DellOemResources",
                  "DellSystem": {"@odata.type": "#DellSystem.v1_4_0.DellSystem"},
                  "Again": {"@odata.type": "#DellSystem.v1_4_0.DellSystem"}
                }
              }
            }`,
			want: []string{"#DellOem.v1_3_0.DellOemResources", "#DellSystem.v1_4_0.DellSystem"},
		},
		{
			name: "types inside an array below Oem are found",
			body: `{"Oem": {"Ami": {"Items": [{"@odata.type": "#AmiThing.v1_0_0.AmiThing"}]}}}`,
			want: []string{"#AmiThing.v1_0_0.AmiThing"},
		},
		{name: "no oem block", body: `{"@odata.id": "/redfish/v1"}`},
		{name: "empty body", body: ``},
		{name: "not json", body: `<html></html>`},
		{name: "type is not a string", body: `{"Oem": {"X": {"@odata.type": 42}}}`},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got := redfish.OEMTypes([]byte(test.body))
			if !equal(got, test.want) {
				t.Errorf("OEMTypes() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestOEMTypesOnVendorFixtures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		fixture string
		want    []string
	}{
		{fixture: "hpe-system.json", want: []string{"#HpeComputerSystemExt.v2_11_0.HpeComputerSystemExt"}},
		{
			fixture: "dell-system.json",
			want:    []string{"#DellOem.v1_3_0.DellOemResources", "#DellSystem.v1_4_0.DellSystem"},
		},
		{fixture: "lenovo-system.json", want: []string{"#LenovoComputerSystem.v1_0_0.LenovoComputerSystem"}},
		{fixture: "multivendor.json", want: []string{"#AmiChassis.v1_0_0.AmiChassis"}},
	}

	for _, test := range tests {
		t.Run(test.fixture, func(t *testing.T) {
			t.Parallel()

			body, err := os.ReadFile(filepath.Join("testdata", "oem", test.fixture))
			if err != nil {
				t.Fatalf("reading fixture: %v", err)
			}

			if got := redfish.OEMTypes(body); !equal(got, test.want) {
				t.Errorf("OEMTypes() = %v, want %v", got, test.want)
			}
		})
	}
}
