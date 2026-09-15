package v1

import "github.com/selectel/images-go/pkg/v1/internal/transport"

// Custom HTTPClient implementations receive authentication headers.
type HTTPClient = transport.HTTPClient

type Client = transport.Client

// Endpoint and Token are required. UserAgent and HTTPClient are optional.
//
// Endpoint is the regional Image API endpoint from the service catalog without the /v2 suffix;
// a trailing /v2 is removed. Operations add the API version to their paths.
type Config struct {
	Endpoint   string
	Token      string
	UserAgent  string
	HTTPClient HTTPClient
}

type Response = transport.Response

func NewClient(config Config) (*Client, error) {
	return transport.NewClient(config.Endpoint, config.Token, config.UserAgent, config.HTTPClient)
}
