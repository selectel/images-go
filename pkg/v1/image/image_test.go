package image

import (
	"net/http"
	"strings"
	"testing"
	"time"

	v1 "github.com/selectel/images-go/pkg/v1"
	"github.com/stretchr/testify/require"
)

func TestView_UnmarshalJSON(t *testing.T) {
	var view View
	require.NoError(t, view.UnmarshalJSON([]byte(imageJSON)))

	require.Equal(t, testImageID, view.ID)
	require.Equal(t, testName, view.Name)
	require.Equal(t, StatusActive, view.Status)
	require.Equal(t, VisibilityShared, view.Visibility)
	require.True(t, view.Protected)
	require.False(t, view.Hidden)
	require.Equal(t, "d41d8cd98f00b204e9800998ecf8427e", view.Checksum)
	require.Equal(t, "sha512", view.HashAlgo)
	require.Equal(t, "cf83e1", view.HashValue)
	require.Equal(t, "7f3a", view.Owner)
	require.Equal(t, int64(1024), view.Size)
	require.Equal(t, int64(4096), view.VirtualSize)
	require.Equal(t, testBare, view.ContainerFormat)
	require.Equal(t, "qcow2", view.DiskFormat)
	require.Equal(t, 1, view.MinDisk)
	require.Equal(t, 512, view.MinRAM)
	require.Equal(t, time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC), view.CreatedAt)
	require.Equal(t, time.Date(2026, 9, 8, 11, 30, 0, 0, time.UTC), view.UpdatedAt)
	require.Equal(t, []string{testTag, "lts"}, view.Tags)
	require.Equal(t, "/v2/images/"+testImageID+"/file", view.File)
	require.Equal(t, "/v2/schemas/image", view.Schema)
	require.Equal(t, "rbd://pool/img", view.DirectURL)
	require.Equal(t, "rbd", view.Stores)
	require.Equal(t, map[string]string{
		"os_distro":                     testName,
		"hw_qemu_guest_agent":           "yes",
		"os_glance_importing_to_stores": "",
		"weird_number":                  "42",
	}, view.Properties, "custom properties keep strings and stringify other values; locations is not a property")
}

func TestView_UnmarshalJSON_NullSizeBeforeUpload(t *testing.T) {
	var view View
	require.NoError(t, view.UnmarshalJSON([]byte(`{"id": "x", "status": "queued", "size": null, "checksum": null, "tags": []}`)))
	require.Zero(t, view.Size)
	require.Empty(t, view.Checksum)
	require.Empty(t, view.Tags)
	require.Empty(t, view.Properties)
}

func TestTimestamp_UnmarshalJSON(t *testing.T) {
	cases := map[string]time.Time{
		`"2026-09-08T10:00:00Z"`:       time.Date(2026, 9, 8, 10, 0, 0, 0, time.UTC),
		`"2026-09-08T10:00:00.123456"`: time.Date(2026, 9, 8, 10, 0, 0, 123456000, time.UTC),
		`"2026-09-08T10:00:00+03:00"`:  time.Date(2026, 9, 8, 7, 0, 0, 0, time.UTC),
		`""`:                           {},
	}

	for input, expected := range cases {
		var stamp Timestamp
		require.NoError(t, stamp.UnmarshalJSON([]byte(input)), input)
		require.True(t, expected.Equal(stamp.Time), "%s: %s", input, stamp.Time)
	}

	var stamp Timestamp
	require.Error(t, stamp.UnmarshalJSON([]byte(`"yesterday"`)))
}

