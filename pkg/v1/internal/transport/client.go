package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	defaultHTTPTimeout           = 120
	defaultDialTimeout           = 60
	defaultKeepaliveTimeout      = 60
	defaultMaxIdleConns          = 100
	defaultIdleConnTimeout       = 100
	defaultTLSHandshakeTimeout   = 60
	defaultExpectContinueTimeout = 1
)

const (
	headerAuthToken     = "X-Auth-Token" //nolint:gosec // G101: this is an HTTP header name, not a credential.
	headerUserAgent     = "User-Agent"
	headerAccept        = "Accept"
	headerContentType   = "Content-Type"
	contentTypeJSON     = "application/json"
	contentTypeBinary   = "application/octet-stream"
	versionPathSegment  = "/v2"
	absoluteHTTPPrefix  = "http://"
	absoluteHTTPSPrefix = "https://"
)

type HTTPClient interface {
	Do(request *http.Request) (*http.Response, error)
}

// Client sends JSON requests through httpClient and image data uploads through uploadClient.
// The default upload client has no overall timeout because an upload may legitimately take longer
// than any fixed limit; the caller bounds it with the request context.
type Client struct {
	httpClient   HTTPClient
	uploadClient HTTPClient
	tokenID      string
	endpoint     *url.URL
	userAgent    string
}

func NewClient(endpoint, tokenID, userAgent string, httpClient HTTPClient) (*Client, error) {
	if strings.TrimSpace(endpoint) == "" {
		return nil, &Error{Kind: KindInvalidRequest, Message: "the client endpoint is required"}
	}

	parsedEndpoint, err := url.Parse(endpoint)
	if err != nil {
		return nil, &Error{Kind: KindInvalidRequest, Message: "unable to parse the client endpoint", Err: err}
	}

	if (parsedEndpoint.Scheme != "http" && parsedEndpoint.Scheme != "https") || parsedEndpoint.Host == "" {
		return nil, &Error{
			Kind:    KindInvalidRequest,
			Message: "the client endpoint must be an absolute HTTP or HTTPS URL",
		}
	}

	if strings.TrimSpace(tokenID) == "" {
		return nil, &Error{Kind: KindInvalidRequest, Message: "the project-scoped token is required"}
	}

	parsedEndpoint.Path = strings.TrimSuffix(strings.TrimSuffix(parsedEndpoint.Path, "/"), versionPathSegment)
	parsedEndpoint.RawPath = ""

	client := &Client{
		endpoint:  parsedEndpoint,
		tokenID:   tokenID,
		userAgent: buildUserAgent(userAgent),
	}

	switch standard := httpClient.(type) {
	case nil:
		transport := newHTTPTransport()
		client.httpClient = &http.Client{Timeout: defaultHTTPTimeout * time.Second, Transport: transport}
		client.uploadClient = &http.Client{Transport: transport}
	case *http.Client:
		// A custom standard client owns its timeout, for uploads as well.
		client.httpClient = withoutRedirects(standard)
		client.uploadClient = client.httpClient
	default:
		// A custom implementation owns its redirect and timeout policy.
		client.httpClient = httpClient
		client.uploadClient = httpClient
	}

	return client, nil
}

// withoutRedirects copies the client and prevents credentials from being forwarded by redirects.
func withoutRedirects(httpClient *http.Client) *http.Client {
	copied := *httpClient
	copied.CheckRedirect = func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}

	return &copied
}

type Response struct {
	StatusCode int
	RequestID  string
}

// DoRequest sends a JSON request and decodes a JSON result when one is expected.
func DoRequest(
	ctx context.Context,
	client *Client,
	method, path string, expectedStatus int,
	requestBody, result any,
	options ...RequestOption,
) (*Response, error) {
	requestOptions := newRequestOptions(options)

	var bodyReader io.Reader

	if requestBody != nil {
		encoded, marshalErr := json.Marshal(requestBody)
		if marshalErr != nil {
			return nil, &Error{
				Kind:    KindInvalidRequest,
				Message: "unable to encode the request body",
				Err:     marshalErr,
			}
		}

		bodyReader = bytes.NewReader(encoded)
	}

	request, err := client.newRequest(ctx, method, path, bodyReader, requestOptions)
	if err != nil {
		return nil, err
	}

	request.Header.Set(headerAccept, contentTypeJSON)

	if bodyReader != nil {
		contentType := contentTypeJSON
		if requestOptions.contentType != "" {
			contentType = requestOptions.contentType
		}

		request.Header.Set(headerContentType, contentType)
	}

	return client.do(ctx, client.httpClient, request, result, expectedStatus, requestOptions)
}

// DoUpload streams binary image data with an explicit Content-Length. The upload client has no
// overall timeout; the caller bounds the upload with ctx.
func DoUpload(
	ctx context.Context,
	client *Client,
	method, path string, expectedStatus int,
	data io.Reader, size int64,
	options ...RequestOption,
) (*Response, error) {
	if data == nil || size < 0 {
		return nil, &Error{Kind: KindInvalidRequest, Message: "the upload data and its size are required"}
	}

	requestOptions := newRequestOptions(options)

	request, err := client.newRequest(ctx, method, path, data, requestOptions)
	if err != nil {
		return nil, err
	}

	request.ContentLength = size
	request.Header.Set(headerContentType, contentTypeBinary)

	return client.do(ctx, client.uploadClient, request, nil, expectedStatus, requestOptions)
}

