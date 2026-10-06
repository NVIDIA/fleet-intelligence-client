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

Select the out-of-band node view and retrieve its BMC inventory with explicit
source options:

```go
page, err := client.ListNodes(ctx, nvfleetint.ListNodesOptions{
	DataSource:   nvfleetint.NodeDataSourceOOB,
	BMCHostname: "bmc-01",
})

node, err := client.DescribeNodeWithOptions(ctx, nodeUUID, nvfleetint.DescribeNodeOptions{
	DataSource: nvfleetint.NodeDataSourceOOB,
})
if err == nil && node.OOBInventory != nil {
	fmt.Println(node.OOBInventory.SchemaVersion, node.NodeKind)
	for _, system := range node.OOBInventory.Systems {
		if system.Status != nil {
			fmt.Println(system.Status.State, system.Status.Health, system.Status.Conditions)
		}
	}
}
```

Use `DataSource` on `ListNodesOptions`, `DescribeNodeOptions`,
`NodeFilterOptionsRequest`, and `InventoryReportOptions`. This is a breaking
rename: `AgentType`, `NodeAgentType`, and its constants have been removed from
the public SDK. Use `NodeDataSourceInband` or `NodeDataSourceOOB`; node results
expose `DataSource` only.

With source omitted, SDK detail queries and inventory reports default to
in-band. Basic node queries combine both sources. The CLI separately retains
its default of fetching both detail views for node list, describe, and options
in table output. Detail JSON for CLI node list and options requires
`--data-source`. CLI inventory tables also fetch both sources by default,
while inventory JSON, CSV, and signed downloads require `--data-source`.
SDK defaults are unchanged.

```go
options, err := client.GetNodeFilterOptionsWithOptions(ctx, nvfleetint.NodeFilterOptionsRequest{
    View: nvfleetint.NodeViewBasic,
    DataSource: nvfleetint.NodeDataSourceOOB,
})
report, err := client.GetInventoryReport(ctx, nvfleetint.InventoryReportOptions{
    DataSource: nvfleetint.NodeDataSourceOOB,
    SortBy: nvfleetint.InventoryReportSortByBMCHostname,
})
```

OOB report rows expose `NodeName`, `BMCHostname`, and `BMCIP`, plus shared
verification, location, enrollment, and removal information. Source selection
also applies to CSV and signed bundle downloads. `GetNodeFilterOptions(ctx, dataSource)`
accepts `NodeDataSource` for detail-view callers.

OOB node list/detail results now retain optional `GPUType` and `GPUCount`.
Basic node queries support GPU filters and inventory sorting while returning
identity-only rows. Agent-status filters accept `NodeAgentOnline` and
`NodeAgentOffline`; the deprecated `NodeAgentUnknown` value is rejected.

OOB node list and detail results expose the inventory classification as
`Node.NodeKind`.

OOB inventory exposes node kinds (`unknown`, `compute`, `nvswitch`, and
`power_shelf`) and nested Redfish status conditions on systems, processors,
managers, chassis, PCIe devices, and firmware.

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

Alert timeline filter values and sorting choices are available through
`GetAlertTimelineFilterOptions`. Pass `true` for the active-alert view or
`false` for the historical view:

```go
options, err := client.GetAlertTimelineFilterOptions(ctx, true)
```

## Writes

`SetNodeTags` replaces a node's tags with the set given. It is a replacement
rather than a merge — a tag the node already carries that is absent from
`Tags` is removed, and an empty `Tags` clears them all:

```go
result, err := client.SetNodeTags(ctx, nodeUUID, nvfleetint.SetNodeTagsOptions{
	Tags: []string{"gpu-health", "burn_in"},
})
if err != nil {
	log.Fatal(err)
}
fmt.Println(result.Tags)
```

Tags are validated before the request is sent, so a malformed tag is named
locally instead of returning an opaque 400. Unlike the read methods, a failed
write is never retried.
