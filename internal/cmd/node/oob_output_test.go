// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package node

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdtest"
	"github.com/NVIDIA/fleet-intelligence-client/nvfleetint"
)

// Verifies OOB node describe defaults to a summary and supports domain drill-downs
func TestNodeDescribeOOBTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("dataSource"); got != "oob" {
			t.Fatalf("unexpected dataSource: %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"nodeUUID":"node-oob-1",
			"nodeKind":"nvswitch",
			"hostname":"host-001",
			"dataSource":"oob",
			"bmcMac":"bmc-001",
			"bmcIP":"192.0.2.10",
			"oobInventory":{
				"collectedAt":"2026-07-30T20:00:00Z",
				"schemaVersion":"inventory.v1alpha1",
				"source":{"sourceType":"redfish","vendor":"Dell","address":"192.0.2.10:443","mac":"00:11:22:33:44:55","redfishVersion":"1.17.0"},
				"systems":[{"id":"System.Embedded.1","uuid":"system-uuid","manufacturer":"Dell","model":"PowerEdge XE9680","sku":"sku-1","serialNumber":"serial-1","biosVersion":"2.1.0","hostName":"host-001","assetTag":"asset-1","powerState":"on","status":{"state":"Enabled","health":"Critical","healthRollup":"Warning","conditions":[{"messageId":"ResourceEvent.1.0.ResourceErrorsDetected","severity":"Critical","message":"nested status condition"}]},"processorSummary":{"count":2},"memorySummary":{"totalSystemMemoryGiB":2048},"secureBootEnabled":true,"processors":[{"id":"CPU.Socket.1","socket":"CPU 1","processorType":"cpu","processorArchitecture":"x86","manufacturer":"Intel","model":"Xeon","maxSpeedMhz":3800,"totalCores":56,"totalThreads":112,"status":{"state":"Enabled","health":"OK","healthRollup":"OK"}},{"id":"GPU.Slot.1","processorType":"gpu","manufacturer":"NVIDIA","model":"NVIDIA H100","status":{"state":"Enabled","health":"OK","healthRollup":"OK"}}]}],
				"managers":[{"id":"iDRAC.Embedded.1","uuid":"manager-uuid","model":"iDRAC","managerType":"bmc","firmwareVersion":"7.10.00.00","status":{"state":"Enabled","health":"OK","healthRollup":"OK"}}],
				"chassis":[{"id":"System.Embedded.1","chassisType":"rack_mount","manufacturer":"Dell","model":"XE9680","sku":"sku-1","serialNumber":"serial-1","partNumber":"part-1","assetTag":"asset-1","powerState":"on","status":{"state":"Enabled","health":"OK","healthRollup":"Warning"},"pcieDevices":[{"id":"GPU.Slot.1","uuid":"gpu-uuid","deviceType":"single_function","manufacturer":"NVIDIA","model":"NVIDIA H100","sku":"gpu-sku","serialNumber":"gpu-serial","partNumber":"gpu-part","firmwareVersion":"96.00.00","status":{"state":"Enabled","health":"OK","healthRollup":"OK"}}]}],
				"firmware":[{"id":"BIOS","name":"System BIOS","serviceId":"fw-service","version":"1.2.3","releaseDate":"2026-01-01","status":{"state":"Enabled","health":"OK","healthRollup":"Warning"}}],
				"domainErrors":[{"domain":"storage","resource":"Disk.1","message":"collection failed"}]
			}
		}`))
	}))
	defer server.Close()

	cmdtest.SaveConfig(t, server.URL, "test-key")

	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "describe", "node-oob-1", "--data-source", "oob"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("command failed: %v", err)
	}

	got := out.String()
	for _, want := range []string{
		"FIELD", "VALUE",
		"BMC MAC", "bmc-001",
		"INVENTORY SCHEMA VERSION", "inventory.v1alpha1",
		"NODE KIND", "nvswitch",
		"SOURCE ADDRESS", "192.0.2.10:443", "SOURCE MAC", "00:11:22:33:44:55", "SOURCE VENDOR", "Dell",
		"INVENTORY DOMAIN ERROR 1", "storage: Disk.1: collection failed",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %q", want, got)
		}
	}
	for _, unwanted := range []string{
		"\nHOSTNAME ", "\nAGENT TYPE ",
		"\nFIRMWARE CHECK ", "\nPUBLIC IP ", "\nPRIVATE IP ",
		"nested status condition", "ResourceEvent.1.0.ResourceErrorsDetected",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("OOB summary unexpectedly contains %q: %q", unwanted, got)
		}
	}
	for _, unwanted := range []string{"\nManagers\n", "\nSystems\n", "\nChassis\n", "\nFirmware\n"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("default output unexpectedly contains %q: %q", unwanted, got)
		}
	}

	out.Reset()
	cmd = newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{
		"node", "describe", "node-oob-1", "--data-source", "oob",
		"--section", "systems,chassis",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("section command failed: %v", err)
	}

	got = out.String()
	for _, want := range []string{
		"\nSystems\n", "PowerEdge XE9680", "2048",
		"system-uuid", "sku-1", "2.1.0", "asset-1", "SECURE BOOT", "true", "ROLLUP", "Warning",
		"\nCPUs\n", "CPU.Socket.1", "x86", "3800", "56", "112",
		"\nGPUs\n", "GPU.Slot.1", "NVIDIA H100",
		"\nChassis\n", "serial-1",
		"part-1", "\nPCIe Devices\n", "gpu-uuid", "single_function", "gpu-part", "96.00.00",
		"Domain Errors", "collection failed",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("section output missing %q: %q", want, got)
		}
	}
	for _, unwanted := range []string{"\nManagers\n", "\nFirmware\n", "ODATA ID", "HEALTH ROLLUP"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("section output unexpectedly contains %q: %q", unwanted, got)
		}
	}
	for _, unwanted := range []string{
		"FIELD", "INVENTORY SCHEMA VERSION", "SOURCE ADDRESS", "BMC MAC",
	} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("section output unexpectedly contains summary value %q: %q", unwanted, got)
		}
	}

	out.Reset()
	cmd = newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{
		"node", "describe", "node-oob-1", "--data-source", "oob", "--section", "all",
	})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("all sections command failed: %v", err)
	}

	got = out.String()
	for _, want := range []string{
		"FIELD", "INVENTORY SCHEMA VERSION", "SOURCE ADDRESS",
		"NODE KIND", "nvswitch",
		"\nManagers\n", "iDRAC.Embedded.1", "manager-uuid", "7.10.00.00",
		"\nSystems\n", "\nCPUs\n", "\nGPUs\n",
		"\nChassis\n", "\nPCIe Devices\n",
		"\nFirmware\n", "System BIOS", "1.2.3", "2026-01-01", "Enabled", "OK", "Warning",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("all sections output missing %q: %q", want, got)
		}
	}
	if strings.Contains(got, "nested status condition") ||
		strings.Contains(got, "ResourceEvent.1.0.ResourceErrorsDetected") {
		t.Fatalf("table output unexpectedly contains nested status details: %q", got)
	}

	out.Reset()
	cmd = newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "describe", "node-oob-1", "--data-source", "oob", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("JSON command failed: %v", err)
	}
	jsonOutput := out.String()
	for _, want := range []string{
		`"nodeKind":"nvswitch"`,
		`"status":{`,
		`"messageId":"ResourceEvent.1.0.ResourceErrorsDetected"`,
		`"message":"nested status condition"`,
	} {
		if !strings.Contains(jsonOutput, want) {
			t.Fatalf("JSON output missing %q: %q", want, jsonOutput)
		}
	}
}

func TestOOBDetailNodeRowsOmitInbandHostname(t *testing.T) {
	rows := oobDetailNodeRows([]nvfleetint.Node{{
		UUID:     "node-1",
		Hostname: "inband-hostname",
		NodeName: "machine-001",
		NodeKind: nvfleetint.NodeKindNVSwitch,
		BMCMAC:   "bmc-001",
		BMCIP:    "192.0.2.10:443",
	}})

	if len(rows) != 1 || len(rows[0]) != 12 {
		t.Fatalf("unexpected OOB row shape: %#v", rows)
	}
	if slices.Contains(rows[0], "inband-hostname") {
		t.Fatalf("OOB row contains in-band hostname: %#v", rows[0])
	}
	if rows[0][1] != "machine-001" || rows[0][2] != "nvswitch" || rows[0][3] != "bmc-001" || rows[0][4] != "192.0.2.10:443" {
		t.Fatalf("unexpected OOB identity columns: %#v", rows[0])
	}
}

func TestOOBNodeDescribeRowsOmitInbandFields(t *testing.T) {
	rows := oobNodeDescribeRows(nvfleetint.NodeDetails{
		Node: nvfleetint.Node{
			UUID:          "node-1",
			Hostname:      "inband-hostname",
			NodeName:      "machine-001",
			DataSource:    "oob",
			GPUType:       "NVIDIA H100",
			FirmwareCheck: "Passed",
			PublicIP:      "192.0.2.20",
			PrivateIP:     "10.0.0.20",
		},
	})

	labels := make([]string, 0, len(rows))
	for _, row := range rows {
		labels = append(labels, row[0])
	}
	for _, unwanted := range []string{
		"HOSTNAME", "AGENT TYPE",
		"FIRMWARE CHECK", "PUBLIC IP", "PRIVATE IP",
	} {
		if slices.Contains(labels, unwanted) {
			t.Fatalf("OOB summary contains in-band field %q: %#v", unwanted, labels)
		}
	}
	if !slices.Contains(labels, "NODE NAME") {
		t.Fatalf("OOB summary missing node name: %#v", labels)
	}
}

func TestNodeDescribeCompleteOOBInventory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", os.Getenv("HOME"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("dataSource") != "oob" {
			t.Errorf("unexpected source: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(completeOOBNodeJSON))
	}))
	defer server.Close()
	cmdtest.SaveConfig(t, server.URL, "test-key")
	for _, tc := range []struct {
		name, section string
		want, absent  []string
	}{
		{"summary", "", []string{"INVENTORY MEMORY MODULES", "INVENTORY STORAGE", "INVENTORY DRIVES", "INVENTORY POWER SUPPLIES", "INVENTORY FABRICS", "INVENTORY SWITCHES"}, []string{"\nSystems\n", "\nFabrics\n"}},
		{"systems", "systems", []string{"\nMemory Modules\n", "DIMM A1", "65536", "\nStorage\n", "\nDrives\n", "drive-1", "4000000000000", "CPU COUNT", "Grace", "Critical"}, []string{"\nPower Supplies\n", "\nFabrics\n"}},
		{"chassis", "chassis", []string{"\nPower Supplies\n", "psu-1", "2400"}, []string{"\nSystems\n", "\nFabrics\n"}},
		{"fabrics", "fabrics", []string{"\nFabrics\n", "\nSwitches\n", "fabric-1", "switch-1", "NVSwitch", "Warning"}, []string{"\nSystems\n", "\nChassis\n"}},
		{"all", "all", []string{"\nSystems\n", "\nMemory Modules\n", "\nDrives\n", "\nPower Supplies\n", "\nFabrics\n", "\nSwitches\n", "\nFirmware\n", "INVENTORY SWITCHES"}, []string{"PRIMARY"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			args := []string{"node", "describe", "node-1", "--data-source", "oob"}
			if tc.section != "" {
				args = append(args, "--section", tc.section)
			}
			cmd.SetArgs(args)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("missing %q in %s", want, &out)
				}
			}
			for _, absent := range append(tc.absent, "  LOCATION", "GPU slot 1", "memory slot 1", "compute tray", "rack R01 U0", "rack R02 U10") {
				if strings.Contains(out.String(), absent) {
					t.Errorf("unexpected %q in %s", absent, &out)
				}
			}
		})
	}
	var out bytes.Buffer
	cmd := newRootCmd()
	cmd.SetOut(&out)
	cmd.SetArgs([]string{"node", "describe", "node-1", "--data-source", "oob", "--output", "json"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var got, want any
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(completeOOBNodeJSON), &want); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatal("raw JSON changed, including unknown fields or references")
	}
}

// Absent nested fields stay empty; explicit zero values remain visible.
func TestOOBTablesHandleAbsentAndZeroNestedFields(t *testing.T) {
	var inventory nvfleetint.OOBInventory
	if err := json.Unmarshal([]byte(`{"systems":[{"id":"system-1"}]}`), &inventory); err != nil {
		t.Fatal(err)
	}
	row := oobSystemRows(&inventory)[0]
	for _, column := range []int{10, 11, 12, 13, 15, 16, 17} {
		if row[column] != "" {
			t.Fatalf("absent nested column %d = %q", column, row[column])
		}
	}
	zeroMemory, zeroCount := float32(0), 0
	inventory.Systems[0].Status = &nvfleetint.OOBInventoryStatus{Health: "OK"}
	inventory.Systems[0].MemorySummary = &nvfleetint.OOBMemorySummary{TotalSystemMemoryGiB: &zeroMemory}
	inventory.Systems[0].ProcessorSummary = &nvfleetint.OOBProcessorSummary{Count: &zeroCount}
	row = oobSystemRows(&inventory)[0]
	if row[11] != "OK" || row[13] != "0" || row[15] != "0" || row[10] != "" || row[12] != "" || row[16] != "" || row[17] != "" {
		t.Fatalf("partial canonical values rendered incorrectly: %v", row)
	}
}

func TestOOBInventorySummaryCounts(t *testing.T) {
	inventory := nvfleetint.OOBInventory{
		Systems: []nvfleetint.OOBSystem{
			{Memory: make([]nvfleetint.OOBMemory, 2), Storage: []nvfleetint.OOBStorage{{Drives: make([]nvfleetint.OOBDrive, 2)}, {}}},
			{Memory: make([]nvfleetint.OOBMemory, 3), Storage: []nvfleetint.OOBStorage{{Drives: make([]nvfleetint.OOBDrive, 4)}}},
		},
		Chassis: []nvfleetint.OOBChassis{{PowerSupplies: make([]nvfleetint.OOBPowerSupply, 2)}, {PowerSupplies: make([]nvfleetint.OOBPowerSupply, 1)}},
		Fabrics: []nvfleetint.OOBFabric{{Switches: make([]nvfleetint.OOBSwitch, 2)}, {Switches: make([]nvfleetint.OOBSwitch, 4)}},
	}
	got := map[string]string{}
	for _, row := range oobInventorySummaryRows(&inventory) {
		got[row[0]] = row[1]
	}
	for name, want := range map[string]string{"MEMORY MODULES": "5", "STORAGE": "3", "DRIVES": "6", "POWER SUPPLIES": "3", "FABRICS": "2", "SWITCHES": "6"} {
		if got["INVENTORY "+name] != want {
			t.Errorf("%s count = %s, want %s", name, got["INVENTORY "+name], want)
		}
	}
}

func TestOOBEmptyFabricSection(t *testing.T) {
	options, err := parseOOBInventorySections(nodeDescribeFlags{sections: "fabrics"}, nvfleetint.DataSourceOOB, "table")
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := writeOOBInventoryTables(&out, &nvfleetint.OOBInventory{}, options); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Fabrics") || strings.Contains(out.String(), "\nSwitches\n") {
		t.Fatalf("unexpected empty section: %s", &out)
	}
	for _, tc := range []struct {
		source nvfleetint.DataSource
		output string
	}{{nvfleetint.DataSourceInband, "table"}, {nvfleetint.DataSourceOOB, "json"}} {
		if _, err := parseOOBInventorySections(nodeDescribeFlags{sections: "fabrics"}, tc.source, tc.output); err == nil {
			t.Fatal("fabric section accepted invalid source/output combination")
		}
	}
}

func TestNodeDescribeOOBSectionWriteErrors(t *testing.T) {
	var oob nvfleetint.NodeDetails
	if err := json.Unmarshal([]byte(completeOOBNodeJSON), &oob); err != nil {
		t.Fatalf("decode OOB fixture: %v", err)
	}
	if oob.OOBInventory == nil {
		t.Fatal("OOB fixture has no inventory")
	}
	options, err := parseOOBInventorySections(nodeDescribeFlags{sections: "all"}, nvfleetint.DataSourceOOB, "table")
	if err != nil {
		t.Fatalf("parse sections: %v", err)
	}

	titles := []string{"Managers", "Systems", "CPUs", "GPUs", "Memory Modules", "Chassis", "PCIe Devices", "Power Supplies", "Fabrics", "Switches", "Firmware"}
	result := nodeDescribeResult{OOB: &oob}
	var control bytes.Buffer
	if err := writeNodeDescribeResultTable(&control, result, nvfleetint.DataSourceOOB, options); err != nil {
		t.Fatalf("render OOB control: %v", err)
	}
	for _, want := range append(append([]string{}, titles...), "manager-1", "system-1", "cpu-1", "gpu-1", "dimm-1", "chassis-1", "pcie-1", "psu-1", "fabric-1", "switch-1", "firmware-1") {
		if !strings.Contains(control.String(), want) {
			t.Errorf("OOB control missing %q", want)
		}
	}

	checkFailure := func(t *testing.T, result nodeDescribeResult, source nvfleetint.DataSource, expected, title string) {
		t.Helper()
		heading := "\n" + title + "\n"
		index := strings.Index(expected, heading)
		if index < 0 {
			t.Fatalf("control missing heading %q", heading)
		}
		sentinel := errors.New("section write failed")
		writer := &nodeSectionFailureWriter{heading: heading, err: sentinel}
		err := writeNodeDescribeResultTable(writer, result, source, options)
		if !errors.Is(err, sentinel) {
			t.Errorf("error = %v, want sentinel", err)
		}
		if !writer.reached {
			t.Errorf("writer did not reach heading %q", heading)
		}
		if writer.after != 0 {
			t.Errorf("writer received %d writes after failure", writer.after)
		}
		if got := writer.accepted.String(); got != expected[:index] {
			t.Errorf("accepted output differs from control prefix before %q: got %q", title, got)
		}
	}
	for _, title := range titles {
		t.Run(title, func(t *testing.T) {
			checkFailure(t, result, nvfleetint.DataSourceOOB, control.String(), title)
		})
	}

	t.Run("combined sources", func(t *testing.T) {
		inband := nvfleetint.NodeDetails{}
		inband.DataSource = string(nvfleetint.DataSourceInband)
		combined := nodeDescribeResult{Inband: &inband, OOB: &oob}
		var combinedControl bytes.Buffer
		if err := writeNodeDescribeResultTable(&combinedControl, combined, "", options); err != nil {
			t.Fatalf("render combined control: %v", err)
		}
		checkFailure(t, combined, "", combinedControl.String(), "Switches")
	})
}

func TestNodeDescribeCombinedSelectedInventorySections(t *testing.T) {
	for _, tc := range []struct {
		name         string
		inbandStatus int
	}{
		{name: "inband present", inbandStatus: http.StatusOK},
		{name: "inband missing", inbandStatus: http.StatusNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			t.Setenv("USERPROFILE", os.Getenv("HOME"))
			var mu sync.Mutex
			counts := map[string]int{"inband": 0, "oob": 0}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != "/v1/nodes/node-1" {
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected request", http.StatusBadRequest)
					return
				}
				sources := r.URL.Query()["dataSource"]
				if len(sources) != 1 || (sources[0] != "inband" && sources[0] != "oob") {
					t.Errorf("unexpected dataSource query: %q", sources)
					http.Error(w, "unexpected source", http.StatusBadRequest)
					return
				}
				source := sources[0]
				mu.Lock()
				counts[source]++
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if source == "oob" {
					_, _ = w.Write([]byte(completeOOBNodeJSON))
				} else if tc.inbandStatus == http.StatusNotFound {
					w.WriteHeader(http.StatusNotFound)
					_, _ = w.Write([]byte(`{"error":"source not found"}`))
				} else {
					_, _ = w.Write([]byte(`{"nodeUUID":"node-1","dataSource":"inband","hostname":"distinctive-inband-host"}`))
				}
			}))
			defer server.Close()
			cmdtest.SaveConfig(t, server.URL, "test-key")

			var out bytes.Buffer
			cmd := newRootCmd()
			cmd.SetOut(&out)
			cmd.SetArgs([]string{"node", "describe", "node-1", "--section", "systems,chassis"})
			if err := cmd.Execute(); err != nil {
				t.Fatalf("command failed: %v", err)
			}
			mu.Lock()
			for _, source := range []string{"inband", "oob"} {
				if counts[source] != 1 {
					t.Errorf("%s requests = %d, want 1", source, counts[source])
				}
			}
			mu.Unlock()
			got := out.String()
			for _, want := range []string{"Systems", "system-1", "CPUs", "cpu-1", "GPUs", "gpu-1", "Chassis", "chassis-1"} {
				if !strings.Contains(got, want) {
					t.Errorf("output missing %q: %q", want, got)
				}
			}
			for _, unwanted := range []string{"distinctive-inband-host", "In-band", "Out-of-band"} {
				if strings.Contains(got, unwanted) {
					t.Errorf("output contains %q: %q", unwanted, got)
				}
			}
			for _, line := range strings.Split(got, "\n") {
				fields := strings.Fields(line)
				if len(fields) > 0 && fields[0] == "FIELD" {
					t.Errorf("output contains summary header: %q", line)
				}
				for _, heading := range []string{"Managers", "Firmware", "Fabrics"} {
					if strings.TrimSpace(line) == heading {
						t.Errorf("output contains unselected heading %q", heading)
					}
				}
			}
		})
	}
}
