package member

import (
	"context"
	"net/http"
	"net/url"

	v1 "github.com/selectel/images-go/pkg/v1"
	"github.com/selectel/images-go/pkg/v1/internal/transport"
)

const membersEnvelopeName = "members"

func requireIDs(imageID, memberID string, memberRequired bool) error {
	if imageID == "" {
		return &v1.Error{Kind: v1.KindInvalidRequest, Message: "the image ID is required"}
	}

	if memberRequired && memberID == "" {
		return &v1.Error{Kind: v1.KindInvalidRequest, Message: "the member project ID is required"}
	}

	return nil
}

func membersPath(imageID string) string {
	return "v2/images/" + url.PathEscape(imageID) + "/members"
}

func memberPath(imageID, memberID string) string {
	return membersPath(imageID) + "/" + url.PathEscape(memberID)
}

type createRequest struct {
	Member string `json:"member"`
}

type updateRequest struct {
	Status string `json:"status"`
}

// Create grants the project memberID access to a shared image. The new membership is pending until
// the member project accepts it.
func Create(ctx context.Context, client *v1.Client, imageID, memberID string) (*View, *v1.Response, error) {
	if err := requireIDs(imageID, memberID, true); err != nil {
		return nil, nil, err
	}

	view := &View{}

	response, err := transport.DoRequest(
		ctx, client, http.MethodPost, membersPath(imageID), http.StatusOK, createRequest{Member: memberID}, view,
	)
	if err != nil {
		return nil, response, err
	}

	return view, response, nil
}

// Get returns one membership of an image.
func Get(ctx context.Context, client *v1.Client, imageID, memberID string) (*View, *v1.Response, error) {
	if err := requireIDs(imageID, memberID, true); err != nil {
		return nil, nil, err
	}

	view := &View{}

	response, err := transport.DoRequest(
		ctx, client, http.MethodGet, memberPath(imageID, memberID), http.StatusOK, nil, view,
	)
	if err != nil {
		return nil, response, err
	}

	return view, response, nil
}

// List returns the memberships of an image that are visible to the caller. The list is not
// paginated by the API.
func List(ctx context.Context, client *v1.Client, imageID string) ([]View, *v1.Response, error) {
	if err := requireIDs(imageID, "", false); err != nil {
		return nil, nil, err
	}

	envelope := &listEnvelope{}

	response, err := transport.DoRequest(
		ctx, client, http.MethodGet, membersPath(imageID), http.StatusOK, nil, envelope,
		transport.WithResponseEnvelope(membersEnvelopeName),
	)
	if err != nil {
		return nil, response, err
	}

	return envelope.Members, response, nil
}

// Update sets the status of a membership. Only the member project may change it.
func Update(ctx context.Context, client *v1.Client, imageID, memberID, status string) (*View, *v1.Response, error) {
	if err := requireIDs(imageID, memberID, true); err != nil {
		return nil, nil, err
	}

	if status == "" {
		return nil, nil, &v1.Error{Kind: v1.KindInvalidRequest, Message: "the member status is required"}
	}

	view := &View{}

	response, err := transport.DoRequest(
		ctx, client, http.MethodPut, memberPath(imageID, memberID), http.StatusOK, updateRequest{Status: status}, view,
	)
	if err != nil {
		return nil, response, err
	}

	return view, response, nil
}

// Delete revokes the access of a member project.
func Delete(ctx context.Context, client *v1.Client, imageID, memberID string) (*v1.Response, error) {
	if err := requireIDs(imageID, memberID, true); err != nil {
		return nil, err
	}

	return transport.DoRequest(
		ctx, client, http.MethodDelete, memberPath(imageID, memberID), http.StatusNoContent, nil, nil,
	)
}
