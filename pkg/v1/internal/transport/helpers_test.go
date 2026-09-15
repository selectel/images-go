package transport

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const (
	testTokenID  = "fake-project-scoped-token" //nolint:gosec // G101: this is a fake value used by the tests.
	testEndpoint = "https://api.example.com/image"

	apiNotFoundHTML  = `<html><head><title>404 Not Found</title></head><body>No image found with ID 0e1c2b3a</body></html>`
	apiForbiddenJSON = `{"code": 403, "message": "You are not permitted to modify 'status' on this image member."}`
	apiFaultJSON     = `{"computeFault": {"code": 500, "message": "The server has either erred or is incapable."}}`
)

type testAnswer struct {
	status  int
	body    string
	header  http.Header
	bodyErr error
}

type testHTTPClient struct {
	requests []*http.Request
	bodies   []string
	answers  []testAnswer
	err      error
}

func (client *testHTTPClient) Do(request *http.Request) (*http.Response, error) {
	body := ""
	if request.Body != nil {
		raw, _ := io.ReadAll(request.Body)
		body = string(raw)
	}

	client.requests = append(client.requests, request)
	client.bodies = append(client.bodies, body)

	if client.err != nil {
		return nil, client.err
	}

	answer := client.answers[0]
	if len(client.answers) > 1 {
		client.answers = client.answers[1:]
	}

	header := answer.header
	if header == nil {
		header = http.Header{}
	}

	var reader io.Reader = strings.NewReader(answer.body)
	if answer.bodyErr != nil {
		reader = errorReader{err: answer.bodyErr}
	}

	return &http.Response{StatusCode: answer.status, Header: header, Body: io.NopCloser(reader)}, nil
}

type errorReader struct {
	err error
}

func (r errorReader) Read([]byte) (int, error) {
	return 0, r.err
}

func (client *testHTTPClient) lastRequest(t *testing.T) *http.Request {
	t.Helper()

	require.NotEmpty(t, client.requests)

	return client.requests[len(client.requests)-1]
}

func (client *testHTTPClient) lastBody(t *testing.T) string {
	t.Helper()

	require.NotEmpty(t, client.bodies)

	return client.bodies[len(client.bodies)-1]
}

func testClient(httpClient HTTPClient, userAgent ...string) *Client {
	prefix := ""
	if len(userAgent) != 0 {
		prefix = userAgent[0]
	}

	client, err := NewClient(testEndpoint, testTokenID, prefix, httpClient)
	if err != nil {
		panic(err)
	}

	return client
}
