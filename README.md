# images-go: Go SDK for the Selectel Images API
[![Go.dev reference](https://img.shields.io/badge/go.dev-reference-007d9c?logo=go&logoColor=white&style=flat-square)](https://pkg.go.dev/github.com/selectel/images-go/)
[![Go Report Card](https://goreportcard.com/badge/github.com/selectel/images-go)](https://goreportcard.com/report/github.com/selectel/images-go)
![Build Status](https://github.com/selectel/images-go/actions/workflows/unit-tests.yml/badge.svg)

Package images-go provides Go SDK to work with the Selectel Images API, which is an OpenStack Glance v2 API.

## Documentation

The Go library documentation is available at [go.dev](https://pkg.go.dev/github.com/selectel/images-go/).

## What this library is capable of

You can use this library to work with the following objects of the Selectel Images API:

* [image](https://pkg.go.dev/github.com/selectel/images-go/pkg/v1/image)
* [member](https://pkg.go.dev/github.com/selectel/images-go/pkg/v1/member)

## Getting started

### Installation

You can install needed `images-go` packages via `go get` command:

```bash
go get github.com/selectel/images-go/pkg/v1/image github.com/selectel/images-go/pkg/v1/member
```

### Authentication

To work with the Selectel Images API you first need to:

* Create a Selectel account: [registration page](https://my.selectel.ru/registration).
* Create a project in Selectel Cloud Platform [projects](https://my.selectel.ru/vpc/projects).
* Retrieve a token for your project via API or [go-selvpcclient](https://github.com/selectel/go-selvpcclient).

### Endpoints

The Images API endpoint is region-specific. Retrieve the endpoint of the `image` service type for
the required region from the Identity catalog.

### Usage example

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	v1 "github.com/selectel/images-go/pkg/v1"
	"github.com/selectel/images-go/pkg/v1/image"
	"github.com/selectel/images-go/pkg/v1/member"
)

func main() {
	ctx := context.Background()

	// Token of the project and endpoint of the Images API in the required region.
	token := "gAAAAABeVNzu-..."
	endpoint := "<image-endpoint-from-the-identity-catalog>"

	// Initialize the Images API client.
	client, err := v1.NewClient(v1.Config{
		Endpoint:  endpoint,
		Token:     token,
		UserAgent: "my-application/v1.0.0",
	})
	if err != nil {
		log.Fatal(err)
	}

	// Create an image.
	newImage, _, err := image.Create(ctx, client, image.CreateOpts{
		Name:            "test-image",
		ContainerFormat: "bare",
		DiskFormat:      "qcow2",
	})
	if err != nil {
		log.Fatal(err)
	}

	// Upload the image data.
	file, err := os.Open("test-image.qcow2")
	if err != nil {
		log.Fatal(err)
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		log.Fatal(err)
	}

	if _, err := image.Upload(ctx, client, newImage.ID, file, info.Size()); err != nil {
		log.Fatal(err)
	}

	// Print the image fields.
	uploaded, _, err := image.Get(ctx, client, newImage.ID)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Image: %+v\n", uploaded)

	// Share the image with another project.
	if _, _, err := member.Create(ctx, client, newImage.ID, "9c5cd8d6d4bb4f4a91e3d5e6f7a8b9c0"); err != nil {
		log.Fatal(err)
	}
}
```
