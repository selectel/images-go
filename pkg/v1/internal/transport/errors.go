package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
)

type Kind string

const (
	KindNotFound Kind = "not_found"

	KindForbidden Kind = "forbidden"

	KindConflict Kind = "conflict"

	KindInvalidRequest Kind = "invalid_request"

	KindOverQuota Kind = "over_quota"

	KindRateLimited Kind = "rate_limited"

	KindServerError Kind = "server_error"

	KindTransport Kind = "transport"

	KindTimeout Kind = "timeout"

	KindCanceled Kind = "canceled"

	KindIncompleteList Kind = "incomplete_list"

	KindUnexpected Kind = "unexpected"
)

const (
	headerRequestID            = "X-Openstack-Request-Id"
	maxDiagnosticMessageLength = 1024
	maxErrorResponseBodyLength = 1024 * 1024
	diagnosticTruncationMarker = "..."
)

var htmlTagPattern = regexp.MustCompile(`<[^>]*>`)

type Error struct {
	Kind    Kind
	Message string

	// StatusCode and RequestID are set when the failure produced a response.
	StatusCode int
	RequestID  string

	// StructuredFault reports whether the response body contained a structured API fault.
	StructuredFault bool

	// Err is the underlying error, if the failure was caused by one.
	Err error
}

func (e *Error) Error() string {
	details := string(e.Kind)

	if e.StatusCode != 0 {
		details = fmt.Sprintf("%s: status %d", details, e.StatusCode)
	}

	if e.RequestID != "" {
		details = fmt.Sprintf("%s: request id %s", details, e.RequestID)
	}

	if e.Message != "" {
		details = fmt.Sprintf("%s: %s", details, e.Message)
	}

	return "images: " + details
}

func (e *Error) Unwrap() error {
	return e.Err
}

func IsKind(err error, kind Kind) bool {
	for err != nil {
		var sdkErr *Error
		if !errors.As(err, &sdkErr) {
			return false
		}

		if sdkErr.Kind == kind {
			return true
		}

		err = sdkErr.Err
	}

	return false
}

func newTransportError(ctx context.Context, err error) *Error {
	switch {
	case errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled):
		return &Error{Kind: KindCanceled, Message: "the request was canceled by the caller", Err: err}
	case errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) || isTimeout(err):
		return &Error{Kind: KindTimeout, Message: "the request timed out", Err: err}
	default:
		return &Error{Kind: KindTransport, Message: "the request has not reached the API", Err: err}
	}
}

func newBodyReadError(ctx context.Context, meta *Response, err error) *Error {
	sdkErr := newTransportError(ctx, err)
	sdkErr.StatusCode = meta.StatusCode
	sdkErr.RequestID = meta.RequestID
	sdkErr.Message = "unable to read the response body: " + sdkErr.Message

	return sdkErr
}

func isTimeout(err error) bool {
	var netErr net.Error

	return errors.As(err, &netErr) && netErr.Timeout()
}

func errorFromResponse(meta *Response, body io.Reader) *Error {
	raw, _ := io.ReadAll(io.LimitReader(body, maxErrorResponseBodyLength))
	fault := parseFault(raw)

	return &Error{
		Kind:            kindOfStatus(meta.StatusCode),
		StatusCode:      meta.StatusCode,
		RequestID:       meta.RequestID,
		StructuredFault: fault != nil,
		Message:         diagnosticMessage(raw, fault),
	}
}

// Glance reports an exhausted image storage quota with 413 and a full backend store with 507.
func kindOfStatus(statusCode int) Kind {
	switch statusCode {
	case http.StatusBadRequest, http.StatusMethodNotAllowed, http.StatusNotAcceptable,
		http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity:
		return KindInvalidRequest
	case http.StatusUnauthorized, http.StatusForbidden:
		return KindForbidden
	case http.StatusNotFound:
		return KindNotFound
	case http.StatusConflict:
		return KindConflict
	case http.StatusRequestEntityTooLarge:
		return KindOverQuota
	case http.StatusTooManyRequests:
		return KindRateLimited
	}

	if statusCode >= http.StatusInternalServerError {
		return KindServerError
	}

	return KindUnexpected
}

func extractRequestID(header http.Header) string {
	return header.Get(headerRequestID)
}

// fault is a structured API error: either a top-level object with code and message or, as in other
// OpenStack services, such an object wrapped in a status-specific field.
type fault struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func parseFault(raw []byte) *fault {
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return nil
	}

	top := &fault{}
	if err := json.Unmarshal(raw, top); err == nil && (top.Message != "" || top.Code != 0) {
		return top
	}

	for _, value := range wrapper {
		parsed := &fault{}
		if err := json.Unmarshal(value, parsed); err != nil {
			continue
		}

		if parsed.Message != "" || parsed.Code != 0 {
			return parsed
		}
	}

	return nil
}

// diagnosticMessage keeps the fault message or, for the HTML pages that webob produces, the text
// without tags.
func diagnosticMessage(raw []byte, fault *fault) string {
	if fault != nil && fault.Message != "" {
		return truncate(strings.Join(strings.Fields(fault.Message), " "))
	}

	text := htmlTagPattern.ReplaceAllString(string(raw), " ")

	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}

	return truncate(strings.Join(strings.Fields(trimmed), " "))
}

func truncate(message string) string {
	if len(message) <= maxDiagnosticMessageLength {
		return message
	}

	return message[:maxDiagnosticMessageLength-len(diagnosticTruncationMarker)] + diagnosticTruncationMarker
}
