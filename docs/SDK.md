# Go SDK

The `nvfleetint` package provides a handwritten Go client for the Fleet
Intelligence customer API.

## Install

```bash
go get github.com/NVIDIA/fleet-intelligence-client/nvfleetint
```

Use the Go version declared in the repository's `go.mod` file or newer.

## Create a client

```go
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

func main() {
	client, err := nvfleetint.NewClient(
		"https://api.fleet-intelligence.nvidia.com",
		os.Getenv("NVFLEETINT_API_KEY"),
	)
	if err != nil {
		log.Fatal(err)
	}

	page, err := client.ListNodes(context.Background(), nvfleetint.ListNodesOptions{})
	if err != nil {
		log.Fatal(err)
	}

	for _, node := range page.Nodes {
		fmt.Printf("%s\t%s\n", node.UUID, node.Hostname)
	}
}
```

`NewClient` requires an HTTPS API URL and an API key. Plain HTTP is accepted
only for loopback addresses used during local development.

The default per-request timeout is two minutes. Override it when constructing
the client:

```go
client, err := nvfleetint.NewClient(
	apiURL,
	apiKey,
	nvfleetint.WithTimeout(30*time.Second),
)
```

Public SDK types and methods are documented in the package source under
[`nvfleetint`](../nvfleetint).
