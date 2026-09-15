package member

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	v1 "github.com/selectel/images-go/pkg/v1"
	"github.com/stretchr/testify/require"
)

const (
	testEndpoint = "https://api.example.com/image"
	testTokenID  = "fake-project-scoped-token" //nolint:gosec // G101: this is a fake value used by the tests.
	testImageID  = "0e1c2b3a-4d5e-6f70-8192-a3b4c5d6e7f8"
	testMemberID = "8f0a1b2c3d4e5f60718293a4b5c6d7e8"

	memberJSON = `{"member_id": "` + testMemberID + `", "image_id": "` + testImageID + `", "status": "pending",` +
		` "created_at": "2026-09-08T10:00:00Z", "updated_at": "2026-09-08T10:00:00Z", "schema": "/v2/schemas/member"}`
	forbiddenJSON = `{"code": 403, "message": "You are not permitted to modify 'status' on this image member."}`
)

type testAnswer struct {
	status int
	body   string
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

	return &http.Response{StatusCode: answer.status, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(answer.body))}, nil
}

func testClient(t *testing.T, answers ...testAnswer) (*v1.Client, *testHTTPClient) {
	t.Helper()

	httpClient := &testHTTPClient{answers: answers}

	client, err := v1.NewClient(v1.Config{Endpoint: testEndpoint, Token: testTokenID, HTTPClient: httpClient})
	require.NoError(t, err)

	return client, httpClient
}

func requireRequest(t *testing.T, httpClient *testHTTPClient, method, path string) *http.Request {
	t.Helper()

	require.Len(t, httpClient.requests, 1)
	request := httpClient.requests[0]
	require.Equal(t, method, request.Method)
	require.Equal(t, testEndpoint+path, request.URL.String())

	return request
}

func TestCreate(t *testing.T) {
	t.Run("GrantsAccess", func(t *testing.T) {
		client, httpClient := testClient(t, testAnswer{status: http.StatusOK, body: memberJSON})

		view, response, err := Create(t.Context(), client, testImageID, testMemberID)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)
		require.Equal(t, testMemberID, view.MemberID)
		require.Equal(t, testImageID, view.ImageID)
		require.Equal(t, StatusPending, view.Status)
		require.Equal(t, time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC), view.CreatedAt)
		require.Equal(t, "/v2/schemas/member", view.Schema)

		requireRequest(t, httpClient, http.MethodPost, "/v2/images/"+testImageID+"/members")
		require.JSONEq(t, `{"member":"`+testMemberID+`"}`, httpClient.bodies[0])
	})

	t.Run("NotSharedImage", func(t *testing.T) {
		client, _ := testClient(t, testAnswer{status: http.StatusForbidden, body: `{"code": 403, "message": "Only shared images have members."}`})

		view, _, err := Create(t.Context(), client, testImageID, testMemberID)
		require.Nil(t, view)
		require.True(t, v1.IsKind(err, v1.KindForbidden), "unexpected error: %v", err)
		require.Contains(t, err.Error(), "Only shared images")
	})

	t.Run("RequiresIDs", func(t *testing.T) {
		client, httpClient := testClient(t)

		_, _, err := Create(t.Context(), client, testImageID, "")
		require.True(t, v1.IsKind(err, v1.KindInvalidRequest), "unexpected error: %v", err)

		_, _, err = Create(t.Context(), client, "", testMemberID)
		require.True(t, v1.IsKind(err, v1.KindInvalidRequest), "unexpected error: %v", err)
		require.Empty(t, httpClient.requests)
	})
}

func TestGet(t *testing.T) {
	t.Run("DecodesMember", func(t *testing.T) {
		client, httpClient := testClient(t, testAnswer{status: http.StatusOK, body: memberJSON})

		view, _, err := Get(t.Context(), client, testImageID, testMemberID)
		require.NoError(t, err)
		require.Equal(t, StatusPending, view.Status)

		requireRequest(t, httpClient, http.MethodGet, "/v2/images/"+testImageID+"/members/"+testMemberID)
		require.Empty(t, httpClient.bodies[0])
	})

	t.Run("NotFound", func(t *testing.T) {
		client, _ := testClient(t, testAnswer{status: http.StatusNotFound, body: ``})

		view, _, err := Get(t.Context(), client, testImageID, testMemberID)
		require.Nil(t, view)
		require.True(t, v1.IsKind(err, v1.KindNotFound), "unexpected error: %v", err)
	})
}

