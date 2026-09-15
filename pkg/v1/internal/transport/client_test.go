package transport

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestClient_Scope(t *testing.T) {
	t.Run("UsesProvidedScope", func(t *testing.T) {
		client, err := NewClient(testEndpoint, testTokenID, "", nil)
		require.NoError(t, err)

		require.Equal(t, testEndpoint, client.endpoint.String())
		require.Equal(t, testTokenID, client.tokenID)
		require.NotNil(t, client.httpClient)
		require.NotNil(t, client.uploadClient)
	})

	t.Run("StripsTheVersionSuffix", func(t *testing.T) {
		for _, endpoint := range []string{testEndpoint + "/v2", testEndpoint + "/v2/", testEndpoint + "/"} {
			client, err := NewClient(endpoint, testTokenID, "", nil)
			require.NoError(t, err)
			require.Equal(t, testEndpoint, client.endpoint.String(), endpoint)
		}
	})

	t.Run("DefaultClientsDifferInTimeout", func(t *testing.T) {
		client, err := NewClient(testEndpoint, testTokenID, "", nil)
		require.NoError(t, err)

		standard, ok := client.httpClient.(*http.Client)
		require.True(t, ok)
		require.Equal(t, defaultHTTPTimeout*time.Second, standard.Timeout)

		upload, ok := client.uploadClient.(*http.Client)
		require.True(t, ok)
		require.Zero(t, upload.Timeout, "the upload client must not have an overall timeout")
		require.Same(t, standard.Transport, upload.Transport)
	})

	t.Run("DefaultUserAgent", func(t *testing.T) {
		client, err := NewClient(testEndpoint, testTokenID, "", nil)
		require.NoError(t, err)

		require.Equal(t, appName+"/"+unknownModuleVersion, client.userAgent)
	})

	t.Run("UserAgentPrefix", func(t *testing.T) {
		client, err := NewClient(testEndpoint, testTokenID, "terraform-provider-selectel/v8.0.0", nil)
		require.NoError(t, err)

		require.Equal(t, "terraform-provider-selectel/v8.0.0 "+appName+"/"+unknownModuleVersion, client.userAgent)
	})

	t.Run("StandardClientRefusesRedirectsAndOwnsUploads", func(t *testing.T) {
		httpClient := &http.Client{Timeout: time.Second}

		client, err := NewClient(testEndpoint, testTokenID, "", httpClient)
		require.NoError(t, err)

		copied, ok := client.httpClient.(*http.Client)
		require.True(t, ok)
		require.NotSame(t, httpClient, copied)
		require.Equal(t, time.Second, copied.Timeout)
		require.Nil(t, httpClient.CheckRedirect)
		require.ErrorIs(t, copied.CheckRedirect(nil, nil), http.ErrUseLastResponse)
		require.Same(t, copied, client.uploadClient)
	})

	t.Run("OwnImplementationIsUsedAsIs", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{{status: http.StatusOK, body: "{}"}}}

		client, err := NewClient(testEndpoint, testTokenID, "", httpClient)
		require.NoError(t, err)

		require.Same(t, httpClient, client.httpClient)
		require.Same(t, httpClient, client.uploadClient)
	})

	t.Run("RejectsMissingEndpoint", func(t *testing.T) {
		client, err := NewClient("", testTokenID, "", nil)

		require.Nil(t, client)
		require.True(t, IsKind(err, KindInvalidRequest), "unexpected error: %v", err)
		require.Contains(t, err.Error(), "endpoint")
	})

	t.Run("RejectsInvalidEndpoint", func(t *testing.T) {
		client, err := NewClient("api.example.com/image", testTokenID, "", nil)

		require.Nil(t, client)
		require.True(t, IsKind(err, KindInvalidRequest), "unexpected error: %v", err)
		require.Contains(t, err.Error(), "absolute HTTP or HTTPS URL")
	})

	t.Run("RejectsMissingToken", func(t *testing.T) {
		client, err := NewClient(testEndpoint, "  ", "", nil)

		require.Nil(t, client)
		require.True(t, IsKind(err, KindInvalidRequest), "unexpected error: %v", err)
		require.Contains(t, err.Error(), "token")
	})
}