func TestCreate(t *testing.T) {
	t.Run("SendsFlattenedBody", func(t *testing.T) {
		client, httpClient := testClient(t, testAnswer{status: http.StatusCreated, body: imageJSON})

		view, response, err := Create(t.Context(), client, CreateOpts{
			Name: testName, ID: testImageID, Visibility: VisibilityShared, Protected: boolPtr(true),
			Hidden: boolPtr(false), ContainerFormat: testBare, DiskFormat: "qcow2", MinDisk: 1, MinRAM: 512,
			Tags: []string{testTag}, Properties: map[string]string{"os_distro": testName},
		})
		require.NoError(t, err)
		require.Equal(t, http.StatusCreated, response.StatusCode)
		require.Equal(t, testImageID, view.ID)

		request := httpClient.lastRequest(t)
		require.Equal(t, http.MethodPost, request.Method)
		require.Equal(t, testEndpoint+"/v2/images", request.URL.String())
		require.JSONEq(t, `{"name":"ubuntu","id":"`+testImageID+`","visibility":"shared","protected":true,`+
			`"os_hidden":false,"container_format":"bare","disk_format":"qcow2","min_disk":1,"min_ram":512,`+
			`"tags":["web"],"os_distro":"ubuntu"}`, httpClient.lastBody(t))
	})

	t.Run("OmitsZeroValues", func(t *testing.T) {
		client, httpClient := testClient(t, testAnswer{status: http.StatusCreated, body: imageJSON})

		_, _, err := Create(t.Context(), client, CreateOpts{Name: "minimal", ContainerFormat: testBare, DiskFormat: "raw"})
		require.NoError(t, err)
		require.JSONEq(t, `{"name":"minimal","container_format":"bare","disk_format":"raw"}`, httpClient.lastBody(t))
	})

	t.Run("RejectsPropertyNamedLikeABaseField", func(t *testing.T) {
		client, httpClient := testClient(t, testAnswer{status: http.StatusCreated, body: imageJSON})

		_, _, err := Create(t.Context(), client, CreateOpts{Name: "x", Properties: map[string]string{"status": "active"}})
		require.True(t, v1.IsKind(err, v1.KindInvalidRequest), "unexpected error: %v", err)
		require.Empty(t, httpClient.requests)
	})

	t.Run("ReportsForbidden", func(t *testing.T) {
		client, _ := testClient(t, testAnswer{status: http.StatusForbidden, body: `{"code": 403, "message": "denied"}`})

		view, response, err := Create(t.Context(), client, CreateOpts{Name: "x"})
		require.Nil(t, view)
		require.Equal(t, http.StatusForbidden, response.StatusCode)
		require.True(t, v1.IsKind(err, v1.KindForbidden), "unexpected error: %v", err)
	})
}

func TestGet(t *testing.T) {
	t.Run("DecodesImage", func(t *testing.T) {
		client, httpClient := testClient(t, testAnswer{status: http.StatusOK, body: imageJSON})

		view, response, err := Get(t.Context(), client, testImageID)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)
		require.Equal(t, testName, view.Name)

		request := httpClient.lastRequest(t)
		require.Equal(t, http.MethodGet, request.Method)
		require.Equal(t, testEndpoint+"/v2/images/"+testImageID, request.URL.String())
		require.Empty(t, httpClient.lastBody(t))
	})

	t.Run("NotFound", func(t *testing.T) {
		client, _ := testClient(t, testAnswer{status: http.StatusNotFound, body: `<html><body>404 Not Found</body></html>`})

		view, _, err := Get(t.Context(), client, testImageID)
		require.Nil(t, view)
		require.True(t, v1.IsKind(err, v1.KindNotFound), "unexpected error: %v", err)
	})

	t.Run("RequiresID", func(t *testing.T) {
		client, httpClient := testClient(t)

		_, _, err := Get(t.Context(), client, "")
		require.True(t, v1.IsKind(err, v1.KindInvalidRequest), "unexpected error: %v", err)
		require.Empty(t, httpClient.requests)
	})
}

