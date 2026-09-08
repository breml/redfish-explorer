// Package redfish connects to a Redfish service and fetches its resources in
// raw form, so that every response can be shown exactly as the service sent it.
package redfish

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/stmcginnis/gofish"
)

// contentTypeJSON is what rfx asks every Redfish service for.
const contentTypeJSON = "application/json"

// Transport tuning, matching the Go defaults except where a BMC needs slack.
const (
	dialTimeout           = 30 * time.Second
	keepAlive             = 30 * time.Second
	maxIdleConns          = 100
	idleConnTimeout       = 90 * time.Second
	tlsHandshakeTimeout   = 10 * time.Second
	expectContinueTimeout = time.Second
)

// Config describes the service rfx talks to and how requests are rendered.
type Config struct {
	// Endpoint is the scheme and authority of the service, already normalized.
	Endpoint string
	// Username and Password authenticate every request via HTTP basic auth.
	Username string
	Password string
	// Insecure skips TLS certificate verification.
	Insecure bool
	// ShowPassword renders the real password in the curl command.
	ShowPassword bool
	// UserAgent identifies rfx to the service.
	UserAgent string
}

// ServiceInfo holds the ServiceRoot metadata shown in the header.
type ServiceInfo struct {
	RedfishVersion string
	Vendor         string
	Product        string
	// OEMVendors lists the keys found under the ServiceRoot Oem object.
	OEMVendors []string
}

// Client is a connected Redfish service.
type Client struct {
	cfg     Config
	api     *gofish.APIClient
	http    *http.Client
	service ServiceInfo
}

// Response is a single HTTP exchange, preserved whole. Every status code is
// reported here rather than as an error: a 404 or a 501 body is exactly what
// makes a service's coverage visible.
type Response struct {
	// URL is the absolute URL that was requested.
	URL string
	// Path is the resource part of URL, as shown in the header.
	Path string
	// Status is the textual status, for example "200 OK".
	Status string
	// StatusCode is the numeric status.
	StatusCode int
	// Proto is the protocol version the service answered with.
	Proto string
	// Header holds the response headers.
	Header http.Header
	// Body is the response body, read to completion.
	Body []byte
	// Duration covers the round trip including reading the body.
	Duration time.Duration
	// FetchedAt is when the response arrived.
	FetchedAt time.Time
}

// Connect opens a session with the Redfish service described by cfg. It fails
// fast: the caller is expected to report the error and exit before starting a
// terminal UI.
func Connect(ctx context.Context, cfg Config) (*Client, error) {
	api, err := gofish.ConnectContext(ctx, gofish.ClientConfig{
		Endpoint:  cfg.Endpoint,
		Username:  cfg.Username,
		Password:  cfg.Password,
		Insecure:  cfg.Insecure,
		BasicAuth: true,
		// Supplying the transport keeps gofish away from http.DefaultTransport.
		// Left to itself it copies the default transport's *tls.Config pointer
		// and then writes InsecureSkipVerify through it (gofish client.go:164),
		// which both races between concurrent connections and quietly disables
		// certificate verification for everything else in the process.
		// See: https://github.com/stmcginnis/gofish/issues/567
		HTTPClient:        &http.Client{Transport: transport(cfg)},
		NoModifyTransport: true,
	})
	if err != nil {
		return nil, connectError(cfg, err)
	}

	client := &Client{
		cfg:     cfg,
		api:     api,
		http:    api.HTTPClient,
		service: serviceInfo(api.Service),
	}

	err = client.verifyCredentials(ctx)
	if err != nil {
		client.Close()

		return nil, err
	}

	return client, nil
}

// Config returns the configuration the client was built with.
func (c *Client) Config() Config {
	return c.cfg
}

// Service returns the ServiceRoot metadata collected during Connect.
func (c *Client) Service() ServiceInfo {
	return c.service
}

// Close releases any session held by the underlying client.
func (c *Client) Close() {
	c.api.Logout()
}

// Fetch performs a GET and returns the whole exchange. It reports an error only
// when the request could not be made at all; any HTTP status, including 4xx and
// 5xx, is returned as a Response.
func (c *Client) Fetch(ctx context.Context, resource string) (*Response, error) {
	target, err := c.Resolve(resource)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("building request for %s: %w", target, err)
	}

	req.Header.Set("Accept", contentTypeJSON)
	req.Header.Set("User-Agent", c.cfg.UserAgent)
	req.SetBasicAuth(c.cfg.Username, c.cfg.Password)

	start := time.Now()

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("requesting %s: %w", target, err)
	}

	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response body from %s: %w", target, err)
	}

	return &Response{
		URL:        target,
		Path:       req.URL.RequestURI(),
		Status:     resp.Status,
		StatusCode: resp.StatusCode,
		Proto:      resp.Proto,
		Header:     resp.Header,
		Body:       body,
		Duration:   time.Since(start),
		FetchedAt:  time.Now(),
	}, nil
}

// probeTarget picks a resource from the service root that a service is
// expected to protect, in decreasing order of how universally it is present.
func probeTarget(serviceRoot []byte) string {
	var root map[string]json.RawMessage

	err := json.Unmarshal(serviceRoot, &root)
	if err != nil {
		return ""
	}

	for _, key := range []string{"Systems", "Chassis", "Managers", "AccountService", "SessionService"} {
		raw, ok := root[key]
		if !ok {
			continue
		}

		var link struct {
			ID string `json:"@odata.id"`
		}

		err = json.Unmarshal(raw, &link)
		if err == nil && link.ID != "" {
			return link.ID
		}
	}

	return ""
}