func TestClient_Request(t *testing.T) {
	t.Run("SendsScopeAndHeaders", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{{status: http.StatusCreated, body: `{"id":"0e1c2b3a"}`}}}
		client := testClient(httpClient, "terraform-provider-selectel/v8.0.0")

		var result map[string]any

		response, err := DoRequest(
			t.Context(), client, http.MethodPost, "v2/images", http.StatusCreated,
			map[string]string{"disk_format": "raw"}, &result,
		)
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, response.StatusCode)
		require.Equal(t, "0e1c2b3a", result["id"])

		request := httpClient.lastRequest(t)
		require.Equal(t, http.MethodPost, request.Method)
		require.Equal(t, testEndpoint+"/v2/images", request.URL.String())
		require.Equal(t, testTokenID, request.Header.Get(headerAuthToken))
		require.Equal(t, client.userAgent, request.Header.Get(headerUserAgent))
		require.Equal(t, contentTypeJSON, request.Header.Get(headerAccept))
		require.Equal(t, contentTypeJSON, request.Header.Get(headerContentType))
		require.JSONEq(t, `{"disk_format":"raw"}`, httpClient.lastBody(t))
	})

	t.Run("SendsJSONPatchContentType", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{{status: http.StatusOK, body: `{"id":"0e1c2b3a"}`}}}
		patchType := "application/openstack-images-v2.1-json-patch"

		var result map[string]any

		_, err := DoRequest(
			t.Context(), testClient(httpClient), http.MethodPatch, "v2/images/0e1c2b3a", http.StatusOK,
			[]map[string]string{{"op": "replace", "path": "/name", "value": "renamed"}}, &result,
			WithContentType(patchType),
		)
		require.NoError(t, err)

		request := httpClient.lastRequest(t)
		require.Equal(t, http.MethodPatch, request.Method)
		require.Equal(t, patchType, request.Header.Get(headerContentType))
		require.Equal(t, contentTypeJSON, request.Header.Get(headerAccept))
		require.JSONEq(t, `[{"op":"replace","path":"/name","value":"renamed"}]`, httpClient.lastBody(t))
	})

	t.Run("AcceptsEmptySuccessBodies", func(t *testing.T) {
		for _, status := range []int{http.StatusAccepted, http.StatusNoContent} {
			httpClient := &testHTTPClient{answers: []testAnswer{{status: status, body: ""}}}

			response, err := DoRequest(
				t.Context(), testClient(httpClient), http.MethodPost, "v2/images/0e1c2b3a/import", status,
				map[string]any{"method": map[string]string{"name": "web-download"}}, nil,
			)
			require.NoError(t, err)
			require.Equal(t, status, response.StatusCode)
		}
	})

	t.Run("SendsQuery", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{{status: http.StatusOK, body: ""}}}

		_, err := DoRequest(
			t.Context(), testClient(httpClient), http.MethodGet, "v2/images", http.StatusOK,
			nil, nil, WithQuery(url.Values{"limit": []string{"100"}}),
		)
		require.NoError(t, err)

		request := httpClient.lastRequest(t)
		require.Equal(t, "limit=100", request.URL.RawQuery)
		require.Empty(t, request.Header.Get(headerContentType))
	})

	t.Run("DecodesResult", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{
			{status: http.StatusOK, body: `{"images": [{"id": "0e1c2b3a", "status": "active"}], "next": "/v2/images?marker=0e1c2b3a"}`},
		}}

		var result struct {
			Images []struct {
				ID     string `json:"id"`
				Status string `json:"status"`
			} `json:"images"`
			Next string `json:"next"`
		}

		response, err := DoRequest(
			t.Context(), testClient(httpClient), http.MethodGet, "v2/images", http.StatusOK,
			nil, &result, WithResponseEnvelope("images"),
		)
		require.NoError(t, err)

		require.Equal(t, http.StatusOK, response.StatusCode)
		require.Len(t, result.Images, 1)
		require.Equal(t, "active", result.Images[0].Status)
		require.Equal(t, "/v2/images?marker=0e1c2b3a", result.Next)
	})

	t.Run("EmptyListEnvelopeIsValid", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{{status: http.StatusOK, body: `{"images": []}`}}}

		var result map[string]any

		_, err := DoRequest(
			t.Context(), testClient(httpClient), http.MethodGet, "v2/images", http.StatusOK,
			nil, &result, WithResponseEnvelope("images"),
		)
		require.NoError(t, err)
	})

	t.Run("UndecodableResult", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{{status: http.StatusOK, body: "not a json body"}}}

		var result map[string]any

		_, err := DoRequest(
			t.Context(), testClient(httpClient), http.MethodGet, "v2/images", http.StatusOK, nil, &result,
		)

		require.Error(t, err)
		require.True(t, IsKind(err, KindUnexpected), "unexpected error: %v", err)
	})

	t.Run("IgnoresUnreadBodyWithoutResult", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{
			{status: http.StatusNoContent, bodyErr: errors.New("connection reset by peer")},
		}}

		result, err := DoRequest(
			t.Context(), testClient(httpClient), http.MethodDelete, "v2/images/0e1c2b3a", http.StatusNoContent,
			nil, nil,
		)

		require.NoError(t, err)
		require.Equal(t, http.StatusNoContent, result.StatusCode)
	})
}

