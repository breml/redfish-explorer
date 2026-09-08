package redfish

import (
	"encoding/json"
	"slices"
)

// OEMTypes lists the distinct "@odata.type" values a response carries below an
// "Oem" key, for example "#HpeThermalExt.v2_0_0.HpeThermalExt".
//
// It is advisory: the header uses it to say which vendor schemas a resource
// pulls in, and it never affects navigation. The rule is the same one that
// marks a link as OEM — the value sits below a key named "Oem" — rather than a
// guess about which schema names belong to DMTF, because the whole point is to
// find vendor schemas nobody has a list of.
func OEMTypes(body []byte) []string {
	if !json.Valid(body) {
		return nil
	}

	var types []string

	collectOEMTypes(body, outsideOEM, &types)
	slices.Sort(types)

	return slices.Compact(types)
}

// scope says whether a walk has passed below an "Oem" key.
type scope int

const (
	// outsideOEM is the standard part of a document.
	outsideOEM scope = iota
	// belowOEM is anywhere under an "Oem" key.
	belowOEM
)

// collectOEMTypes walks a JSON value, gathering the "@odata.type" values found
// once the walk is below an "Oem" key.
func collectOEMTypes(raw json.RawMessage, where scope, types *[]string) {
	members, ok := objectMembers(raw)
	if ok {
		collectOEMTypesFromObject(members, where, types)

		return
	}

	elements, ok := arrayElements(raw)
	if ok {
		for _, element := range elements {
			collectOEMTypes(element, where, types)
		}
	}
}

// collectOEMTypesFromObject handles the object case of collectOEMTypes.
func collectOEMTypesFromObject(members []member, where scope, types *[]string) {
	for _, m := range members {
		if m.key == odataType && where == belowOEM {
			value, ok := stringValue(m.value)
			if ok && value != "" {
				*types = append(*types, value)
			}

			continue
		}

		next := where
		if m.key == oemKey {
			next = belowOEM
		}

		collectOEMTypes(m.value, next, types)
	}
}
