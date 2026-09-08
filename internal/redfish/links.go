package redfish

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
)

// Kind says what sort of thing a link points at, which decides how it is drawn
// and what happens when it is followed.
type Kind int

const (
	// KindResource is a navigable resource, found as an "@odata.id".
	KindResource Kind = iota
	// KindAction is an action target. It answers to POST, not to GET, and is
	// listed so that vendor actions can be discovered at all.
	KindAction
	// KindURI is a path-like string found under a key other than "@odata.id",
	// which some vendors use instead of a proper navigation link.
	KindURI
	// KindHeader is a link taken from a response header.
	KindHeader
)

// Group titles, in the order they are shown.
const (
	groupResource    = "Resource"
	groupMembers     = "Members"
	groupLinks       = "Links"
	groupActions     = "Actions"
	groupAnnotations = "Annotations"
	groupHeaders     = "Headers"

	// oemGroupPrefix introduces a per-vendor OEM group.
	oemGroupPrefix = "Oem · "

	// unnamedVendor labels OEM content whose vendor cannot be determined.
	unnamedVendor = "unnamed"
)

// JSON keys that carry meaning for the walk.
const (
	odataID      = "@odata.id"
	odataContext = "@odata.context"
	odataType    = "@odata.type"
	oemKey       = "Oem"
	actionsKey   = "Actions"
	membersKey   = "Members"
	linksKey     = "Links"
	targetKey    = "target"
	actionInfo   = "@Redfish.ActionInfo"
)

// redfishPrefix is the prefix every Redfish resource path starts with. Some
// services spell the version in upper case, hence the second form.
const (
	redfishPrefix      = "/redfish/v1/"
	redfishPrefixUpper = "/redfish/V1/"
)

// Link is one thing the user can navigate to from the current resource.
type Link struct {
	// Label is the short name shown in the link pane.
	Label string
	// Target is the resource path the link points at.
	Target string
	// JSONPath says where in the document the link was found.
	JSONPath string
	// Kind decides how the link is drawn and whether it can be followed.
	Kind Kind
	// OEM marks a link found below an "Oem" key.
	OEM bool
	// Vendor is the vendor key an OEM link sits under.
	Vendor string
	// ActionInfo is the ActionInfo resource of an action, when it advertises one.
	ActionInfo string
}

// Group is a set of links sharing an origin within the document.
type Group struct {
	// Title names the origin, for example "Links" or "Oem · Hpe".
	Title string
	// OEM marks a group holding vendor extensions.
	OEM bool
	// Vendor is the vendor key of an OEM group.
	Vendor string
	// Links are the group's links, sorted by label.
	Links []Link
}

// ErrNotJSON reports that a response body could not be read as JSON. Some BMCs
// answer with an HTML error page, which is worth showing but holds no links.
var ErrNotJSON = errors.New("response body is not valid JSON")

// annotationKeys are the Redfish annotations whose contents are collected into
// their own group. They are a common hiding place for vendor behaviour.
func annotationKeys() []string {
	return []string{
		"@Redfish.Settings",
		"@Redfish.ActionInfo",
		"@Redfish.CollectionCapabilities",
		"@Redfish.OperationApplyTimeSupport",
	}
}

// ExtractLinks finds everything the user can navigate to from a response, and
// returns it grouped by where it was found, along with the resource's own self
// link. The self link is never among the groups: the brief asks for the current
// location to stay out of the navigation.
//
// The whole response is needed, not just its body, because the Location headers
// are one of the sources.
func ExtractLinks(resp *Response) ([]Group, string, error) {
	if resp == nil {
		return nil, "", nil
	}

	ext := &extractor{base: responseBase(resp)}

	err := ext.walkBody(resp.Body)
	if err != nil {
		return nil, "", err
	}

	ext.headerLinks(resp)
	ext.dropShadowedURIs()

	return ext.groups(), ext.self, nil
}

// found is a link together with the group it belongs to.
type found struct {
	link  Link
	group string
}

// extractor accumulates links while walking one response.
type extractor struct {
	found []found
	self  string
	// base is the URL the response came from, against which a target spelled
	// as an absolute URL is recognised as belonging to the same service.
	base *url.URL
}

// responseBase parses the URL a response was fetched from. A response built by
// hand, as in a test, need not carry one.
func responseBase(resp *Response) *url.URL {
	if resp.URL == "" {
		return nil
	}

	base, err := url.Parse(resp.URL)
	if err != nil {
		return nil
	}

	return base
}

// resourceTarget reduces a link target to a path on the service the response
// came from. Some services spell "@odata.id" and action targets as absolute
// URLs, which would otherwise be appended to the endpoint a second time when
// the link is followed or rendered as a curl command. A target on another host
// is left as it stands, so that following it is still refused rather than
// quietly redirected to the connected machine.
func (e *extractor) resourceTarget(target string) string {
	if strings.HasPrefix(target, "/") {
		return target
	}

	parsed, err := url.Parse(target)
	if err != nil || !parsed.IsAbs() {
		return target
	}

	if e.base == nil || !strings.EqualFold(parsed.Host, e.base.Host) {
		return target
	}

	return parsed.RequestURI()
}

