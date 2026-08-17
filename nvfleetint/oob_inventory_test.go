// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

// Verifies generated OOB inventory models are mapped into the public SDK models.
func TestOOBInventoryFromGenerated(t *testing.T) {
	processor := fleetapi.ModelsOobProcessor{
		Id:             "cpu-1",
		Model:          testPointer("Grace"),
		MaxSpeedMhz:    testPointer(3200),
		TotalCores:     testPointer(72),
		TotalThreads:   testPointer(144),
		StatusState:    testPointer("Enabled"),
		Health:         testPointer("OK"),
		HealthRollup:   testPointer("OK"),
		ProcessorType:  testPointer("CPU"),
		Socket:         testPointer("CPU0"),
		InstructionSet: testPointer("ARM-A64"),
	}
	pcieDevice := fleetapi.ModelsOobPcieDevice{
		Id:              "gpu-1",
		DeviceType:      testPointer("GPU"),
		Manufacturer:    testPointer("NVIDIA"),
		Model:           testPointer("H100"),
		FirmwareVersion: testPointer("96.00.5E.00.01"),
		StatusState:     testPointer("Enabled"),
		Health:          testPointer("OK"),
	}
	inventory := &fleetapi.ModelsOobInventory{
		CollectedAt:     "2026-08-17T12:00:00Z",
		SchemaVersion:   "inventory.v1alpha1",
		PrimarySystemId: testPointer("system-1"),
		Source: fleetapi.ModelsOobSource{
			Address:        "192.0.2.10",
			HostName:       testPointer("bmc-1"),
			Mac:            "00:11:22:33:44:55",
			RedfishVersion: testPointer("1.17.0"),
			ServiceUuid:    testPointer("service-1"),
			SourceType:     "redfish",
			Vendor:         testPointer("NVIDIA"),
		},
		Systems: &[]fleetapi.ModelsOobSystem{{
			Id:                "system-1",
			Model:             testPointer("GB200 NVL72"),
			CpuCount:          testPointer(2),
			CpuCoreCount:      testPointer(144),
			MemoryGib:         testPointer(float32(2048)),
			SecureBootEnabled: testPointer(true),
			Processors:        &[]fleetapi.ModelsOobProcessor{processor},
		}},
		Managers: &[]fleetapi.ModelsOobManager{{
			Id:              "manager-1",
			FirmwareVersion: testPointer("7.10.00.00"),
			ManagerType:     testPointer("BMC"),
			StatusState:     testPointer("Enabled"),
		}},
		Chassis: &[]fleetapi.ModelsOobChassis{{
			Id:          "chassis-1",
			ChassisType: testPointer("RackMount"),
			Location: &fleetapi.ModelsOobChassisLocation{
				Rack:       testPointer("R01"),
				RackOffset: testPointer(12),
			},
			PcieDevices: &[]fleetapi.ModelsOobPcieDevice{pcieDevice},
		}},
		Firmware: &[]fleetapi.ModelsOobFirmware{{
			Id:           "bios-1",
			Name:         "System BIOS",
			ServiceId:    "firmware-service-1",
			Version:      testPointer("1.2.3"),
			StatusState:  testPointer("Enabled"),
			Health:       testPointer("OK"),
			HealthRollup: testPointer("Warning"),
		}},
		DomainErrors: &[]fleetapi.ModelsOobDomainError{{
			Domain:   "firmware",
			Message:  "partial collection",
			Resource: testPointer("/redfish/v1/UpdateService/FirmwareInventory"),
		}},
		TargetError: testPointer("one target was unavailable"),
	}

	got := oobInventoryFromGenerated(inventory)
	if got == nil || got.CollectedAt != inventory.CollectedAt || got.SchemaVersion != inventory.SchemaVersion ||
		got.PrimarySystemID != "system-1" || got.TargetError != "one target was unavailable" {
		t.Fatalf("unexpected inventory metadata: %#v", got)
	}
	if got.Source == nil || got.Source.Address != "192.0.2.10" || got.Source.Hostname != "bmc-1" ||
		got.Source.ServiceUUID != "service-1" {
		t.Fatalf("unexpected source: %#v", got.Source)
	}
	if len(got.Systems) != 1 || got.Systems[0].CPUCount == nil || *got.Systems[0].CPUCount != 2 ||
		len(got.Systems[0].Processors) != 1 || got.Systems[0].Processors[0].TotalCores == nil ||
		*got.Systems[0].Processors[0].TotalCores != 72 {
		t.Fatalf("unexpected systems: %#v", got.Systems)
	}
	if len(got.Managers) != 1 || got.Managers[0].FirmwareVersion != "7.10.00.00" {
		t.Fatalf("unexpected managers: %#v", got.Managers)
	}
	if len(got.Chassis) != 1 || got.Chassis[0].Location == nil || got.Chassis[0].Location.RackOffset == nil ||
		*got.Chassis[0].Location.RackOffset != 12 || len(got.Chassis[0].PCIeDevices) != 1 ||
		got.Chassis[0].PCIeDevices[0].Model != "H100" {
		t.Fatalf("unexpected chassis: %#v", got.Chassis)
	}
	if len(got.Firmware) != 1 || got.Firmware[0].ServiceID != "firmware-service-1" ||
		got.Firmware[0].HealthRollup != "Warning" {
		t.Fatalf("unexpected firmware: %#v", got.Firmware)
	}
	if len(got.DomainErrors) != 1 || got.DomainErrors[0].Domain != "firmware" ||
		got.DomainErrors[0].Resource != "/redfish/v1/UpdateService/FirmwareInventory" {
		t.Fatalf("unexpected domain errors: %#v", got.DomainErrors)
	}
}

func TestOOBInventoryFromGeneratedNil(t *testing.T) {
	if got := oobInventoryFromGenerated(nil); got != nil {
		t.Fatalf("expected nil inventory, got %#v", got)
	}
}

func testPointer[T any](value T) *T {
	return &value
}