func TestList(t *testing.T) {
	t.Run("SendsFiltersAndFollowsPages", func(t *testing.T) {
		client, httpClient := testClient(t,
			testAnswer{status: http.StatusOK, body: `{"images": [` + imageJSON + `], "next": "/v2/images?marker=` + testImageID + `&limit=1"}`},
			testAnswer{status: http.StatusOK, body: `{"images": [{"id": "second", "name": "debian", "status": "active"}]}`},
		)

		views, err := List(t.Context(), client, ListOpts{
			Name: testName, Visibility: VisibilityShared, Hidden: boolPtr(false), MemberStatus: "all",
			Owner: "7f3a", Status: StatusActive, SizeMin: 10, SizeMax: 2048, Sort: "name:asc,created_at:desc",
			ContainerFormat: testBare, DiskFormat: "qcow2", Tags: []string{testTag, " lts ", ""},
		})
		require.NoError(t, err)
		require.Len(t, views, 2)
		require.Equal(t, testName, views[0].Name)
		require.Equal(t, "second", views[1].ID)
		require.Len(t, httpClient.requests, 2)

		first := httpClient.requests[0].URL
		require.Equal(t, "/image/v2/images", first.Path)
		query := first.Query()
		require.Equal(t, testName, query.Get("name"))
		require.Equal(t, "shared", query.Get("visibility"))
		require.Equal(t, "false", query.Get("os_hidden"))
		require.Equal(t, "all", query.Get("member_status"))
		require.Equal(t, "7f3a", query.Get("owner"))
		require.Equal(t, "active", query.Get("status"))
		require.Equal(t, "10", query.Get("size_min"))
		require.Equal(t, "2048", query.Get("size_max"))
		require.Equal(t, "name:asc,created_at:desc", query.Get("sort"))
		require.Equal(t, testBare, query.Get("container_format"))
		require.Equal(t, "qcow2", query.Get("disk_format"))
		require.Equal(t, []string{testTag, "lts"}, query["tag"])
		require.Empty(t, query.Get("limit"))

		second := httpClient.requests[1].URL
		require.Equal(t, "/image/v2/images", second.Path)
		require.Equal(t, "marker="+testImageID+"&limit=1", second.RawQuery)
	})

	t.Run("OmitsEmptyFilters", func(t *testing.T) {
		client, httpClient := testClient(t, testAnswer{status: http.StatusOK, body: `{"images": []}`})

		views, err := List(t.Context(), client, ListOpts{})
		require.NoError(t, err)
		require.Empty(t, views)
		require.Empty(t, httpClient.lastRequest(t).URL.RawQuery)
	})

	t.Run("FailedPageIsIncomplete", func(t *testing.T) {
		client, _ := testClient(t,
			testAnswer{status: http.StatusOK, body: `{"images": [` + imageJSON + `], "next": "/v2/images?marker=x"}`},
			testAnswer{status: http.StatusInternalServerError, body: ``},
		)

		views, err := List(t.Context(), client, ListOpts{})
		require.Nil(t, views)
		require.True(t, v1.IsKind(err, v1.KindIncompleteList), "unexpected error: %v", err)
		require.True(t, v1.IsKind(err, v1.KindServerError), "unexpected error: %v", err)
	})
}

func TestUpdate(t *testing.T) {
	t.Run("SendsJSONPatch", func(t *testing.T) {
		client, httpClient := testClient(t, testAnswer{status: http.StatusOK, body: imageJSON})

		view, _, err := Update(t.Context(), client, testImageID, []PatchOp{
			{Op: OpReplace, Path: "/name", Value: "renamed"},
			{Op: OpAdd, Path: "/os_distro", Value: testName},
			{Op: OpRemove, Path: "/hw_qemu_guest_agent"},
			{Op: OpReplace, Path: "/tags", Value: []string{"a"}},
			{Op: OpReplace, Path: "/protected", Value: false},
		})
		require.NoError(t, err)
		require.Equal(t, testImageID, view.ID)

		request := httpClient.lastRequest(t)
		require.Equal(t, http.MethodPatch, request.Method)
		require.Equal(t, testEndpoint+"/v2/images/"+testImageID, request.URL.String())
		require.Equal(t, patchContentType, request.Header.Get("Content-Type"))
		require.JSONEq(t, `[{"op":"replace","path":"/name","value":"renamed"},{"op":"add","path":"/os_distro","value":"ubuntu"},`+
			`{"op":"remove","path":"/hw_qemu_guest_agent"},{"op":"replace","path":"/tags","value":["a"]},`+
			`{"op":"replace","path":"/protected","value":false}]`, httpClient.lastBody(t))
	})

	t.Run("RequiresOperations", func(t *testing.T) {
		client, httpClient := testClient(t)

		_, _, err := Update(t.Context(), client, testImageID, nil)
		require.True(t, v1.IsKind(err, v1.KindInvalidRequest), "unexpected error: %v", err)
		require.Empty(t, httpClient.requests)
	})

	t.Run("Forbidden", func(t *testing.T) {
		client, _ := testClient(t, testAnswer{status: http.StatusForbidden, body: `{"code": 403, "message": "You are not permitted to publicize."}`})

		_, _, err := Update(t.Context(), client, testImageID, []PatchOp{{Op: OpReplace, Path: "/visibility", Value: "public"}})
		require.True(t, v1.IsKind(err, v1.KindForbidden), "unexpected error: %v", err)
		require.Contains(t, err.Error(), "publicize")
	})
}