func (client *Client) do(
	ctx context.Context,
	httpClient HTTPClient,
	request *http.Request,
	result any,
	expectedStatus int,
	options *requestOptions,
) (*Response, error) {
	response, err := httpClient.Do(request)
	if err != nil {
		return nil, newTransportError(ctx, err)
	}
	defer response.Body.Close()

	return handleResponse(ctx, response, result, expectedStatus, options)
}

func (client *Client) newRequest(
	ctx context.Context, method, path string, body io.Reader, options *requestOptions,
) (*http.Request, error) {
	requestURL, err := client.resolveURL(path, options.query)
	if err != nil {
		return nil, err
	}

	request, err := http.NewRequestWithContext(ctx, method, requestURL, body)
	if err != nil {
		return nil, &Error{Kind: KindInvalidRequest, Message: "unable to build the request", Err: err}
	}

	request.Header.Set(headerUserAgent, client.userAgent)
	request.Header.Set(headerAuthToken, client.tokenID)

	return request, nil
}

// resolveURL builds the request URL from an operation path such as v2/images, from the relative
// path that Glance returns in the next field of a listing, such as /v2/images?marker=..., or from
// an absolute URL. Every form is anchored to the configured endpoint, so a bad link cannot send the
// token to another host and a proxy prefix in the endpoint path is preserved.
func (client *Client) resolveURL(path string, query url.Values) (string, error) {
	parsed, err := url.Parse(path)
	if err != nil {
		return "", &Error{Kind: KindInvalidRequest, Message: "unable to parse the request URL", Err: err}
	}

	if parsed.Path == "" {
		return "", &Error{Kind: KindInvalidRequest, Message: "the request URL has no path"}
	}

	target := &url.URL{Scheme: client.endpoint.Scheme, Host: client.endpoint.Host, RawQuery: parsed.RawQuery}

	switch {
	case strings.HasPrefix(path, absoluteHTTPPrefix) || strings.HasPrefix(path, absoluteHTTPSPrefix):
		target.Path = parsed.Path
	case strings.HasPrefix(parsed.Path, "/"):
		target.Path = client.endpoint.Path + parsed.Path
	default:
		target.Path = client.endpoint.JoinPath(parsed.Path).Path
	}

	if len(query) != 0 {
		target.RawQuery = mergeQuery(target.Query(), query).Encode()
	}

	return target.String(), nil
}

func mergeQuery(target, extra url.Values) url.Values {
	for key, values := range extra {
		for _, value := range values {
			target.Add(key, value)
		}
	}

	return target
}

func handleResponse(
	ctx context.Context, response *http.Response, result any, expectedStatus int, options *requestOptions,
) (*Response, error) {
	meta := &Response{
		StatusCode: response.StatusCode,
		RequestID:  extractRequestID(response.Header),
	}

	if response.StatusCode != expectedStatus {
		return meta, errorFromResponse(meta, response.Body)
	}

	if result == nil {
		// Ignore drain errors after the API has accepted the operation.
		_, _ = io.Copy(io.Discard, response.Body)

		return meta, nil
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		return meta, newBodyReadError(ctx, meta, err)
	}

	if len(bytes.TrimSpace(body)) == 0 {
		return meta, unexpectedResponse(meta, "the response body is empty", nil)
	}

	if err := json.Unmarshal(body, result); err != nil {
		return meta, unexpectedResponse(meta, "unable to decode the response body", err)
	}

	if options.responseEnvelope != "" {
		if err := validateResponseEnvelope(body, options.responseEnvelope); err != nil {
			return meta, unexpectedResponse(meta, err.Error(), nil)
		}
	}

	return meta, nil
}

func validateResponseEnvelope(body []byte, name string) error {
	var envelope map[string]json.RawMessage
	if err := json.Unmarshal(body, &envelope); err != nil {
		return err
	}

	raw, ok := envelope[name]
	trimmed := bytes.TrimSpace(raw)
	if !ok || len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return &responseEnvelopeError{name: name}
	}

	return nil
}

type responseEnvelopeError struct {
	name string
}

func (e *responseEnvelopeError) Error() string {
	return "the response body does not contain a " + e.name + " field"
}

func unexpectedResponse(meta *Response, message string, err error) *Error {
	return &Error{
		Kind:       KindUnexpected,
		StatusCode: meta.StatusCode,
		RequestID:  meta.RequestID,
		Message:    message,
		Err:        err,
	}
}

func newHTTPTransport() *http.Transport {
	return &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   defaultDialTimeout * time.Second,
			KeepAlive: defaultKeepaliveTimeout * time.Second,
		}).DialContext,
		MaxIdleConns:          defaultMaxIdleConns,
		IdleConnTimeout:       defaultIdleConnTimeout * time.Second,
		TLSHandshakeTimeout:   defaultTLSHandshakeTimeout * time.Second,
		ExpectContinueTimeout: defaultExpectContinueTimeout * time.Second,
	}
}