func TestClient_Upload(t *testing.T) {
	t.Run("StreamsBinaryDataWithLength", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{{status: http.StatusNoContent, body: ""}}}
		data := "raw image bytes"

		response, err := DoUpload(
			t.Context(), testClient(httpClient), http.MethodPut, "v2/images/0e1c2b3a/file", http.StatusNoContent,
			strings.NewReader(data), int64(len(data)),
		)
		require.NoError(t, err)
		require.Equal(t, http.StatusNoContent, response.StatusCode)

		request := httpClient.lastRequest(t)
		require.Equal(t, http.MethodPut, request.Method)
		require.Equal(t, testEndpoint+"/v2/images/0e1c2b3a/file", request.URL.String())
		require.Equal(t, int64(len(data)), request.ContentLength)
		require.Equal(t, contentTypeBinary, request.Header.Get(headerContentType))
		require.Empty(t, request.Header.Get(headerAccept), "an upload does not ask for a JSON body")
		require.Equal(t, testTokenID, request.Header.Get(headerAuthToken))
		require.Equal(t, data, httpClient.lastBody(t))
	})

	t.Run("UsesTheUploadClient", func(t *testing.T) {
		jsonClient := &testHTTPClient{answers: []testAnswer{{status: http.StatusOK, body: "{}"}}}
		uploadClient := &testHTTPClient{answers: []testAnswer{{status: http.StatusNoContent, body: ""}}}
		client := testClient(jsonClient)
		client.uploadClient = uploadClient

		_, err := DoUpload(
			t.Context(), client, http.MethodPut, "v2/images/0e1c2b3a/file", http.StatusNoContent,
			strings.NewReader("x"), 1,
		)
		require.NoError(t, err)
		require.Empty(t, jsonClient.requests)
		require.Len(t, uploadClient.requests, 1)
	})

	t.Run("ReportsAPIFailure", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{{status: http.StatusConflict, body: `{"message": "Image status transition from active to saving is not allowed"}`}}}

		_, err := DoUpload(
			t.Context(), testClient(httpClient), http.MethodPut, "v2/images/0e1c2b3a/file", http.StatusNoContent,
			strings.NewReader("x"), 1,
		)
		require.True(t, IsKind(err, KindConflict), "unexpected error: %v", err)
		require.Contains(t, err.Error(), "not allowed")
	})

	t.Run("RejectsMissingData", func(t *testing.T) {
		httpClient := &testHTTPClient{}

		_, err := DoUpload(
			t.Context(), testClient(httpClient), http.MethodPut, "v2/images/0e1c2b3a/file", http.StatusNoContent,
			nil, 0,
		)
		require.True(t, IsKind(err, KindInvalidRequest), "unexpected error: %v", err)
		require.Empty(t, httpClient.requests)
	})
}