// walkBody reads the body and collects every link it holds.
func (e *extractor) walkBody(body []byte) error {
	// A body-less response, such as a 204, is not an error.
	if strings.TrimSpace(string(body)) == "" {
		return nil
	}

	if !json.Valid(body) {
		return ErrNotJSON
	}

	e.walk(body, nil)

	return nil
}

// walk descends through one JSON value.
func (e *extractor) walk(raw json.RawMessage, path jsonPath) {
	members, ok := objectMembers(raw)
	if ok {
		e.object(members, path)

		return
	}

	elements, ok := arrayElements(raw)
	if ok {
		for i, element := range elements {
			e.walk(element, path.element(i))
		}

		return
	}

	value, ok := stringValue(raw)
	if ok {
		e.pathLikeString(value, path)
	}
}

// object handles one JSON object: it records the object's own link, if it has
// one, then descends into the members that can hold further links.
func (e *extractor) object(members []member, path jsonPath) {
	e.selfOrLink(members, path)

	for _, m := range members {
		switch m.key {
		case odataID, odataContext, odataType:
			// Handled above, or pointing at a schema rather than a resource.

		case actionsKey:
			e.actions(m.value, path.child(m.key))

		default:
			e.walk(m.value, path.child(m.key))
		}
	}
}

// selfOrLink records the "@odata.id" of an object, as the self link when the
// object is the document root and as a navigable link otherwise.
func (e *extractor) selfOrLink(members []member, path jsonPath) {
	target := ""

	for _, m := range members {
		if m.key == odataID {
			target, _ = stringValue(m.value)

			break
		}
	}

	if target == "" {
		return
	}

	target = e.resourceTarget(target)

	if len(path) == 0 {
		e.self = target

		return
	}

	e.add(Link{
		Label:    linkLabel(path, target),
		Target:   target,
		JSONPath: path.String(),
		Kind:     KindResource,
	}, path)
}

// linkLabel names a link. Collection members are labelled by the last segment
// of their target, so that a collection reads as a list of member IDs rather
// than as Members[0], Members[1] and so on.
func linkLabel(path jsonPath, target string) string {
	if path.firstName() == membersKey {
		segment := lastSegment(target)
		if segment != "" {
			return segment
		}
	}

	return path.label()
}

// actions handles the Actions object, whose members are action targets that
// answer to POST rather than GET.
func (e *extractor) actions(raw json.RawMessage, path jsonPath) {
	members, ok := objectMembers(raw)
	if !ok {
		return
	}

	for _, m := range members {
		// Under Actions.Oem some services nest the actions below a vendor key
		// and others list them directly, so descend through either shape.
		if m.key == oemKey || !strings.HasPrefix(m.key, "#") {
			e.actions(m.value, path.child(m.key))

			continue
		}

		e.action(m.value, path.child(m.key))
	}
}

// action records one action target together with its ActionInfo, if it has one.
func (e *extractor) action(raw json.RawMessage, path jsonPath) {
	members, ok := objectMembers(raw)
	if !ok {
		return
	}

	var target, info string

	for _, m := range members {
		switch m.key {
		case targetKey:
			target, _ = stringValue(m.value)

		case actionInfo:
			info, _ = stringValue(m.value)

		default:
			// The rest of an action object describes its parameters.
		}
	}

	if target == "" {
		return
	}

	// The ActionInfo is fetched when the action is followed, so it needs the
	// same reduction to a path as the target itself.
	e.add(Link{
		Label:      path.label(),
		Target:     e.resourceTarget(target),
		JSONPath:   path.String(),
		Kind:       KindAction,
		ActionInfo: e.resourceTarget(info),
	}, path)
}

// pathLikeString records a string that looks like a resource path but was not
// written as an "@odata.id". Some vendors use plain Uri, href or Target keys.
func (e *extractor) pathLikeString(value string, path jsonPath) {
	if !IsResourcePath(value) {
		return
	}

	e.add(Link{
		Label:    path.label(),
		Target:   value,
		JSONPath: path.String(),
		Kind:     KindURI,
	}, path)
}

// headerLinks records the resource paths advertised in response headers.
func (e *extractor) headerLinks(resp *Response) {
	for _, name := range []string{"Location", "Content-Location"} {
		value := resp.Header.Get(name)
		if value == "" {
			continue
		}

		target := headerTarget(value)
		if target == "" {
			continue
		}

		e.found = append(e.found, found{
			group: groupHeaders,
			link: Link{
				Label:    name,
				Target:   target,
				JSONPath: name + " header",
				Kind:     KindHeader,
			},
		})
	}
}

// add files a link under the group its JSON path puts it in.
func (e *extractor) add(link Link, path jsonPath) {
	title, oem, vendor := groupFor(path)

	link.OEM = oem
	link.Vendor = vendor

	e.found = append(e.found, found{link: link, group: title})
}

