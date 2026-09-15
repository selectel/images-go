package transport

import "net/url"

type requestOptions struct {
	query            url.Values
	responseEnvelope string
	contentType      string
}

type RequestOption func(*requestOptions)

func newRequestOptions(options []RequestOption) *requestOptions {
	result := &requestOptions{}

	for _, option := range options {
		option(result)
	}

	return result
}

func WithQuery(query url.Values) RequestOption {
	return func(options *requestOptions) {
		options.query = query
	}
}

func WithResponseEnvelope(name string) RequestOption {
	return func(options *requestOptions) {
		options.responseEnvelope = name
	}
}

// WithContentType replaces the JSON content type of an encoded request body, for example with the
// Glance JSON Patch media type.
func WithContentType(contentType string) RequestOption {
	return func(options *requestOptions) {
		options.contentType = contentType
	}
}