func TestClient_ResponseEnvelope(t *testing.T) {
	testCases := []struct {
		name string
		body string
	}{
		{name: "EmptyBody", body: ""},
		{name: "NullBody", body: "null"},
		{name: "EmptyObject", body: "{}"},
		{name: "MissingEnvelope", body: `{"other": []}`},
		{name: "NullEnvelope", body: `{"images": null}`},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			httpClient := &testHTTPClient{answers: []testAnswer{{status: http.StatusOK, body: testCase.body}}}
			var result map[string]any

			response, err := DoRequest(
				t.Context(), testClient(httpClient), http.MethodGet, "v2/images", http.StatusOK,
				nil, &result, WithResponseEnvelope("images"),
			)

			require.Equal(t, http.StatusOK, response.StatusCode)
			require.True(t, IsKind(err, KindUnexpected), "unexpected error: %v", err)
		})
	}
}

func TestClient_URLResolution(t *testing.T) {
	t.Run("RelativeNextPathKeepsTheEndpointPrefix", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{{status: http.StatusOK, body: ""}}}

		_, err := DoRequest(t.Context(), testClient(httpClient), http.MethodGet,
			"/v2/images?marker=0e1c2b3a&limit=2", http.StatusOK, nil, nil)
		require.NoError(t, err)

		request := httpClient.lastRequest(t)
		require.Equal(t, "api.example.com", request.URL.Host)
		require.Equal(t, "/image/v2/images", request.URL.Path)
		require.Equal(t, "marker=0e1c2b3a&limit=2", request.URL.RawQuery)
	})

	t.Run("RelativeNextPathOnARootEndpoint", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{{status: http.StatusOK, body: ""}}}
		client, err := NewClient("https://glance.example.com:9292", testTokenID, "", httpClient)
		require.NoError(t, err)

		_, err = DoRequest(t.Context(), client, http.MethodGet, "/v2/images?marker=0e1c2b3a", http.StatusOK, nil, nil)
		require.NoError(t, err)

		require.Equal(t, "https://glance.example.com:9292/v2/images?marker=0e1c2b3a", httpClient.lastRequest(t).URL.String())
	})

	t.Run("AbsoluteURLKeepsPathAndQuery", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{{status: http.StatusOK, body: ""}}}

		_, err := DoRequest(t.Context(), testClient(httpClient), http.MethodGet,
			testEndpoint+"/v2/images?marker=0e1c2b3a&limit=2", http.StatusOK, nil, nil)
		require.NoError(t, err)

		request := httpClient.lastRequest(t)
		require.Equal(t, "/image/v2/images", request.URL.Path)
		require.Equal(t, "marker=0e1c2b3a&limit=2", request.URL.RawQuery)
	})

	t.Run("ForeignAbsoluteURLStaysOnTheEndpoint", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{{status: http.StatusOK, body: ""}}}

		_, err := DoRequest(t.Context(), testClient(httpClient), http.MethodGet,
			"https://another.example.com/image/v2/images?marker=img-1", http.StatusOK, nil, nil)
		require.NoError(t, err)

		request := httpClient.lastRequest(t)
		require.Equal(t, "api.example.com", request.URL.Host)
		require.Equal(t, "/image/v2/images", request.URL.Path)
		require.Equal(t, "marker=img-1", request.URL.RawQuery)
	})

	t.Run("MergesExtraQuery", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{{status: http.StatusOK, body: ""}}}

		_, err := DoRequest(t.Context(), testClient(httpClient), http.MethodGet,
			"/v2/images?marker=img-1", http.StatusOK, nil, nil, WithQuery(url.Values{"limit": []string{"5"}}))
		require.NoError(t, err)

		require.Equal(t, "limit=5&marker=img-1", httpClient.lastRequest(t).URL.RawQuery)
	})
}

