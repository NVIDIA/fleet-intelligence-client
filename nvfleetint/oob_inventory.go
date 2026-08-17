// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import "github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"

// Represents inventory collected out of band through a node's BMC
type OOBInventory struct {
	Chassis         []OOBChassis     `json:"chassis,omitempty"`
	CollectedAt     string           `json:"collectedAt"`
	DomainErrors    []OOBDomainError `json:"domainErrors,omitempty"`
	Firmware        []OOBFirmware    `json:"firmware,omitempty"`
	Managers        []OOBManager     `json:"managers,omitempty"`
	PrimarySystemID string           `json:"primarySystemId,omitempty"`
	SchemaVersion   string           `json:"schemaVersion"`
	Source          *OOBSource       `json:"source,omitempty"`
	Systems         []OOBSystem      `json:"systems,omitempty"`
	TargetError     string           `json:"targetError,omitempty"`
}

// Identifies the source used to collect OOB inventory
type OOBSource struct {
	Address        string `json:"address,omitempty"`
	Hostname       string `json:"hostName,omitempty"`
	MAC            string `json:"mac,omitempty"`
	RedfishVersion string `json:"redfishVersion,omitempty"`
	ServiceUUID    string `json:"serviceUuid,omitempty"`
	SourceType     string `json:"sourceType,omitempty"`
	Vendor         string `json:"vendor,omitempty"`
}

// Represents a computer system reported by the BMC
type OOBSystem struct {
	AssetTag          string         `json:"assetTag,omitempty"`
	BIOSVersion       string         `json:"biosVersion,omitempty"`
	CPUCoreCount      *int           `json:"cpuCoreCount,omitempty"`
	CPUCount          *int           `json:"cpuCount,omitempty"`
	CPUModel          string         `json:"cpuModel,omitempty"`
	Health            string         `json:"health,omitempty"`
	HealthRollup      string         `json:"healthRollup,omitempty"`
	Hostname          string         `json:"hostName,omitempty"`
	ID                string         `json:"id,omitempty"`
	Manufacturer      string         `json:"manufacturer,omitempty"`
	MemoryGiB         *float32       `json:"memoryGib,omitempty"`
	Model             string         `json:"model,omitempty"`
	ODataID           string         `json:"odataId,omitempty"`
	PowerState        string         `json:"powerState,omitempty"`
	Processors        []OOBProcessor `json:"processors,omitempty"`
	SecureBootEnabled *bool          `json:"secureBootEnabled,omitempty"`
	SerialNumber      string         `json:"serialNumber,omitempty"`
	SKU               string         `json:"sku,omitempty"`
	StatusState       string         `json:"statusState,omitempty"`
	UUID              string         `json:"uuid,omitempty"`
}

// Represents a processor reported by the BMC
type OOBProcessor struct {
	Health                string `json:"health,omitempty"`
	HealthRollup          string `json:"healthRollup,omitempty"`
	ID                    string `json:"id,omitempty"`
	InstructionSet        string `json:"instructionSet,omitempty"`
	Manufacturer          string `json:"manufacturer,omitempty"`
	MaxSpeedMHz           *int   `json:"maxSpeedMhz,omitempty"`
	Model                 string `json:"model,omitempty"`
	ODataID               string `json:"odataId,omitempty"`
	ProcessorArchitecture string `json:"processorArchitecture,omitempty"`
	ProcessorType         string `json:"processorType,omitempty"`
	Socket                string `json:"socket,omitempty"`
	StatusState           string `json:"statusState,omitempty"`
	TotalCores            *int   `json:"totalCores,omitempty"`
	TotalThreads          *int   `json:"totalThreads,omitempty"`
}

// Represents a BMC manager
type OOBManager struct {
	FirmwareVersion string `json:"firmwareVersion,omitempty"`
	Health          string `json:"health,omitempty"`
	HealthRollup    string `json:"healthRollup,omitempty"`
	ID              string `json:"id,omitempty"`
	ManagerType     string `json:"managerType,omitempty"`
	Model           string `json:"model,omitempty"`
	ODataID         string `json:"odataId,omitempty"`
	StatusState     string `json:"statusState,omitempty"`
	UUID            string `json:"uuid,omitempty"`
}

