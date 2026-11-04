// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

// Verifies generated OOB inventory models are mapped into the public SDK models.
func TestOOBInventoryFromGenerated(t *testing.T) {
	status := &fleetapi.ModelsOobInventoryStatus{
		Health:       testPointer("Critical"),
		HealthRollup: testPointer("Warning"),
		State:        testPointer("Enabled"),
		Conditions: &[]fleetapi.ModelsOobInventoryCondition{{
			ConditionType: testPointer("Alert"),
			Message:       testPointer("One or more errors were detected."),
			MessageArgs:   &[]string{"GPU_SXM_1"},
			MessageId:     "ResourceEvent.1.0.ResourceErrorsDetected",
			OriginOfCondition: &fleetapi.ModelsOobInventoryReference{
				OdataId: "/redfish/v1/Chassis/HGX_GPU_SXM_1",
			},
			Resolution: testPointer("Replace the failed component."),
			Severity:   testPointer("Critical"),
			Timestamp:  testPointer("2026-07-23T17:20:43Z"),
		}},
	}
	processor := fleetapi.ModelsOobProcessor{
		Id:             "cpu-1",
		Model:          testPointer("Grace"),
		MaxSpeedMhz:    testPointer(3200),
		TotalCores:     testPointer(72),
		TotalThreads:   testPointer(144),
		ProcessorType:  testPointer("CPU"),
		Socket:         testPointer("CPU0"),
		InstructionSet: testPointer("ARM-A64"),
		Status:         status,
	}
	pcieDevice := fleetapi.ModelsOobPcieDevice{
		Id:              "gpu-1",
		DeviceType:      testPointer("GPU"),
		Manufacturer:    testPointer("NVIDIA"),
		Model:           testPointer("H100"),
		FirmwareVersion: testPointer("96.00.5E.00.01"),
		Status:          status,
	}
	inventory := &fleetapi.ModelsOobInventory{
		CollectedAt:   "2026-08-17T12:00:00Z",
		SchemaVersion: "inventory.v1alpha1",
		Source: fleetapi.ModelsOobSource{
			Address:        "192.0.2.10",
			Mac:            "00:11:22:33:44:55",
			RedfishVersion: testPointer("1.17.0"),
			ServiceUuid:    testPointer("service-1"),
			SourceType:     "redfish",
			Vendor:         testPointer("NVIDIA"),
		},
		Systems: &[]fleetapi.ModelsOobSystem{{
			Id:                "system-1",
			Model:             testPointer("GB200 NVL72"),
			ProcessorSummary:  &fleetapi.ModelsOobProcessorSummary{Count: testPointer(2), CoreCount: testPointer(144)},
			MemorySummary:     &fleetapi.ModelsOobMemorySummary{TotalSystemMemoryGiB: testPointer(float32(2048))},
			SecureBootEnabled: testPointer(true),
			Processors:        &[]fleetapi.ModelsOobProcessor{processor},
			Status:            status,
		}},
		Managers: &[]fleetapi.ModelsOobManager{{
			Id:              "manager-1",
			FirmwareVersion: testPointer("7.10.00.00"),
			ManagerType:     testPointer("BMC"),
			Status:          status,
		}},
		Chassis: &[]fleetapi.ModelsOobChassis{{
			Id:          "chassis-1",
			ChassisType: testPointer("RackMount"),
			Location: &fleetapi.ModelsOobChassisLocation{
				Placement: &fleetapi.ModelsOobPlacement{Rack: testPointer("R01"), RackOffset: testPointer(12)},
			},
			PcieDevices: &[]fleetapi.ModelsOobPcieDevice{pcieDevice},
			Status:      status,
		}},
		Firmware: &[]fleetapi.ModelsOobFirmware{{
			Id:        "bios-1",
			Name:      "System BIOS",
			ServiceId: "firmware-service-1",
			Version:   testPointer("1.2.3"),
			Status:    status,
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
		got.TargetError != "one target was unavailable" {
		t.Fatalf("unexpected inventory metadata: %#v", got)
	}
	if got.Source == nil || got.Source.Address != "192.0.2.10" ||
		got.Source.ServiceUUID != "service-1" {
		t.Fatalf("unexpected source: %#v", got.Source)
	}
	if len(got.Systems) != 1 || got.Systems[0].ProcessorSummary == nil || got.Systems[0].ProcessorSummary.Count == nil || *got.Systems[0].ProcessorSummary.Count != 2 ||
		len(got.Systems[0].Processors) != 1 || got.Systems[0].Processors[0].TotalCores == nil ||
		*got.Systems[0].Processors[0].TotalCores != 72 || got.Systems[0].Status == nil ||
		got.Systems[0].Status.Health != "Critical" || len(got.Systems[0].Status.Conditions) != 1 ||
		got.Systems[0].Status.Conditions[0].MessageID != "ResourceEvent.1.0.ResourceErrorsDetected" ||
		got.Systems[0].Status.Conditions[0].OriginOfCondition == nil ||
		got.Systems[0].Status.Conditions[0].OriginOfCondition.ODataID != "/redfish/v1/Chassis/HGX_GPU_SXM_1" {
		t.Fatalf("unexpected systems: %#v", got.Systems)
	}
	if len(got.Managers) != 1 || got.Managers[0].FirmwareVersion != "7.10.00.00" ||
		got.Managers[0].Status == nil {
		t.Fatalf("unexpected managers: %#v", got.Managers)
	}
	if len(got.Chassis) != 1 || got.Chassis[0].Location == nil || got.Chassis[0].Location.Placement == nil || got.Chassis[0].Location.Placement.RackOffset == nil ||
		*got.Chassis[0].Location.Placement.RackOffset != 12 || len(got.Chassis[0].PCIeDevices) != 1 ||
		got.Chassis[0].PCIeDevices[0].Model != "H100" || got.Chassis[0].Status == nil ||
		got.Chassis[0].PCIeDevices[0].Status == nil {
		t.Fatalf("unexpected chassis: %#v", got.Chassis)
	}
	if len(got.Firmware) != 1 || got.Firmware[0].ServiceID != "firmware-service-1" ||
		got.Firmware[0].Status == nil || got.Firmware[0].Status.HealthRollup != "Warning" {
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

// Populate every current field in the generated inventory contract so additions cannot
// silently disappear at the handwritten SDK boundary. Distinct values detect
// crossed mappings; zero numbers and false booleans detect lost presence.
func TestOOBInventoryCompleteContract(t *testing.T) {
	for _, zero := range []bool{false, true} {
		t.Run(map[bool]string{false: "populated", true: "zero-scalars"}[zero], func(t *testing.T) {
			var input fleetapi.ModelsOobInventory
			populateOOBContract(t, reflect.ValueOf(&input).Elem(), "inventory", zero)
			got := oobInventoryFromGenerated(&input)
			wantJSON := oobJSONValue(t, input)
			if actual := oobJSONValue(t, got); !reflect.DeepEqual(actual, wantJSON) {
				t.Fatalf("SDK inventory lost or changed contract fields\ngot: %#v\nwant: %#v", actual, wantJSON)
			}
			// Mutating a generated response must not alter the public SDK result.
			populateOOBContract(t, reflect.ValueOf(&input).Elem(), "changed", !zero)
			if actual := oobJSONValue(t, got); !reflect.DeepEqual(actual, wantJSON) {
				t.Fatal("SDK inventory aliases generated response data")
			}
		})
	}
}

func TestOOBInventoryAbsentFields(t *testing.T) {
	var input fleetapi.ModelsOobInventory
	if err := json.Unmarshal([]byte(`{
  "schemaVersion":"inventory.v1alpha1", "collectedAt":"2026-10-28T12:00:00Z",
  "source":{"sourceType":"redfish","address":"192.0.2.10","mac":"02:00:0a:00:00:0a"},
  "systems":[{"id":"system-1","processors":[{"id":"gpu-1"}],"memory":[{"id":"dimm-1","systemId":"system-1"}],"storage":[{"id":"storage-1","systemId":"system-1"}]}],
  "chassis":[{"id":"chassis-1","powerSupplies":[{"id":"psu-1","chassisId":"chassis-1"}]}],
  "fabrics":[{"id":"fabric-1","name":"NVLink","switches":[{"id":"switch-1","name":"Switch"}]}]
 }`), &input); err != nil {
		t.Fatal(err)
	}
	got := oobInventoryFromGenerated(&input)
	if !reflect.DeepEqual(oobJSONValue(t, got), oobJSONValue(t, input)) {
		t.Fatal("absent fields acquired values during conversion")
	}
	if got.Systems[0].Memory[0].CapacityMiB != nil || got.Chassis[0].PowerSupplies[0].PowerCapacityWatts != nil || got.Fabrics[0].Switches[0].NvidiaSwitch != nil {
		t.Fatal("absent optional values must remain nil")
	}
}

func TestOOBInventoryReferenceJSON(t *testing.T) {
	data, err := json.Marshal(OOBInventoryReference{ODataID: "/redfish/v1/Systems/1"})
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"odataId":"/redfish/v1/Systems/1"}` {
		t.Fatalf("incorrect reference key: %s", data)
	}
	var ref OOBInventoryReference
	if err := json.Unmarshal(data, &ref); err != nil || ref.ODataID != "/redfish/v1/Systems/1" {
		t.Fatalf("reference round trip: %#v, %v", ref, err)
	}
}

func populateOOBContract(t *testing.T, value reflect.Value, path string, zero bool) {
	t.Helper()
	switch value.Kind() {
	case reflect.Pointer:
		if value.IsNil() {
			value.Set(reflect.New(value.Type().Elem()))
		}
		populateOOBContract(t, value.Elem(), path, zero)
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			populateOOBContract(t, value.Field(i), path+"."+value.Type().Field(i).Name, zero)
		}
	case reflect.Slice:
		if value.Len() == 0 {
			value.Set(reflect.MakeSlice(value.Type(), 1, 1))
		}
		populateOOBContract(t, value.Index(0), path+"[0]", zero)
	case reflect.Map:
		if value.IsNil() {
			value.Set(reflect.MakeMap(value.Type()))
		}
		entry := value.MapIndex(reflect.ValueOf("Related"))
		if !entry.IsValid() {
			entry = reflect.New(value.Type().Elem()).Elem()
		}
		populateOOBContract(t, entry, path+".Related", zero)
		value.SetMapIndex(reflect.ValueOf("Related"), entry)
	case reflect.String:
		value.SetString(path)
	case reflect.Int:
		if zero {
			value.SetInt(0)
		} else {
			value.SetInt(int64(len(path)))
		}
	case reflect.Float32:
		if zero {
			value.SetFloat(0)
		} else {
			value.SetFloat(float64(len(path)) + .5)
		}
	case reflect.Bool:
		value.SetBool(!zero)
	default:
		t.Fatalf("unhandled inventory field %s: %s", path, value.Kind())
	}
}

func oobJSONValue(t *testing.T, value any) any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	return decoded
}