// dropShadowedURIs removes path-like strings that are already reachable as a
// proper navigation link, so the same resource is not listed twice.
func (e *extractor) dropShadowedURIs() {
	resources := map[string]bool{}

	// The self link is not among the found links, but a path-like string that
	// repeats it still points at the current resource, which the contract keeps
	// out of the groups.
	if e.self != "" {
		resources[e.self] = true
	}

	for _, f := range e.found {
		if f.link.Kind == KindResource {
			resources[f.link.Target] = true
		}
	}

	e.found = slices.DeleteFunc(e.found, func(f found) bool {
		return f.link.Kind == KindURI && resources[f.link.Target]
	})
}

// groups collects the links into their groups, in display order, dropping
// duplicates within a group and empty groups altogether. The links of a group
// are sorted by label, so that a group reads as an alphabetical list rather
// than in the order the service happened to write the document.
func (e *extractor) groups() []Group {
	byTitle := map[string]*Group{}
	seen := map[string]bool{}

	var titles []string

	for _, f := range e.found {
		if seen[f.group+"\x00"+f.link.Target] {
			continue
		}

		seen[f.group+"\x00"+f.link.Target] = true

		group, ok := byTitle[f.group]
		if !ok {
			group = &Group{Title: f.group, OEM: f.link.OEM, Vendor: f.link.Vendor}
			byTitle[f.group] = group
			titles = append(titles, f.group)
		}

		group.Links = append(group.Links, f.link)
	}

	slices.SortStableFunc(titles, func(a, b string) int {
		rank := groupRank(a) - groupRank(b)
		if rank != 0 {
			return rank
		}

		// Only the OEM groups share a rank, and their titles carry the vendor.
		return strings.Compare(a, b)
	})

	groups := make([]Group, 0, len(titles))

	for _, title := range titles {
		group := byTitle[title]
		slices.SortStableFunc(group.Links, compareLinks)
		groups = append(groups, *group)
	}

	return groups
}

// compareLinks orders two links of a group alphabetically by label, ignoring
// case so that the order matches how the labels read. Labels need not be
// unique, so the target breaks the tie and keeps the order stable.
func compareLinks(a Link, b Link) int {
	label := strings.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label))
	if label != 0 {
		return label
	}

	return strings.Compare(a.Target, b.Target)
}

// groupFor decides which group a link found at path belongs to.
func groupFor(path jsonPath) (title string, oem bool, vendor string) {
	// An OEM link is an OEM link wherever it sits, Actions.Oem included.
	oemAt := path.indexOfKey(oemKey)
	if oemAt >= 0 {
		name := oemVendor(path.nameAfter(oemAt))
		if name == "" {
			return oemGroupPrefix + unnamedVendor, true, ""
		}

		return oemGroupPrefix + name, true, name
	}

	for _, annotation := range annotationKeys() {
		if path.indexOfKey(annotation) >= 0 {
			return groupAnnotations, false, ""
		}
	}

	switch path.firstName() {
	case actionsKey:
		return groupActions, false, ""

	case linksKey:
		return groupLinks, false, ""

	case membersKey:
		return groupMembers, false, ""

	default:
		return groupResource, false, ""
	}
}

// oemVendor names the vendor a link belongs to, given the key that follows the
// "Oem" key. That key is usually the vendor itself, but where a service lists
// OEM actions directly under Actions.Oem it is an action name, which carries
// the vendor by the "#Vendor.Action" convention.
func oemVendor(name string) string {
	if !strings.HasPrefix(name, "#") {
		return name
	}

	vendor, _, found := strings.Cut(strings.TrimPrefix(name, "#"), ".")
	if !found {
		return ""
	}

	return vendor
}

// groupRank orders the groups for display. OEM groups share one rank, between
// the standard groups and the header links; groups breaks the tie by title,
// which orders them by vendor.
func groupRank(title string) int {
	order := []string{groupResource, groupMembers, groupLinks, groupActions, groupAnnotations}

	rank := slices.Index(order, title)
	if rank >= 0 {
		return rank
	}

	if strings.HasPrefix(title, oemGroupPrefix) {
		return len(order)
	}

	return len(order) + 1
}

// IsResourcePath reports whether a string looks like a Redfish resource path.
func IsResourcePath(value string) bool {
	return strings.HasPrefix(value, redfishPrefix) || strings.HasPrefix(value, redfishPrefixUpper)
}

// headerTarget turns a Location header into a resource path, accepting both the
// relative and the absolute form.
func headerTarget(value string) string {
	if IsResourcePath(value) {
		return value
	}

	parsed, err := url.Parse(value)
	if err != nil {
		return ""
	}

	if IsResourcePath(parsed.Path) {
		return parsed.Path
	}

	return ""
}

// lastSegment returns the final path segment of a resource path.
func lastSegment(target string) string {
	trimmed := strings.TrimRight(target, "/")

	cut := strings.LastIndex(trimmed, "/")
	if cut < 0 {
		return trimmed
	}

	segment, err := url.PathUnescape(trimmed[cut+1:])
	if err != nil {
		return trimmed[cut+1:]
	}

	return segment
}

// String names a Kind, for tests and debugging.
func (k Kind) String() string {
	switch k {
	case KindResource:
		return "resource"

	case KindAction:
		return "action"

	case KindURI:
		return "uri"

	case KindHeader:
		return "header"

	default:
		return fmt.Sprintf("Kind(%d)", int(k))
	}
}