// Resolve turns a user-supplied resource into an absolute URL on the connected
// endpoint, refusing anything that would leave it. It is what the cache keys
// on, so that a key stays correct if rfx ever talks to more than one endpoint.
func (c *Client) Resolve(resource string) (string, error) {
	resource = strings.TrimSpace(resource)
	if resource == "" {
		resource = RootPath
	}

	if strings.HasPrefix(resource, "/") {
		return c.cfg.Endpoint + resource, nil
	}

	parsed, err := url.Parse(resource)
	if err != nil {
		return "", fmt.Errorf("parsing resource %q: %w", resource, err)
	}

	if !parsed.IsAbs() {
		return "", fmt.Errorf("resource %q is neither an absolute path nor an absolute URL", resource)
	}

	base, err := url.Parse(c.cfg.Endpoint)
	if err != nil {
		return "", fmt.Errorf("parsing endpoint %q: %w", c.cfg.Endpoint, err)
	}

	// Following a link to another host would silently query a different
	// machine than the one named in the header.
	if !strings.EqualFold(parsed.Host, base.Host) {
		return "", fmt.Errorf("refusing to fetch %s: not on the connected endpoint %s", resource, c.cfg.Endpoint)
	}

	return c.cfg.Endpoint + parsed.RequestURI(), nil
}

// verifyCredentials probes one protected resource so that bad credentials are
// reported before the terminal UI starts.
//
// gofish reads the service root before it configures authentication, and the
// Redfish specification makes the service root readable without credentials, so
// a successful Connect proves nothing about the user name and password. With
// basic auth gofish never validates them either: it only stores them.
func (c *Client) verifyCredentials(ctx context.Context) error {
	root, err := c.Fetch(ctx, RootPath)
	if err != nil {
		return fmt.Errorf("reading %s from %s: %w", RootPath, c.cfg.Endpoint, err)
	}

	target := probeTarget(root.Body)
	if target == "" {
		// Nothing protected is advertised; the first navigation will show the
		// service's own answer.
		return nil
	}

	resp, err := c.Fetch(ctx, target)
	if err != nil {
		return fmt.Errorf("reading %s from %s: %w", target, c.cfg.Endpoint, err)
	}

	// A 403 means the credentials were accepted but the account lacks the
	// privilege, which is a normal thing to discover while exploring.
	if resp.StatusCode == http.StatusUnauthorized {
		return fmt.Errorf("%s: authentication failed for user %q, check --username and --password",
			c.cfg.Endpoint, c.cfg.Username)
	}

	return nil
}

// transport builds the HTTP transport rfx talks to a service with.
func transport(cfg Config) *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: dialTimeout, KeepAlive: keepAlive}).DialContext,
		MaxIdleConns:          maxIdleConns,
		IdleConnTimeout:       idleConnTimeout,
		TLSHandshakeTimeout:   tlsHandshakeTimeout,
		ExpectContinueTimeout: expectContinueTimeout,
		ForceAttemptHTTP2:     true,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
			// BMCs almost always present a self-signed certificate. Skipping
			// verification is an explicit opt-in, never a default.
			InsecureSkipVerify: cfg.Insecure, //nolint:gosec // Requested by --insecure.
		},
	}
}

// serviceInfo extracts the header metadata from a ServiceRoot.
func serviceInfo(service *gofish.Service) ServiceInfo {
	if service == nil {
		return ServiceInfo{}
	}

	return ServiceInfo{
		RedfishVersion: service.RedfishVersion,
		Vendor:         service.Vendor,
		Product:        service.Product,
		OEMVendors:     oemVendors(service.OEM),
	}
}

// oemVendors lists the vendor keys of an Oem object, ignoring anything that is
// not a JSON object.
func oemVendors(raw json.RawMessage) []string {
	if len(raw) == 0 {
		return nil
	}

	var oem map[string]json.RawMessage

	err := json.Unmarshal(raw, &oem)
	if err != nil {
		return nil
	}

	vendors := make([]string, 0, len(oem))
	for vendor := range oem {
		vendors = append(vendors, vendor)
	}

	return vendors
}

// connectError turns a connection failure into a message that says what to do
// about it. A bare certificate or status-code error is not actionable enough.
func connectError(cfg Config, err error) error {
	switch {
	case isTLSVerificationError(err):
		return fmt.Errorf("%s: TLS certificate could not be verified, pass --insecure to skip verification: %w",
			cfg.Endpoint, err)

	case isMalformedServiceRoot(err):
		return fmt.Errorf("%s does not look like a Redfish service, its %s is not valid Redfish JSON: %w",
			cfg.Endpoint, RootPath, err)

	default:
		return fmt.Errorf("connecting to %s: %w", cfg.Endpoint, err)
	}
}

// isTLSVerificationError reports whether err comes from certificate validation.
func isTLSVerificationError(err error) bool {
	var (
		unknownAuthority   x509.UnknownAuthorityError
		hostname           x509.HostnameError
		certificateInvalid x509.CertificateInvalidError
		verification       *tls.CertificateVerificationError
	)

	return errors.As(err, &unknownAuthority) ||
		errors.As(err, &hostname) ||
		errors.As(err, &certificateInvalid) ||
		errors.As(err, &verification)
}

// isMalformedServiceRoot reports whether err is a JSON decoding failure, which
// at connect time means the endpoint answered with something other than a
// Redfish service root.
func isMalformedServiceRoot(err error) bool {
	var (
		syntax        *json.SyntaxError
		unmarshalType *json.UnmarshalTypeError
	)

	return errors.As(err, &syntax) || errors.As(err, &unmarshalType)
}