func TestDelete(t *testing.T) {
	t.Run("Deletes", func(t *testing.T) {
		client, httpClient := testClient(t, testAnswer{status: http.StatusNoContent})

		response, err := Delete(t.Context(), client, testImageID)
		require.NoError(t, err)
		require.Equal(t, http.StatusNoContent, response.StatusCode)

		request := httpClient.lastRequest(t)
		require.Equal(t, http.MethodDelete, request.Method)
		require.Equal(t, testEndpoint+"/v2/images/"+testImageID, request.URL.String())
	})

	t.Run("ProtectedImage", func(t *testing.T) {
		client, _ := testClient(t, testAnswer{status: http.StatusForbidden, body: `{"code": 403, "message": "Image is protected"}`})

		_, err := Delete(t.Context(), client, testImageID)
		require.True(t, v1.IsKind(err, v1.KindForbidden), "unexpected error: %v", err)
	})

	t.Run("NotFound", func(t *testing.T) {
		client, _ := testClient(t, testAnswer{status: http.StatusNotFound, body: ``})

		_, err := Delete(t.Context(), client, testImageID)
		require.True(t, v1.IsKind(err, v1.KindNotFound), "unexpected error: %v", err)
	})
}

func TestUpload(t *testing.T) {
	t.Run("StreamsData", func(t *testing.T) {
		client, httpClient := testClient(t, testAnswer{status: http.StatusNoContent})
		data := "raw disk bytes"

		response, err := Upload(t.Context(), client, testImageID, strings.NewReader(data), int64(len(data)))
		require.NoError(t, err)
		require.Equal(t, http.StatusNoContent, response.StatusCode)

		request := httpClient.lastRequest(t)
		require.Equal(t, http.MethodPut, request.Method)
		require.Equal(t, testEndpoint+"/v2/images/"+testImageID+"/file", request.URL.String())
		require.Equal(t, int64(len(data)), request.ContentLength)
		require.Equal(t, "application/octet-stream", request.Header.Get("Content-Type"))
		require.Equal(t, data, httpClient.lastBody(t))
	})

	t.Run("Conflict", func(t *testing.T) {
		client, _ := testClient(t, testAnswer{status: http.StatusConflict, body: `{"message": "Image status transition from active to saving is not allowed"}`})

		_, err := Upload(t.Context(), client, testImageID, strings.NewReader("x"), 1)
		require.True(t, v1.IsKind(err, v1.KindConflict), "unexpected error: %v", err)
	})

	t.Run("StorageQuota", func(t *testing.T) {
		client, _ := testClient(t, testAnswer{status: http.StatusRequestEntityTooLarge, body: `<html><body>Image storage media is full</body></html>`})

		_, err := Upload(t.Context(), client, testImageID, strings.NewReader("x"), 1)
		require.True(t, v1.IsKind(err, v1.KindOverQuota), "unexpected error: %v", err)
	})
}

