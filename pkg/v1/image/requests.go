package image

import (
	"context"
	"io"
	"net/http"
	"net/url"

	v1 "github.com/selectel/images-go/pkg/v1"
	"github.com/selectel/images-go/pkg/v1/internal/pagination"
	"github.com/selectel/images-go/pkg/v1/internal/transport"
)

const (
	basePath          = "v2/images"
	patchContentType  = "application/openstack-images-v2.1-json-patch"
	tasksEnvelopeName = "tasks"
)

func requireID(imageID string) error {
	if imageID == "" {
		return &v1.Error{Kind: v1.KindInvalidRequest, Message: "the image ID is required"}
	}

	return nil
}

func imagePath(imageID string, segments ...string) string {
	path := basePath + "/" + url.PathEscape(imageID)
	for _, segment := range segments {
		path += "/" + segment
	}

	return path
}

// Create registers a new image without data. The image starts in the queued status.
func Create(ctx context.Context, client *v1.Client, opts CreateOpts) (*View, *v1.Response, error) {
	view := &View{}

	response, err := transport.DoRequest(ctx, client, http.MethodPost, basePath, http.StatusCreated, opts, view)
	if err != nil {
		return nil, response, err
	}

	return view, response, nil
}

// Get returns one image by ID.
func Get(ctx context.Context, client *v1.Client, imageID string) (*View, *v1.Response, error) {
	if err := requireID(imageID); err != nil {
		return nil, nil, err
	}

	view := &View{}

	response, err := transport.DoRequest(ctx, client, http.MethodGet, imagePath(imageID), http.StatusOK, nil, view)
	if err != nil {
		return nil, response, err
	}

	return view, response, nil
}

// List returns all images matching opts and never returns a partial result.
func List(ctx context.Context, client *v1.Client, opts ListOpts) ([]View, error) {
	return pagination.ReadAll[View](ctx, client, basePath, "images", opts.query(), 0, func() *viewPage {
		return &viewPage{}
	})
}

// Update applies JSON Patch operations to an image and returns its new state.
func Update(ctx context.Context, client *v1.Client, imageID string, ops []PatchOp) (*View, *v1.Response, error) {
	if err := requireID(imageID); err != nil {
		return nil, nil, err
	}

	if len(ops) == 0 {
		return nil, nil, &v1.Error{Kind: v1.KindInvalidRequest, Message: "at least one patch operation is required"}
	}

	view := &View{}

	response, err := transport.DoRequest(
		ctx, client, http.MethodPatch, imagePath(imageID), http.StatusOK, ops, view,
		transport.WithContentType(patchContentType),
	)
	if err != nil {
		return nil, response, err
	}

	return view, response, nil
}

// Delete removes an image.
func Delete(ctx context.Context, client *v1.Client, imageID string) (*v1.Response, error) {
	if err := requireID(imageID); err != nil {
		return nil, err
	}

	return transport.DoRequest(ctx, client, http.MethodDelete, imagePath(imageID), http.StatusNoContent, nil, nil)
}

// Upload streams size bytes of image data. The request carries an explicit Content-Length and is
// bounded only by ctx.
func Upload(ctx context.Context, client *v1.Client, imageID string, data io.Reader, size int64) (*v1.Response, error) {
	if err := requireID(imageID); err != nil {
		return nil, err
	}

	return transport.DoUpload(
		ctx, client, http.MethodPut, imagePath(imageID, "file"), http.StatusNoContent, data, size,
	)
}

type importRequest struct {
	Method importMethod `json:"method"`
}

type importMethod struct {
	Name string `json:"name"`
	URI  string `json:"uri"`
}

// Import asks the service to download the image data from uri with the web-download method. The
// import runs asynchronously; its outcome is visible through the image status and Tasks.
func Import(ctx context.Context, client *v1.Client, imageID, uri string) (*v1.Response, error) {
	if err := requireID(imageID); err != nil {
		return nil, err
	}

	if uri == "" {
		return nil, &v1.Error{Kind: v1.KindInvalidRequest, Message: "the import URI is required"}
	}

	body := importRequest{Method: importMethod{Name: ImportMethodWebDownload, URI: uri}}

	return transport.DoRequest(
		ctx, client, http.MethodPost, imagePath(imageID, "import"), http.StatusAccepted, body, nil,
	)
}

// Tasks returns the tasks of an image, including finished imports and their failure reasons.
func Tasks(ctx context.Context, client *v1.Client, imageID string) ([]Task, *v1.Response, error) {
	if err := requireID(imageID); err != nil {
		return nil, nil, err
	}

	envelope := &tasksEnvelope{}

	response, err := transport.DoRequest(
		ctx, client, http.MethodGet, imagePath(imageID, "tasks"), http.StatusOK, nil, envelope,
		transport.WithResponseEnvelope(tasksEnvelopeName),
	)
	if err != nil {
		return nil, response, err
	}

	return envelope.Tasks, response, nil
}
