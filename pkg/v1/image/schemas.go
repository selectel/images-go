package image

import (
	"encoding/json"
	"strings"
	"time"
)

// Image statuses reported by the API.
const (
	StatusQueued        = "queued"
	StatusSaving        = "saving"
	StatusImporting     = "importing"
	StatusActive        = "active"
	StatusKilled        = "killed"
	StatusDeactivated   = "deactivated"
	StatusPendingDelete = "pending_delete"
	StatusDeleted       = "deleted"
)

// Image visibilities.
const (
	VisibilityPrivate   = "private"
	VisibilityShared    = "shared"
	VisibilityCommunity = "community"
	VisibilityPublic    = "public"
)

// Task statuses reported by the API.
const (
	TaskStatusPending    = "pending"
	TaskStatusProcessing = "processing"
	TaskStatusSuccess    = "success"
	TaskStatusFailure    = "failure"
)

// TaskTypeImport is the type of the task that the import operation creates.
const TaskTypeImport = "api_image_import"

// ImportMethodWebDownload asks the service to download the image data from a URL.
const ImportMethodWebDownload = "web-download"

// JSON Patch operations accepted by Update.
const (
	OpAdd     = "add"
	OpReplace = "replace"
	OpRemove  = "remove"
)

// View is an image as the API reports it. Properties holds the custom properties that are not
// part of the base image schema; the API returns every custom property as a string.
type View struct {
	ID              string
	Name            string
	Status          string
	Visibility      string
	Protected       bool
	Hidden          bool
	Checksum        string
	HashAlgo        string
	HashValue       string
	Owner           string
	Size            int64
	VirtualSize     int64
	ContainerFormat string
	DiskFormat      string
	MinDisk         int
	MinRAM          int
	CreatedAt       time.Time
	UpdatedAt       time.Time
	Tags            []string
	File            string
	Self            string
	Schema          string
	DirectURL       string
	Stores          string
	Properties      map[string]string
}

type baseView struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	Status          string    `json:"status"`
	Visibility      string    `json:"visibility"`
	Protected       bool      `json:"protected"`
	Hidden          bool      `json:"os_hidden"`
	Checksum        string    `json:"checksum"`
	HashAlgo        string    `json:"os_hash_algo"`
	HashValue       string    `json:"os_hash_value"`
	Owner           string    `json:"owner"`
	Size            int64     `json:"size"`
	VirtualSize     int64     `json:"virtual_size"`
	ContainerFormat string    `json:"container_format"`
	DiskFormat      string    `json:"disk_format"`
	MinDisk         int       `json:"min_disk"`
	MinRAM          int       `json:"min_ram"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	Tags            []string  `json:"tags"`
	File            string    `json:"file"`
	Self            string    `json:"self"`
	Schema          string    `json:"schema"`
	DirectURL       string    `json:"direct_url"`
	Stores          string    `json:"stores"`
}

// baseKeys are the image fields that are not custom properties. locations is reserved by the API
// and never exposed as a property.
var baseKeys = map[string]struct{}{
	"id": {}, "name": {}, "status": {}, "visibility": {}, "protected": {}, "os_hidden": {},
	"checksum": {}, "os_hash_algo": {}, "os_hash_value": {}, "owner": {}, "size": {},
	"virtual_size": {}, "container_format": {}, "disk_format": {}, "min_disk": {}, "min_ram": {},
	"created_at": {}, "updated_at": {}, "tags": {}, "file": {}, "self": {}, "schema": {},
	"direct_url": {}, "stores": {}, "locations": {},
}

// UnmarshalJSON decodes the base fields and keeps every other key as a custom property. A custom
// property that the API did not return as a string is kept as its JSON text.
func (v *View) UnmarshalJSON(data []byte) error {
	var base baseView
	if err := json.Unmarshal(data, &base); err != nil {
		return err
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	*v = View{
		ID: base.ID, Name: base.Name, Status: base.Status, Visibility: base.Visibility,
		Protected: base.Protected, Hidden: base.Hidden, Checksum: base.Checksum,
		HashAlgo: base.HashAlgo, HashValue: base.HashValue, Owner: base.Owner, Size: base.Size,
		VirtualSize: base.VirtualSize, ContainerFormat: base.ContainerFormat, DiskFormat: base.DiskFormat,
		MinDisk: base.MinDisk, MinRAM: base.MinRAM, CreatedAt: base.CreatedAt, UpdatedAt: base.UpdatedAt,
		Tags: base.Tags, File: base.File, Self: base.Self, Schema: base.Schema,
		DirectURL: base.DirectURL, Stores: base.Stores, Properties: map[string]string{},
	}

	for key, value := range raw {
		if _, base := baseKeys[key]; base {
			continue
		}

		var text string
		if err := json.Unmarshal(value, &text); err != nil {
			text = string(value)
		}

		v.Properties[key] = text
	}

	return nil
}

type viewPage struct {
	Images []View `json:"images"`
	Next   string `json:"next"`
}

func (p *viewPage) Items() []View { return p.Images }

func (p *viewPage) NextHref() string { return p.Next }

// Task is an asynchronous operation on an image, for example an import. Message carries the
// failure reason key when Status is failure.
type Task struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Status    string    `json:"status"`
	Message   string    `json:"message"`
	CreatedAt Timestamp `json:"created_at"`
	UpdatedAt Timestamp `json:"updated_at"`
}

type tasksEnvelope struct {
	Tasks []Task `json:"tasks"`
}

// Timestamp decodes the task timestamps, which the API returns either in RFC 3339 or as an ISO
// 8601 value without a zone that denotes UTC.
type Timestamp struct {
	time.Time
}

const isoWithoutZone = "2006-01-02T15:04:05.999999999"

func (t *Timestamp) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err != nil {
		return err
	}

	text = strings.TrimSpace(text)
	if text == "" {
		t.Time = time.Time{}

		return nil
	}

	parsed, err := time.Parse(time.RFC3339Nano, text)
	if err != nil {
		parsed, err = time.Parse(isoWithoutZone, text)
	}

	if err != nil {
		return err
	}

	t.Time = parsed

	return nil
}