// Represents a chassis reported by the BMC
type OOBChassis struct {
	AssetTag     string              `json:"assetTag,omitempty"`
	ChassisType  string              `json:"chassisType,omitempty"`
	Health       string              `json:"health,omitempty"`
	HealthRollup string              `json:"healthRollup,omitempty"`
	ID           string              `json:"id,omitempty"`
	Location     *OOBChassisLocation `json:"location,omitempty"`
	Manufacturer string              `json:"manufacturer,omitempty"`
	Model        string              `json:"model,omitempty"`
	ODataID      string              `json:"odataId,omitempty"`
	PartNumber   string              `json:"partNumber,omitempty"`
	PCIeDevices  []OOBPCIeDevice     `json:"pcieDevices,omitempty"`
	PowerState   string              `json:"powerState,omitempty"`
	SerialNumber string              `json:"serialNumber,omitempty"`
	SKU          string              `json:"sku,omitempty"`
	StatusState  string              `json:"statusState,omitempty"`
}

// Represents the physical location of a chassis
type OOBChassisLocation struct {
	Rack         string `json:"rack,omitempty"`
	RackOffset   *int   `json:"rackOffset,omitempty"`
	Room         string `json:"room,omitempty"`
	Row          string `json:"row,omitempty"`
	ServiceLabel string `json:"serviceLabel,omitempty"`
}

// Represents a PCIe device reported by the BMC
type OOBPCIeDevice struct {
	DeviceType      string `json:"deviceType,omitempty"`
	FirmwareVersion string `json:"firmwareVersion,omitempty"`
	Health          string `json:"health,omitempty"`
	HealthRollup    string `json:"healthRollup,omitempty"`
	ID              string `json:"id,omitempty"`
	Manufacturer    string `json:"manufacturer,omitempty"`
	Model           string `json:"model,omitempty"`
	ODataID         string `json:"odataId,omitempty"`
	PartNumber      string `json:"partNumber,omitempty"`
	SerialNumber    string `json:"serialNumber,omitempty"`
	SKU             string `json:"sku,omitempty"`
	StatusState     string `json:"statusState,omitempty"`
	UUID            string `json:"uuid,omitempty"`
}

// Represents a firmware inventory entry reported by the BMC
type OOBFirmware struct {
	Health       string `json:"health,omitempty"`
	HealthRollup string `json:"healthRollup,omitempty"`
	ID           string `json:"id,omitempty"`
	Name         string `json:"name,omitempty"`
	ODataID      string `json:"odataId,omitempty"`
	ReleaseDate  string `json:"releaseDate,omitempty"`
	ServiceID    string `json:"serviceId,omitempty"`
	StatusState  string `json:"statusState,omitempty"`
	Version      string `json:"version,omitempty"`
}

// Represents a collection error scoped to one OOB inventory domain
type OOBDomainError struct {
	Domain   string `json:"domain,omitempty"`
	Message  string `json:"message,omitempty"`
	Resource string `json:"resource,omitempty"`
}

