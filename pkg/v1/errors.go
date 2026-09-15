package v1

import "github.com/selectel/images-go/pkg/v1/internal/transport"

type Kind = transport.Kind

const (
	// KindNotFound means that the requested resource does not exist for the current scope.
	KindNotFound = transport.KindNotFound

	// KindForbidden means that the token is not allowed to perform the operation.
	KindForbidden = transport.KindForbidden

	// KindConflict means that the current state of the resource does not allow the operation.
	KindConflict = transport.KindConflict

	// KindInvalidRequest means that the request itself was rejected as malformed or invalid.
	KindInvalidRequest = transport.KindInvalidRequest

	// KindOverQuota means that a project quota is exhausted and requires operator action.
	KindOverQuota = transport.KindOverQuota

	// KindRateLimited means that an intermediary throttled the request and it may succeed later.
	KindRateLimited = transport.KindRateLimited

	// KindServerError means that the API answered with a 5xx status code.
	KindServerError = transport.KindServerError

	// KindTransport means that the request has not produced an HTTP response.
	KindTransport = transport.KindTransport

	// KindTimeout means that the request timed out.
	KindTimeout = transport.KindTimeout

	// KindCanceled means that the caller canceled the request context.
	KindCanceled = transport.KindCanceled

	// KindIncompleteList means that a paginated read did not produce the complete collection.
	KindIncompleteList = transport.KindIncompleteList

	// KindUnexpected means that a response could not be classified or decoded.
	KindUnexpected = transport.KindUnexpected
)

type Error = transport.Error

// IsKind checks the complete error chain for kind.
func IsKind(err error, kind Kind) bool {
	return transport.IsKind(err, kind)
}
