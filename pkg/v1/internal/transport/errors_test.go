package transport

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestError_Reporting(t *testing.T) {
	t.Run("KeepsDiagnosticDetails", func(t *testing.T) {
		err := &Error{
			Kind:       KindForbidden,
			StatusCode: http.StatusForbidden,
			RequestID:  "req-0a1b2c3d",
			Message:    "policy does not allow this operation",
		}

		message := err.Error()

		require.True(t, strings.HasPrefix(message, "images: "))
		require.Contains(t, message, string(KindForbidden))
		require.Contains(t, message, "403")
		require.Contains(t, message, "req-0a1b2c3d")
		require.Contains(t, message, "policy does not allow this operation")
	})

	t.Run("UnwrapsCause", func(t *testing.T) {
		cause := errors.New("connection reset by peer")

		require.ErrorIs(t, &Error{Kind: KindTransport, Err: cause}, cause)
	})

	t.Run("IsKind", func(t *testing.T) {
		err := &Error{Kind: KindNotFound, StatusCode: http.StatusNotFound}

		require.True(t, IsKind(err, KindNotFound))
		require.False(t, IsKind(err, KindForbidden))
		require.False(t, IsKind(errors.New("plain error"), KindNotFound))
	})

	t.Run("IsKindWalksTheChain", func(t *testing.T) {
		err := &Error{Kind: KindIncompleteList, Err: &Error{Kind: KindForbidden}}

		require.True(t, IsKind(err, KindIncompleteList))
		require.True(t, IsKind(err, KindForbidden))
		require.False(t, IsKind(err, KindNotFound))
	})
}

func TestError_ClassesOfAPIResponses(t *testing.T) {
	cases := []struct {
		name       string
		statusCode int
		body       string
		expected   Kind
	}{
		{name: "InvalidRequest", statusCode: http.StatusBadRequest, body: `{"code": 400, "message": "Invalid disk format."}`, expected: KindInvalidRequest},
		{name: "UnsupportedMediaType", statusCode: http.StatusUnsupportedMediaType, body: ``, expected: KindInvalidRequest},
		{name: "NotAcceptable", statusCode: http.StatusNotAcceptable, body: ``, expected: KindInvalidRequest},
		{name: "RejectedToken", statusCode: http.StatusUnauthorized, body: `{"error": {"message": "The request you have made requires authentication."}}`, expected: KindForbidden},
		{name: "ForbiddenOperation", statusCode: http.StatusForbidden, body: apiForbiddenJSON, expected: KindForbidden},
		{name: "MissingImage", statusCode: http.StatusNotFound, body: apiNotFoundHTML, expected: KindNotFound},
		{name: "StateConflict", statusCode: http.StatusConflict, body: `{"message": "Image status transition from active to saving is not allowed"}`, expected: KindConflict},
		{name: "ExhaustedStorageQuota", statusCode: http.StatusRequestEntityTooLarge, body: `<html><body>413 Request Entity Too Large: Image storage media is full</body></html>`, expected: KindOverQuota},
		{name: "ThrottledByAProxy", statusCode: http.StatusTooManyRequests, body: `<html><body>429 Too Many Requests</body></html>`, expected: KindRateLimited},
		{name: "ServerError", statusCode: http.StatusInternalServerError, body: apiFaultJSON, expected: KindServerError},
		{name: "InsufficientStorage", statusCode: http.StatusInsufficientStorage, body: ``, expected: KindServerError},
		{name: "UnavailableService", statusCode: http.StatusServiceUnavailable, body: ``, expected: KindServerError},
		{name: "UnexpectedResponse", statusCode: http.StatusTeapot, body: `{"teapot": {"message": "I am a teapot."}}`, expected: KindUnexpected},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			httpClient := &testHTTPClient{answers: []testAnswer{{
				status: testCase.statusCode,
				body:   testCase.body,
				header: http.Header{headerRequestID: []string{"req-" + testCase.name}},
			}}}

			meta, err := DoRequest(
				t.Context(), testClient(httpClient), http.MethodGet, "v2/images", http.StatusOK, nil, nil,
			)

			require.Error(t, err)

			var sdkErr *Error
			require.ErrorAs(t, err, &sdkErr)

			require.Equal(t, testCase.expected, sdkErr.Kind)
			require.Equal(t, testCase.statusCode, sdkErr.StatusCode)
			require.Equal(t, "req-"+testCase.name, sdkErr.RequestID)
			require.Equal(t, testCase.statusCode, meta.StatusCode)
		})
	}
}

func TestError_DiagnosticMessage(t *testing.T) {
	t.Run("FromTopLevelFault", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{{
			status: http.StatusForbidden,
			body:   apiForbiddenJSON,
		}}}

		_, err := DoRequest(
			t.Context(), testClient(httpClient), http.MethodPut, "v2/images/0e1c2b3a/members/m", http.StatusOK, nil, nil,
		)

		var sdkErr *Error
		require.ErrorAs(t, err, &sdkErr)

		require.Equal(t, "You are not permitted to modify 'status' on this image member.", sdkErr.Message)
		require.True(t, sdkErr.StructuredFault)
	})

	t.Run("FromWrappedFault", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{{
			status: http.StatusInternalServerError,
			body:   apiFaultJSON,
		}}}

		_, err := DoRequest(
			t.Context(), testClient(httpClient), http.MethodGet, "v2/images", http.StatusOK, nil, nil,
		)

		var sdkErr *Error
		require.ErrorAs(t, err, &sdkErr)

		require.Equal(t, "The server has either erred or is incapable.", sdkErr.Message)
		require.True(t, sdkErr.StructuredFault)
	})

	t.Run("FromHTMLBodyWithoutTags", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{
			{status: http.StatusNotFound, body: apiNotFoundHTML},
		}}

		_, err := DoRequest(
			t.Context(), testClient(httpClient), http.MethodGet, "v2/images/0e1c2b3a", http.StatusOK, nil, nil,
		)

		var sdkErr *Error
		require.ErrorAs(t, err, &sdkErr)

		require.Equal(t, "404 Not Found No image found with ID 0e1c2b3a", sdkErr.Message)
		require.Empty(t, sdkErr.RequestID)
		require.False(t, sdkErr.StructuredFault)
	})

	t.Run("IsTruncated", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{
			{status: http.StatusInternalServerError, body: strings.Repeat("a", 4096)},
		}}

		_, err := DoRequest(
			t.Context(), testClient(httpClient), http.MethodGet, "v2/images", http.StatusOK, nil, nil,
		)

		var sdkErr *Error
		require.ErrorAs(t, err, &sdkErr)

		require.Len(t, sdkErr.Message, maxDiagnosticMessageLength)
		require.True(t, strings.HasSuffix(sdkErr.Message, diagnosticTruncationMarker))
	})

	t.Run("OfEmptyBody", func(t *testing.T) {
		httpClient := &testHTTPClient{answers: []testAnswer{{status: http.StatusForbidden, body: ""}}}

		_, err := DoRequest(
			t.Context(), testClient(httpClient), http.MethodGet, "v2/images", http.StatusOK, nil, nil,
		)

		var sdkErr *Error
		require.ErrorAs(t, err, &sdkErr)

		require.True(t, IsKind(sdkErr, KindForbidden))
		require.Empty(t, sdkErr.Message)
		require.False(t, sdkErr.StructuredFault)
	})
}