func TestClient_NoHiddenRetry(t *testing.T) {
	t.Run("ServerError", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{{status: http.StatusInternalServerError, body: apiFaultJSON}}}

		_, err := DoRequest(
			t.Context(), testClient(httpClient), http.MethodGet, "v2/images", http.StatusOK, nil, nil,
		)

		require.Error(t, err)
		require.True(t, IsKind(err, KindServerError))
		require.Len(t, httpClient.requests, 1, "the SDK must not retry the request")
	})

	t.Run("TransportFailure", func(t *testing.T) {
		httpClient := &testHTTPClient{err: errors.New("connection reset by peer")}

		_, err := DoRequest(
			t.Context(), testClient(httpClient), http.MethodGet, "v2/images", http.StatusOK, nil, nil,
		)

		require.Error(t, err)
		require.True(t, IsKind(err, KindTransport), "unexpected error: %v", err)
		require.Len(t, httpClient.requests, 1, "the SDK must not retry the request")
	})

	t.Run("RedirectIsNotFollowed", func(t *testing.T) {
		foreign := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusOK)
			_, _ = writer.Write([]byte(`{"id": "planted"}`))
		}))
		defer foreign.Close()

		reached := false
		origin := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			reached = true
			writer.Header().Set("Location", foreign.URL+"/v2/images")
			writer.WriteHeader(http.StatusTemporaryRedirect)
		}))
		defer origin.Close()

		client, clientErr := NewClient(origin.URL, testTokenID, "", origin.Client())
		require.NoError(t, clientErr)

		var result map[string]any

		response, err := DoRequest(
			t.Context(), client, http.MethodPost, "v2/images", http.StatusCreated,
			map[string]string{"disk_format": "raw"}, &result,
		)

		require.Error(t, err)
		require.True(t, IsKind(err, KindUnexpected), "unexpected error: %v", err)
		require.Equal(t, http.StatusTemporaryRedirect, response.StatusCode)
		require.True(t, reached)
		require.Empty(t, result, "the body of the redirect target must not be decoded")
	})
}

func TestClient_ContextEnds(t *testing.T) {
	t.Run("Canceled", func(t *testing.T) {
		httpClient := &testHTTPClient{err: &url.Error{Op: "Post", Err: context.Canceled}}

		_, err := DoRequest(
			t.Context(), testClient(httpClient), http.MethodGet, "v2/images", http.StatusOK, nil, nil,
		)

		require.Error(t, err)
		require.True(t, IsKind(err, KindCanceled), "unexpected error: %v", err)
		require.ErrorIs(t, err, context.Canceled)
	})

	t.Run("TimedOut", func(t *testing.T) {
		httpClient := &testHTTPClient{err: &url.Error{Op: "Get", Err: context.DeadlineExceeded}}

		_, err := DoRequest(
			t.Context(), testClient(httpClient), http.MethodGet, "v2/images", http.StatusOK, nil, nil,
		)

		require.Error(t, err)
		require.True(t, IsKind(err, KindTimeout), "unexpected error: %v", err)
	})

	t.Run("CanceledWhileReadingBody", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{{
			status:  http.StatusOK,
			header:  http.Header{headerRequestID: []string{"req-partial-body"}},
			bodyErr: context.Canceled,
		}}}

		var result map[string]any

		meta, err := DoRequest(
			t.Context(), testClient(httpClient), http.MethodGet, "v2/images", http.StatusOK, nil, &result,
		)

		require.Error(t, err)

		var sdkErr *Error
		require.ErrorAs(t, err, &sdkErr)

		require.Equal(t, KindCanceled, sdkErr.Kind, "unexpected error: %v", err)
		require.Equal(t, http.StatusOK, sdkErr.StatusCode)
		require.Equal(t, "req-partial-body", sdkErr.RequestID)
		require.Contains(t, sdkErr.Message, "unable to read the response body")
		require.Equal(t, http.StatusOK, meta.StatusCode)
	})
}
