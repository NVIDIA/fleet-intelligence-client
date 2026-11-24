// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"bytes"

	"github.com/spf13/cobra"
)

const completeOOBNodeJSON = `{
 "nodeUUID":"node-1", "dataSource":"oob", "nodeKind":"nvswitch",
 "oobInventory":{
  "schemaVersion":"inventory.v1alpha1", "collectedAt":"2026-10-28T12:00:00Z",
  "source":{"sourceType":"redfish","address":"192.0.2.10","mac":"02:00:0a:00:00:0a"},
  "systems":[{
   "id":"system-1","status":{"state":"Enabled","health":"Critical","healthRollup":"Warning"},
   "memorySummary":{"totalSystemMemoryGiB":0},
   "processorSummary":{"count":0,"coreCount":0,"model":"Grace"},
   "processors":[{"id":"gpu-1","processorType":"GPU","status":{"health":"Warning"},"nvidiaGpu":{"totalNvLinks":18,"migModeEnabled":false,"nvlinkTopology":{"hostId":0,"ibGuid":"0x1234","traySerialNumber":"NA"}},"location":{"partLocation":{"serviceLabel":"GPU slot 1"}}},
    {"id":"cpu-1","processorType":"CPU","status":{"health":"Warning"}}],
   "memory":[{"id":"dimm-1","systemId":"system-1","deviceLocator":"DIMM A1","capacityMib":65536,"status":{"health":"OK"},"location":{"partLocation":{"serviceLabel":"memory slot 1"}}}],
   "storage":[{"id":"storage-1","systemId":"system-1","drives":[{"id":"drive-1","storageId":"storage-1","systemId":"system-1","capacityBytes":4000000000000,"protocol":"NVMe","status":{"health":"OK"}}]}],
   "links":{"Chassis":[{"odataId":"/redfish/v1/Chassis/chassis-1"}]}
  }],
  "managers":[{"id":"manager-1","status":{"health":"OK"}}],
  "chassis":[{
   "id":"chassis-1","status":{"health":"Warning"},
   "location":{"placement":{"rack":"R01","rackOffset":0,"rackOffsetUnits":"EIA_310"},"partLocation":{"serviceLabel":"compute tray"}},
   "pcieDevices":[{"id":"pcie-1","status":{"health":"OK"}}],
   "powerSupplies":[{"id":"psu-1","chassisId":"chassis-1","powerCapacityWatts":2400,"status":{"health":"OK"}}]
  }],
  "fabrics":[{"id":"fabric-1","name":"NVLink Fabric","fabricType":"NVLink","status":{"health":"OK"},"switches":[
   {"id":"switch-1","name":"NVSwitch","status":{"health":"Warning"},"location":{"placement":{"rack":"R02","rackOffset":10}},"nvidiaSwitch":{"pcieReferenceClockEnabled":false,"deviceId":"0x1234"}}
  ]}],
  "firmware":[{"id":"firmware-1","name":"Firmware","serviceId":"service-1","status":{"health":"OK"},"relatedItem":[{"odataId":"/redfish/v1/Systems/system-1"}]}],
  "futureField":{"preserved":true}
 }
}`

// newRootCmd builds a root command carrying only this package's commands, so
// the tests drive them through the same argument path a user types.
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "nvfleetint",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(NewCmd())
	return root
}

// nodeRenderWriter records actual writes so failures can target a boundary
// even when tabwriter emits a table in multiple writes.
type nodeRenderWriter struct {
	output bytes.Buffer
	writes []string
	failAt int
	err    error
	after  int
}

func (w *nodeRenderWriter) Write(p []byte) (int, error) {
	if w.failAt >= 0 && len(w.writes) >= w.failAt {
		if len(w.writes) > w.failAt {
			w.after++
		}
		w.writes = append(w.writes, string(p))
		return 0, w.err
	}
	w.writes = append(w.writes, string(p))
	return w.output.Write(p)
}

type nodeSectionFailureWriter struct {
	accepted bytes.Buffer
	heading  string
	err      error
	reached  bool
	after    int
}

func (w *nodeSectionFailureWriter) Write(p []byte) (int, error) {
	if w.reached {
		w.after++
		return 0, w.err
	}
	if bytes.Contains(p, []byte(w.heading)) {
		w.reached = true
		return 0, w.err
	}
	return w.accepted.Write(p)
}