func oobInventoryFromGenerated(inventory *fleetapi.ModelsOobInventory) *OOBInventory {
	if inventory == nil {
		return nil
	}

	out := &OOBInventory{
		CollectedAt:     inventory.CollectedAt,
		PrimarySystemID: stringValue(inventory.PrimarySystemId),
		SchemaVersion:   inventory.SchemaVersion,
		Source:          oobSourceFromGenerated(inventory.Source),
		TargetError:     stringValue(inventory.TargetError),
	}
	if inventory.Systems != nil {
		out.Systems = make([]OOBSystem, 0, len(*inventory.Systems))
		for _, system := range *inventory.Systems {
			out.Systems = append(out.Systems, oobSystemFromGenerated(system))
		}
	}
	if inventory.Managers != nil {
		out.Managers = make([]OOBManager, 0, len(*inventory.Managers))
		for _, manager := range *inventory.Managers {
			out.Managers = append(out.Managers, oobManagerFromGenerated(manager))
		}
	}
	if inventory.Chassis != nil {
		out.Chassis = make([]OOBChassis, 0, len(*inventory.Chassis))
		for _, chassis := range *inventory.Chassis {
			out.Chassis = append(out.Chassis, oobChassisFromGenerated(chassis))
		}
	}
	if inventory.Firmware != nil {
		out.Firmware = make([]OOBFirmware, 0, len(*inventory.Firmware))
		for _, firmware := range *inventory.Firmware {
			out.Firmware = append(out.Firmware, oobFirmwareFromGenerated(firmware))
		}
	}
	if inventory.DomainErrors != nil {
		out.DomainErrors = make([]OOBDomainError, 0, len(*inventory.DomainErrors))
		for _, domainError := range *inventory.DomainErrors {
			out.DomainErrors = append(out.DomainErrors, oobDomainErrorFromGenerated(domainError))
		}
	}
	return out
}

func oobSourceFromGenerated(source fleetapi.ModelsOobSource) *OOBSource {
	return &OOBSource{
		Address:        source.Address,
		Hostname:       stringValue(source.HostName),
		MAC:            source.Mac,
		RedfishVersion: stringValue(source.RedfishVersion),
		ServiceUUID:    stringValue(source.ServiceUuid),
		SourceType:     source.SourceType,
		Vendor:         stringValue(source.Vendor),
	}
}

func oobSystemFromGenerated(system fleetapi.ModelsOobSystem) OOBSystem {
	out := OOBSystem{
		AssetTag:          stringValue(system.AssetTag),
		BIOSVersion:       stringValue(system.BiosVersion),
		CPUCoreCount:      cloneInt(system.CpuCoreCount),
		CPUCount:          cloneInt(system.CpuCount),
		CPUModel:          stringValue(system.CpuModel),
		Health:            stringValue(system.Health),
		HealthRollup:      stringValue(system.HealthRollup),
		Hostname:          stringValue(system.HostName),
		ID:                system.Id,
		Manufacturer:      stringValue(system.Manufacturer),
		MemoryGiB:         cloneFloat32(system.MemoryGib),
		Model:             stringValue(system.Model),
		ODataID:           stringValue(system.OdataId),
		PowerState:        stringValue(system.PowerState),
		SecureBootEnabled: cloneBool(system.SecureBootEnabled),
		SerialNumber:      stringValue(system.SerialNumber),
		SKU:               stringValue(system.Sku),
		StatusState:       stringValue(system.StatusState),
		UUID:              stringValue(system.Uuid),
	}
	if system.Processors != nil {
		out.Processors = make([]OOBProcessor, 0, len(*system.Processors))
		for _, processor := range *system.Processors {
			out.Processors = append(out.Processors, oobProcessorFromGenerated(processor))
		}
	}
	return out
}

func oobProcessorFromGenerated(processor fleetapi.ModelsOobProcessor) OOBProcessor {
	return OOBProcessor{
		Health:                stringValue(processor.Health),
		HealthRollup:          stringValue(processor.HealthRollup),
		ID:                    processor.Id,
		InstructionSet:        stringValue(processor.InstructionSet),
		Manufacturer:          stringValue(processor.Manufacturer),
		MaxSpeedMHz:           cloneInt(processor.MaxSpeedMhz),
		Model:                 stringValue(processor.Model),
		ODataID:               stringValue(processor.OdataId),
		ProcessorArchitecture: stringValue(processor.ProcessorArchitecture),
		ProcessorType:         stringValue(processor.ProcessorType),
		Socket:                stringValue(processor.Socket),
		StatusState:           stringValue(processor.StatusState),
		TotalCores:            cloneInt(processor.TotalCores),
		TotalThreads:          cloneInt(processor.TotalThreads),
	}
}

