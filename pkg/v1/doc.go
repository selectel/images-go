/*
Package v1 provides the client and common types for the Selectel Images SDK.
The package version is independent of the Image API version.

The client uses a regional Image API endpoint and a project-scoped token supplied by the caller.
It does not authenticate, retry requests, or wait for image state changes. API operations are
provided by resource packages such as github.com/selectel/images-go/pkg/v1/image.
*/
package v1
