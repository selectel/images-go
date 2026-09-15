package image

import (
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
)

// CreateOpts describes a new image. Zero values are not sent, so the API applies its defaults.
// Properties become top-level custom properties of the image and cannot use a base field name.
type CreateOpts struct {
	Name            string
	ID              string
	Visibility      string
	Protected       *bool
	Hidden          *bool
	ContainerFormat string
	DiskFormat      string
	MinDisk         int
	MinRAM          int
	Tags            []string
	Properties      map[string]string
}

var errPropertyCollidesWithBase = errors.New("a custom property cannot use the name of a base image field")

// MarshalJSON flattens the custom properties into the request body next to the base fields.
func (opts CreateOpts) MarshalJSON() ([]byte, error) {
	body := map[string]any{}

	if opts.Name != "" {
		body["name"] = opts.Name
	}

	if opts.ID != "" {
		body["id"] = opts.ID
	}

	if opts.Visibility != "" {
		body["visibility"] = opts.Visibility
	}

	if opts.Protected != nil {
		body["protected"] = *opts.Protected
	}

	if opts.Hidden != nil {
		body["os_hidden"] = *opts.Hidden
	}

	if opts.ContainerFormat != "" {
		body["container_format"] = opts.ContainerFormat
	}

	if opts.DiskFormat != "" {
		body["disk_format"] = opts.DiskFormat
	}

	if opts.MinDisk != 0 {
		body["min_disk"] = opts.MinDisk
	}

	if opts.MinRAM != 0 {
		body["min_ram"] = opts.MinRAM
	}

	if len(opts.Tags) != 0 {
		body["tags"] = opts.Tags
	}

	for key, value := range opts.Properties {
		if _, base := baseKeys[key]; base {
			return nil, errPropertyCollidesWithBase
		}

		body[key] = value
	}

	return json.Marshal(body)
}

// ListOpts are the server-side filters of a listing. Empty values are not sent. Tags requires
// every listed tag. Sort uses the key:direction syntax of the API, comma separated.
type ListOpts struct {
	Name            string
	Visibility      string
	Hidden          *bool
	MemberStatus    string
	Owner           string
	Status          string
	SizeMin         int64
	SizeMax         int64
	Sort            string
	ContainerFormat string
	DiskFormat      string
	Tags            []string
}

func (opts ListOpts) query() url.Values {
	query := url.Values{}

	for key, value := range map[string]string{
		"name": opts.Name, "visibility": opts.Visibility, "member_status": opts.MemberStatus,
		"owner": opts.Owner, "status": opts.Status, "sort": opts.Sort,
		"container_format": opts.ContainerFormat, "disk_format": opts.DiskFormat,
	} {
		if value != "" {
			query.Set(key, value)
		}
	}

	if opts.Hidden != nil {
		query.Set("os_hidden", strconv.FormatBool(*opts.Hidden))
	}

	if opts.SizeMin != 0 {
		query.Set("size_min", strconv.FormatInt(opts.SizeMin, 10))
	}

	if opts.SizeMax != 0 {
		query.Set("size_max", strconv.FormatInt(opts.SizeMax, 10))
	}

	for _, tag := range opts.Tags {
		if trimmed := strings.TrimSpace(tag); trimmed != "" {
			query.Add("tag", trimmed)
		}
	}

	return query
}

// PatchOp is one JSON Patch operation of Update. Path names an image field or custom property,
// for example /name or /os_distro. Value is omitted for remove.
type PatchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value,omitempty"`
}