func TestList(t *testing.T) {
	t.Run("DecodesMembers", func(t *testing.T) {
		client, httpClient := testClient(t, testAnswer{status: http.StatusOK, body: `{"members": [` + memberJSON + `,` +
			`{"member_id": "other", "image_id": "` + testImageID + `", "status": "accepted", "created_at": "2026-09-08T10:00:00Z", "updated_at": "2026-09-08T10:00:00Z", "schema": "/v2/schemas/member"}],` +
			` "schema": "/v2/schemas/members"}`})

		views, response, err := List(t.Context(), client, testImageID)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)
		require.Len(t, views, 2)
		require.Equal(t, testMemberID, views[0].MemberID)
		require.Equal(t, StatusAccepted, views[1].Status)

		requireRequest(t, httpClient, http.MethodGet, "/v2/images/"+testImageID+"/members")
	})

	t.Run("EmptyList", func(t *testing.T) {
		client, _ := testClient(t, testAnswer{status: http.StatusOK, body: `{"members": [], "schema": "/v2/schemas/members"}`})

		views, _, err := List(t.Context(), client, testImageID)
		require.NoError(t, err)
		require.Empty(t, views)
	})

	t.Run("MissingEnvelope", func(t *testing.T) {
		client, _ := testClient(t, testAnswer{status: http.StatusOK, body: `{"schema": "/v2/schemas/members"}`})

		_, _, err := List(t.Context(), client, testImageID)
		require.True(t, v1.IsKind(err, v1.KindUnexpected), "unexpected error: %v", err)
	})

	t.Run("Forbidden", func(t *testing.T) {
		client, _ := testClient(t, testAnswer{status: http.StatusForbidden, body: forbiddenJSON})

		views, _, err := List(t.Context(), client, testImageID)
		require.Nil(t, views)
		require.True(t, v1.IsKind(err, v1.KindForbidden), "unexpected error: %v", err)
	})
}

func TestUpdate(t *testing.T) {
	t.Run("AcceptsMembership", func(t *testing.T) {
		client, httpClient := testClient(t, testAnswer{status: http.StatusOK, body: strings.Replace(memberJSON, "pending", "accepted", 1)})

		view, _, err := Update(t.Context(), client, testImageID, testMemberID, StatusAccepted)
		require.NoError(t, err)
		require.Equal(t, StatusAccepted, view.Status)

		requireRequest(t, httpClient, http.MethodPut, "/v2/images/"+testImageID+"/members/"+testMemberID)
		require.JSONEq(t, `{"status":"accepted"}`, httpClient.bodies[0])
	})

	t.Run("OwnerCannotChangeStatus", func(t *testing.T) {
		client, _ := testClient(t, testAnswer{status: http.StatusForbidden, body: forbiddenJSON})

		_, _, err := Update(t.Context(), client, testImageID, testMemberID, StatusAccepted)
		require.True(t, v1.IsKind(err, v1.KindForbidden), "unexpected error: %v", err)
		require.Contains(t, err.Error(), "not permitted to modify 'status'")
	})

	t.Run("RequiresStatus", func(t *testing.T) {
		client, httpClient := testClient(t)

		_, _, err := Update(t.Context(), client, testImageID, testMemberID, "")
		require.True(t, v1.IsKind(err, v1.KindInvalidRequest), "unexpected error: %v", err)
		require.Empty(t, httpClient.requests)
	})
}

func TestDelete(t *testing.T) {
	t.Run("RevokesAccess", func(t *testing.T) {
		client, httpClient := testClient(t, testAnswer{status: http.StatusNoContent})

		response, err := Delete(t.Context(), client, testImageID, testMemberID)
		require.NoError(t, err)
		require.Equal(t, http.StatusNoContent, response.StatusCode)

		requireRequest(t, httpClient, http.MethodDelete, "/v2/images/"+testImageID+"/members/"+testMemberID)
	})

	t.Run("NotFound", func(t *testing.T) {
		client, _ := testClient(t, testAnswer{status: http.StatusNotFound, body: ``})

		_, err := Delete(t.Context(), client, testImageID, testMemberID)
		require.True(t, v1.IsKind(err, v1.KindNotFound), "unexpected error: %v", err)
	})

	t.Run("MemberCannotDelete", func(t *testing.T) {
		client, _ := testClient(t, testAnswer{status: http.StatusForbidden, body: `{"code": 403, "message": "You cannot delete image member."}`})

		_, err := Delete(t.Context(), client, testImageID, testMemberID)
		require.True(t, v1.IsKind(err, v1.KindForbidden), "unexpected error: %v", err)
	})
}
