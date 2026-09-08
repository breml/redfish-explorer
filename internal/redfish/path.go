package redfish

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// RootPath is the well-known entry point of every Redfish service.
const RootPath = "/redfish/v1"

// rootCrumb labels the service root in a breadcrumb trail.
const rootCrumb = "root"

// NormalizeHost turns a user-supplied host into the endpoint URL rfx connects
// to. It accepts a bare host, a host:port pair, or a full URL, and keeps only
// the scheme and the authority, so a URL pasted from a browser works too.
func NormalizeHost(host string) (string, error) {
	host = strings.TrimSpace(host)
	if host == "" {
		return "", errors.New("empty host")
	}

	// A leading slash means a path was given where a host was expected.
	if strings.HasPrefix(host, "/") {
		return "", fmt.Errorf("%q is a path, not a host", host)
	}

	if !strings.Contains(host, "://") {
		host = "https://" + host
	}

	parsed, err := url.Parse(host)
	if err != nil {
		return "", fmt.Errorf("parsing host %q: %w", host, err)
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("unsupported scheme %q, want http or https", parsed.Scheme)
	}

	if parsed.Host == "" {
		return "", fmt.Errorf("no host in %q", host)
	}

	return parsed.Scheme + "://" + parsed.Host, nil
}

// Parent returns the resource one level up from resource. It never returns
// anything above the service root.
func Parent(resource string) string {
	resource = strings.TrimRight(resource, "/")
	if resource == RootPath || !strings.HasPrefix(resource, RootPath) {
		return RootPath
	}

	cut := strings.LastIndex(resource, "/")
	if cut < len(RootPath) {
		return RootPath
	}

	return resource[:cut]
}

// Breadcrumb splits a resource into the trail shown in the header, starting at
// the service root. Segments are URL-decoded, since Redfish IDs may be escaped.
func Breadcrumb(resource string) []string {
	crumbs := []string{rootCrumb}

	rest := strings.TrimPrefix(strings.TrimRight(resource, "/"), RootPath)
	rest = strings.Trim(rest, "/")
	if rest == "" {
		return crumbs
	}

	for segment := range strings.SplitSeq(rest, "/") {
		decoded, err := url.PathUnescape(segment)
		if err != nil {
			decoded = segment
		}

		crumbs = append(crumbs, decoded)
	}

	return crumbs
}