func oobManagerFromGenerated(manager fleetapi.ModelsOobManager) OOBManager {
	return OOBManager{
		FirmwareVersion: stringValue(manager.FirmwareVersion),
		Health:          stringValue(manager.Health),
		HealthRollup:    stringValue(manager.HealthRollup),
		ID:              manager.Id,
		ManagerType:     stringValue(manager.ManagerType),
		Model:           stringValue(manager.Model),
		ODataID:         stringValue(manager.OdataId),
		StatusState:     stringValue(manager.StatusState),
		UUID:            stringValue(manager.Uuid),
	}
}

func oobChassisFromGenerated(chassis fleetapi.ModelsOobChassis) OOBChassis {
	out := OOBChassis{
		AssetTag:     stringValue(chassis.AssetTag),
		ChassisType:  stringValue(chassis.ChassisType),
		Health:       stringValue(chassis.Health),
		HealthRollup: stringValue(chassis.HealthRollup),
		ID:           chassis.Id,
		Location:     oobChassisLocationFromGenerated(chassis.Location),
		Manufacturer: stringValue(chassis.Manufacturer),
		Model:        stringValue(chassis.Model),
		ODataID:      stringValue(chassis.OdataId),
		PartNumber:   stringValue(chassis.PartNumber),
		PowerState:   stringValue(chassis.PowerState),
		SerialNumber: stringValue(chassis.SerialNumber),
		SKU:          stringValue(chassis.Sku),
		StatusState:  stringValue(chassis.StatusState),
	}
	if chassis.PcieDevices != nil {
		out.PCIeDevices = make([]OOBPCIeDevice, 0, len(*chassis.PcieDevices))
		for _, device := range *chassis.PcieDevices {
			out.PCIeDevices = append(out.PCIeDevices, oobPCIeDeviceFromGenerated(device))
		}
	}
	return out
}

func oobChassisLocationFromGenerated(location *fleetapi.ModelsOobChassisLocation) *OOBChassisLocation {
	if location == nil {
		return nil
	}
	return &OOBChassisLocation{
		Rack:         stringValue(location.Rack),
		RackOffset:   cloneInt(location.RackOffset),
		Room:         stringValue(location.Room),
		Row:          stringValue(location.Row),
		ServiceLabel: stringValue(location.ServiceLabel),
	}
}

func oobPCIeDeviceFromGenerated(device fleetapi.ModelsOobPcieDevice) OOBPCIeDevice {
	return OOBPCIeDevice{
		DeviceType:      stringValue(device.DeviceType),
		FirmwareVersion: stringValue(device.FirmwareVersion),
		Health:          stringValue(device.Health),
		HealthRollup:    stringValue(device.HealthRollup),
		ID:              device.Id,
		Manufacturer:    stringValue(device.Manufacturer),
		Model:           stringValue(device.Model),
		ODataID:         stringValue(device.OdataId),
		PartNumber:      stringValue(device.PartNumber),
		SerialNumber:    stringValue(device.SerialNumber),
		SKU:             stringValue(device.Sku),
		StatusState:     stringValue(device.StatusState),
		UUID:            stringValue(device.Uuid),
	}
}

func oobFirmwareFromGenerated(firmware fleetapi.ModelsOobFirmware) OOBFirmware {
	return OOBFirmware{
		Health:       stringValue(firmware.Health),
		HealthRollup: stringValue(firmware.HealthRollup),
		ID:           firmware.Id,
		Name:         firmware.Name,
		ODataID:      stringValue(firmware.OdataId),
		ReleaseDate:  stringValue(firmware.ReleaseDate),
		ServiceID:    firmware.ServiceId,
		StatusState:  stringValue(firmware.StatusState),
		Version:      stringValue(firmware.Version),
	}
}

func oobDomainErrorFromGenerated(domainError fleetapi.ModelsOobDomainError) OOBDomainError {
	return OOBDomainError{
		Domain:   domainError.Domain,
		Message:  domainError.Message,
		Resource: stringValue(domainError.Resource),
	}
}
