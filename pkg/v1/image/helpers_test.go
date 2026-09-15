package image

import (
	"io"
	"net/http"
	"strings"
	"testing"

	v1 "github.com/selectel/images-go/pkg/v1"
	"github.com/stretchr/testify/require"
)

const (
	testEndpoint = "https://api.example.com/image"
	testTokenID  = "fake-project-scoped-token" //nolint:gosec // G101: this is a fake value used by the tests.
	testImageID  = "0e1c2b3a-4d5e-6f70-8192-a3b4c5d6e7f8"
	testName     = "ubuntu"
	testTag      = "web"
	testBare     = "bare"

	imageJSON = `{
		"id": "` + testImageID + `", "name": "` + testName + `", "status": "active", "visibility": "shared",
		"protected": true, "os_hidden": false, "checksum": "d41d8cd98f00b204e9800998ecf8427e",
		"os_hash_algo": "sha512", "os_hash_value": "cf83e1", "owner": "7f3a", "size": 1024,
		"virtual_size": 4096, "container_format": "` + testBare + `", "disk_format": "qcow2", "min_disk": 1,
		"min_ram": 512, "created_at": "2026-09-08T10:00:00Z", "updated_at": "2026-09-08T11:30:00Z",
		"tags": ["` + testTag + `", "lts"], "file": "/v2/images/` + testImageID + `/file",
		"self": "/v2/images/` + testImageID + `", "schema": "/v2/schemas/image",
		"direct_url": "rbd://pool/img", "stores": "rbd", "locations": [{"url": "rbd://pool/img"}],
		"os_distro": "` + testName + `", "hw_qemu_guest_agent": "yes", "os_glance_importing_to_stores": "",
		"weird_number": 42
	}`
)

type testAnswer struct {
	status int
	body   string
	header http.Header
}

type testHTTPClient struct {
	answers  []testAnswer
	requests []*http.Request
	bodies   []string
}

func (client *testHTTPClient) Do(request *http.Request) (*http.Response, error) {
	body := ""
	if request.Body != nil {
		raw, _ := io.ReadAll(request.Body)
		body = string(raw)
	}

	client.requests = append(client.requests, request)
	client.bodies = append(client.bodies, body)

	answer := client.answers[0]
	if len(client.answers) > 1 {
		client.answers = client.answers[1:]
	}

	header := answer.header
	if header == nil {
		header = http.Header{}
	}

	return &http.Response{StatusCode: answer.status, Header: header, Body: io.NopCloser(strings.NewReader(answer.body))}, nil
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

func testClient(t *testing.T, answers ...testAnswer) (*v1.Client, *testHTTPClient) {
	t.Helper()

	httpClient := &testHTTPClient{answers: answers}

	client, err := v1.NewClient(v1.Config{Endpoint: testEndpoint, Token: testTokenID, HTTPClient: httpClient})
	require.NoError(t, err)

	return client, httpClient
}

func boolPtr(value bool) *bool { return &value }
