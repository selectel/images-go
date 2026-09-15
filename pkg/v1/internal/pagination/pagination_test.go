package pagination

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	v1 "github.com/selectel/images-go/pkg/v1"
	"github.com/stretchr/testify/require"
)

const (
	testEndpoint = "https://api.example.com/image"
	testTokenID  = "fake-project-scoped-token" //nolint:gosec // G101: this is a fake value used by the tests.
)

type testItem struct {
	ID string `json:"id"`
}

type testPage struct {
	Values []testItem `json:"images"`
	Next   string     `json:"next"`
}

func (p *testPage) Items() []testItem { return p.Values }

func (p *testPage) NextHref() string { return p.Next }

type testAnswer struct {
	status int
	body   string
}

type testHTTPClient struct {
	answers  []testAnswer
	requests []*http.Request
}

func (client *testHTTPClient) Do(request *http.Request) (*http.Response, error) {
	client.requests = append(client.requests, request)

	answer := client.answers[0]
	client.answers = client.answers[1:]

	return &http.Response{
		StatusCode: answer.status,
		Header:     http.Header{},
		Body:       io.NopCloser(strings.NewReader(answer.body)),
	}, nil
}

func TestReadAll(t *testing.T) {
	t.Run("CombinesQueryAndLimit", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{{status: http.StatusOK, body: `{"images":[]}`}}}

		items, err := ReadAll[testItem](
			t.Context(), testClient(t, httpClient), "v2/images", "images",
			url.Values{"status": []string{"active"}}, 5, newTestPage,
		)
		require.NoError(t, err)
		require.Empty(t, items)
		require.Len(t, httpClient.requests, 1)
		require.Equal(t, "/image/v2/images", httpClient.requests[0].URL.Path)
		require.Equal(t, "limit=5&status=active", httpClient.requests[0].URL.RawQuery)
	})

	t.Run("FollowsRelativeNextPathsOnTheConfiguredEndpoint", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{
			{status: http.StatusOK, body: pageBody("img-1", "/v2/images?limit=1&marker=img-1")},
			{status: http.StatusOK, body: pageBody("img-2", "/v2/images?limit=1&marker=img-2")},
			{status: http.StatusOK, body: `{"images":[]}`},
		}}

		items, err := ReadAll[testItem](t.Context(), testClient(t, httpClient), "v2/images", "images", nil, 1, newTestPage)
		require.NoError(t, err)
		require.Equal(t, []testItem{{ID: "img-1"}, {ID: "img-2"}}, items)
		require.Len(t, httpClient.requests, 3)

		require.Equal(t, "limit=1", httpClient.requests[0].URL.RawQuery)

		for _, request := range httpClient.requests {
			require.Equal(t, "api.example.com", request.URL.Host)
			require.Equal(t, "/image/v2/images", request.URL.Path)
		}

		require.Equal(t, "limit=1&marker=img-1", httpClient.requests[1].URL.RawQuery)
		require.Equal(t, "limit=1&marker=img-2", httpClient.requests[2].URL.RawQuery)
	})

	t.Run("ReportsAnIncompleteListWithTheCause", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{
			{status: http.StatusOK, body: pageBody("img-1", "/v2/images?limit=1&marker=img-1")},
			{status: http.StatusForbidden, body: `{"code": 403, "message": "Policy does not allow this."}`},
		}}

		items, err := ReadAll[testItem](t.Context(), testClient(t, httpClient), "v2/images", "images", nil, 1, newTestPage)

		require.Nil(t, items, "a partial list must not be returned")
		require.True(t, v1.IsKind(err, v1.KindIncompleteList), "unexpected error: %v", err)
		require.True(t, v1.IsKind(err, v1.KindForbidden), "unexpected error: %v", err)
		require.Len(t, httpClient.requests, 2)
	})

	t.Run("RefusesARepeatedPage", func(t *testing.T) {
		page := pageBody("img-1", "/v2/images?limit=1&marker=img-1")
		httpClient := &testHTTPClient{answers: []testAnswer{
			{status: http.StatusOK, body: page},
			{status: http.StatusOK, body: page},
		}}

		items, err := ReadAll[testItem](t.Context(), testClient(t, httpClient), "v2/images", "images", nil, 1, newTestPage)

		require.Nil(t, items)
		require.True(t, v1.IsKind(err, v1.KindIncompleteList), "unexpected error: %v", err)
		require.Len(t, httpClient.requests, 2)
	})
}

func testClient(t *testing.T, httpClient *testHTTPClient) *v1.Client {
	t.Helper()

	client, err := v1.NewClient(v1.Config{
		Endpoint:   testEndpoint,
		Token:      testTokenID,
		HTTPClient: httpClient,
	})
	require.NoError(t, err)

	return client
}

func newTestPage() *testPage { return &testPage{} }

func pageBody(id, next string) string {
	return `{"images":[{"id":"` + id + `"}],"next":"` + next + `"}`
}