func TestImport(t *testing.T) {
	t.Run("SendsWebDownload", func(t *testing.T) {
		client, httpClient := testClient(t, testAnswer{status: http.StatusAccepted})

		response, err := Import(t.Context(), client, testImageID, "https://cloud-images.example.com/disk.img")
		require.NoError(t, err)
		require.Equal(t, http.StatusAccepted, response.StatusCode)

		request := httpClient.lastRequest(t)
		require.Equal(t, http.MethodPost, request.Method)
		require.Equal(t, testEndpoint+"/v2/images/"+testImageID+"/import", request.URL.String())
		require.JSONEq(t, `{"method":{"name":"web-download","uri":"https://cloud-images.example.com/disk.img"}}`, httpClient.lastBody(t))
	})

	t.Run("RequiresURI", func(t *testing.T) {
		client, httpClient := testClient(t)

		_, err := Import(t.Context(), client, testImageID, "")
		require.True(t, v1.IsKind(err, v1.KindInvalidRequest), "unexpected error: %v", err)
		require.Empty(t, httpClient.requests)
	})

	t.Run("RejectedMethod", func(t *testing.T) {
		client, _ := testClient(t, testAnswer{status: http.StatusBadRequest, body: `{"code": 400, "message": "Unknown import method name 'web-download'."}`})

		_, err := Import(t.Context(), client, testImageID, "https://example.com/x")
		require.True(t, v1.IsKind(err, v1.KindInvalidRequest), "unexpected error: %v", err)
	})
}

func TestTasks(t *testing.T) {
	t.Run("DecodesFailedImport", func(t *testing.T) {
		client, httpClient := testClient(t, testAnswer{status: http.StatusOK, body: `{"tasks": [
			{"id": "t1", "type": "api_image_import", "status": "failure", "message": "url_not_accessible",
			 "created_at": "2026-09-08T10:00:00.123456", "updated_at": "2026-09-08T10:00:05.654321",
			 "owner": "7f3a", "input": {"image_id": "` + testImageID + `"}, "result": null},
			{"id": "t0", "type": "api_image_import", "status": "success", "message": "",
			 "created_at": "2026-09-07T10:00:00Z", "updated_at": "2026-09-07T10:01:00Z"}
		]}`})

		tasks, response, err := Tasks(t.Context(), client, testImageID)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, response.StatusCode)
		require.Len(t, tasks, 2)

		require.Equal(t, "t1", tasks[0].ID)
		require.Equal(t, TaskTypeImport, tasks[0].Type)
		require.Equal(t, TaskStatusFailure, tasks[0].Status)
		require.Equal(t, "url_not_accessible", tasks[0].Message)
		require.True(t, tasks[0].CreatedAt.Equal(time.Date(2026, 9, 8, 10, 0, 0, 123456000, time.UTC)))
		require.True(t, tasks[0].UpdatedAt.After(tasks[0].CreatedAt.Time))
		require.Equal(t, TaskStatusSuccess, tasks[1].Status)

		request := httpClient.lastRequest(t)
		require.Equal(t, http.MethodGet, request.Method)
		require.Equal(t, testEndpoint+"/v2/images/"+testImageID+"/tasks", request.URL.String())
	})

	t.Run("EmptyList", func(t *testing.T) {
		client, _ := testClient(t, testAnswer{status: http.StatusOK, body: `{"tasks": []}`})

		tasks, _, err := Tasks(t.Context(), client, testImageID)
		require.NoError(t, err)
		require.Empty(t, tasks)
	})

	t.Run("MissingEnvelope", func(t *testing.T) {
		client, _ := testClient(t, testAnswer{status: http.StatusOK, body: `{"images": []}`})

		_, _, err := Tasks(t.Context(), client, testImageID)
		require.True(t, v1.IsKind(err, v1.KindUnexpected), "unexpected error: %v", err)
	})

	t.Run("NotFound", func(t *testing.T) {
		client, _ := testClient(t, testAnswer{status: http.StatusNotFound, body: ``})

		_, _, err := Tasks(t.Context(), client, testImageID)
		require.True(t, v1.IsKind(err, v1.KindNotFound), "unexpected error: %v", err)
	})
}
