// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import "github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"

// NodeKind classifies the independently addressable node that produced an OOB inventory document.
type NodeKind string

const (
	NodeKindUnknown    NodeKind = "unknown"
	NodeKindCompute    NodeKind = "compute"
	NodeKindNVSwitch   NodeKind = "nvswitch"
	NodeKindPowerShelf NodeKind = "power_shelf"
)

// Represents inventory collected out of band through a node's BMC
type OOBInventory struct {
	Chassis         []OOBChassis     `json:"chassis,omitempty"`
	CollectedAt     string           `json:"collectedAt"`
	DomainErrors    []OOBDomainError `json:"domainErrors,omitempty"`
	Fabrics         []OOBFabric      `json:"fabrics,omitempty"`
	Firmware        []OOBFirmware    `json:"firmware,omitempty"`
	Managers        []OOBManager     `json:"managers,omitempty"`
	PrimarySystemID string           `json:"primarySystemId,omitempty"`
	SchemaVersion   string           `json:"schemaVersion"`
	Source          *OOBSource       `json:"source,omitempty"`
	Systems         []OOBSystem      `json:"systems,omitempty"`
	TargetError     string           `json:"targetError,omitempty"`
}

// OOBInventoryStatus represents the Redfish status reported for an inventory resource.
type OOBInventoryStatus struct {
	Conditions   []OOBInventoryCondition `json:"conditions,omitempty"`
	Health       string                  `json:"health,omitempty"`
	HealthRollup string                  `json:"healthRollup,omitempty"`
	State        string                  `json:"state,omitempty"`
}

// OOBInventoryCondition contains the details of one Redfish status condition.
type OOBInventoryCondition struct {
	ConditionType     string                 `json:"conditionType,omitempty"`
	Message           string                 `json:"message,omitempty"`
	MessageArgs       []string               `json:"messageArgs,omitempty"`
	MessageID         string                 `json:"messageId"`
	OriginOfCondition *OOBInventoryReference `json:"originOfCondition,omitempty"`
	Resolution        string                 `json:"resolution,omitempty"`
	Severity          string                 `json:"severity,omitempty"`
	Timestamp         string                 `json:"timestamp,omitempty"`
}

// OOBInventoryReference identifies a linked Redfish resource.
type OOBInventoryReference struct {
	ODataID string `json:"odataId"`
}

// Identifies the source used to collect OOB inventory
type OOBSource struct {
	Address        string `json:"address,omitempty"`
	MAC            string `json:"mac,omitempty"`
	RedfishVersion string `json:"redfishVersion,omitempty"`
	ServiceUUID    string `json:"serviceUuid,omitempty"`
	SourceType     string `json:"sourceType,omitempty"`
	Vendor         string `json:"vendor,omitempty"`
}

// Represents a computer system reported by the BMC
type OOBSystem struct {
	AssetTag          string               `json:"assetTag,omitempty"`
	BIOSVersion       string               `json:"biosVersion,omitempty"`
	Description       string               `json:"description,omitempty"`
	Hostname          string               `json:"hostName,omitempty"`
	ID                string               `json:"id,omitempty"`
	Links             OOBInventoryLinks    `json:"links,omitempty"`
	Manufacturer      string               `json:"manufacturer,omitempty"`
	Memory            []OOBMemory          `json:"memory,omitempty"`
	MemorySummary     *OOBMemorySummary    `json:"memorySummary,omitempty"`
	Model             string               `json:"model,omitempty"`
	Name              string               `json:"name,omitempty"`
	ODataID           string               `json:"odataId,omitempty"`
	PowerState        string               `json:"powerState,omitempty"`
	Processors        []OOBProcessor       `json:"processors,omitempty"`
	ProcessorSummary  *OOBProcessorSummary `json:"processorSummary,omitempty"`
	SecureBootEnabled *bool                `json:"secureBootEnabled,omitempty"`
	SerialNumber      string               `json:"serialNumber,omitempty"`
	SKU               string               `json:"sku,omitempty"`
	Status            *OOBInventoryStatus  `json:"status,omitempty"`
	Storage           []OOBStorage         `json:"storage,omitempty"`
	UUID              string               `json:"uuid,omitempty"`
}

// Represents a processor reported by the BMC
type OOBProcessor struct {
	Description           string              `json:"description,omitempty"`
	ID                    string              `json:"id,omitempty"`
	InstructionSet        string              `json:"instructionSet,omitempty"`
	Links                 OOBInventoryLinks   `json:"links,omitempty"`
	Location              *OOBChassisLocation `json:"location,omitempty"`
	Manufacturer          string              `json:"manufacturer,omitempty"`
	MaxSpeedMHz           *int                `json:"maxSpeedMhz,omitempty"`
	Model                 string              `json:"model,omitempty"`
	Name                  string              `json:"name,omitempty"`
	NvidiaGPU             *OOBNvidiaGPU       `json:"nvidiaGpu,omitempty"`
	ODataID               string              `json:"odataId,omitempty"`
	PartNumber            string              `json:"partNumber,omitempty"`
	ProcessorArchitecture string              `json:"processorArchitecture,omitempty"`
	ProcessorType         string              `json:"processorType,omitempty"`
	SerialNumber          string              `json:"serialNumber,omitempty"`
	Socket                string              `json:"socket,omitempty"`
	Status                *OOBInventoryStatus `json:"status,omitempty"`
	TotalCores            *int                `json:"totalCores,omitempty"`
	TotalThreads          *int                `json:"totalThreads,omitempty"`
	UUID                  string              `json:"uuid,omitempty"`
}

// Represents a BMC manager
type OOBManager struct {
	Description     string              `json:"description,omitempty"`
	FirmwareVersion string              `json:"firmwareVersion,omitempty"`
	ID              string              `json:"id,omitempty"`
	Links           OOBInventoryLinks   `json:"links,omitempty"`
	ManagerType     string              `json:"managerType,omitempty"`
	Model           string              `json:"model,omitempty"`
	Name            string              `json:"name,omitempty"`
	ODataID         string              `json:"odataId,omitempty"`
	Status          *OOBInventoryStatus `json:"status,omitempty"`
	UUID            string              `json:"uuid,omitempty"`
}

// Represents a chassis reported by the BMC
type OOBChassis struct {
	AssetTag      string              `json:"assetTag,omitempty"`
	ChassisType   string              `json:"chassisType,omitempty"`
	Description   string              `json:"description,omitempty"`
	ID            string              `json:"id,omitempty"`
	Links         OOBInventoryLinks   `json:"links,omitempty"`
	Location      *OOBChassisLocation `json:"location,omitempty"`
	Manufacturer  string              `json:"manufacturer,omitempty"`
	Model         string              `json:"model,omitempty"`
	Name          string              `json:"name,omitempty"`
	ODataID       string              `json:"odataId,omitempty"`
	PartNumber    string              `json:"partNumber,omitempty"`
	PCIeDevices   []OOBPCIeDevice     `json:"pcieDevices,omitempty"`
	PowerState    string              `json:"powerState,omitempty"`
	PowerSupplies []OOBPowerSupply    `json:"powerSupplies,omitempty"`
	SerialNumber  string              `json:"serialNumber,omitempty"`
	SKU           string              `json:"sku,omitempty"`
	Status        *OOBInventoryStatus `json:"status,omitempty"`
}

// Represents the physical location of a chassis
type OOBChassisLocation struct {
	PartLocation        *OOBPartLocation `json:"partLocation,omitempty"`
	PartLocationContext string           `json:"partLocationContext,omitempty"`
	Placement           *OOBPlacement    `json:"placement,omitempty"`
}

// Represents a PCIe device reported by the BMC
type OOBPCIeDevice struct {
	Description     string              `json:"description,omitempty"`
	DeviceType      string              `json:"deviceType,omitempty"`
	FirmwareVersion string              `json:"firmwareVersion,omitempty"`
	ID              string              `json:"id,omitempty"`
	Links           OOBInventoryLinks   `json:"links,omitempty"`
	Manufacturer    string              `json:"manufacturer,omitempty"`
	Model           string              `json:"model,omitempty"`
	Name            string              `json:"name,omitempty"`
	ODataID         string              `json:"odataId,omitempty"`
	PartNumber      string              `json:"partNumber,omitempty"`
	SerialNumber    string              `json:"serialNumber,omitempty"`
	SKU             string              `json:"sku,omitempty"`
	Status          *OOBInventoryStatus `json:"status,omitempty"`
	UUID            string              `json:"uuid,omitempty"`
}

// Represents a firmware inventory entry reported by the BMC
type OOBFirmware struct {
	Description string                  `json:"description,omitempty"`
	ID          string                  `json:"id,omitempty"`
	Links       OOBInventoryLinks       `json:"links,omitempty"`
	Name        string                  `json:"name,omitempty"`
	ODataID     string                  `json:"odataId,omitempty"`
	RelatedItem []OOBInventoryReference `json:"relatedItem,omitempty"`
	ReleaseDate string                  `json:"releaseDate,omitempty"`
	ServiceID   string                  `json:"serviceId,omitempty"`
	Status      *OOBInventoryStatus     `json:"status,omitempty"`
	Version     string                  `json:"version,omitempty"`
}

// Represents a collection error scoped to one OOB inventory domain
type OOBDomainError struct {
	Domain   string `json:"domain,omitempty"`
	Message  string `json:"message,omitempty"`
	Resource string `json:"resource,omitempty"`
}

// OOBInventoryLinks maps relation names to referenced Redfish resources.
type OOBInventoryLinks map[string][]OOBInventoryReference

// OOBDrive represents a storage drive reported by the BMC.
type OOBDrive struct {
	CapacityBytes *int                `json:"capacityBytes,omitempty"`
	ID            string              `json:"id"`
	MediaType     string              `json:"mediaType,omitempty"`
	Model         string              `json:"model,omitempty"`
	ODataID       string              `json:"odataId,omitempty"`
	Protocol      string              `json:"protocol,omitempty"`
	Revision      string              `json:"revision,omitempty"`
	SerialNumber  string              `json:"serialNumber,omitempty"`
	Status        *OOBInventoryStatus `json:"status,omitempty"`
	StorageID     string              `json:"storageId"`
	SystemID      string              `json:"systemId"`
}

// OOBFabric represents a fabric and its switches reported by the BMC.
type OOBFabric struct {
	Description string              `json:"description,omitempty"`
	FabricType  string              `json:"fabricType,omitempty"`
	ID          string              `json:"id"`
	Links       OOBInventoryLinks   `json:"links,omitempty"`
	Name        string              `json:"name"`
	ODataID     string              `json:"odataId,omitempty"`
	Status      *OOBInventoryStatus `json:"status,omitempty"`
	Switches    []OOBSwitch         `json:"switches,omitempty"`
	UUID        string              `json:"uuid,omitempty"`
}

// OOBMemory represents a memory module reported by the BMC.
type OOBMemory struct {
	CapacityMiB      *int                `json:"capacityMib,omitempty"`
	DeviceLocator    string              `json:"deviceLocator,omitempty"`
	ID               string              `json:"id"`
	Location         *OOBChassisLocation `json:"location,omitempty"`
	Manufacturer     string              `json:"manufacturer,omitempty"`
	MemoryDeviceType string              `json:"memoryDeviceType,omitempty"`
	ODataID          string              `json:"odataId,omitempty"`
	PartNumber       string              `json:"partNumber,omitempty"`
	SerialNumber     string              `json:"serialNumber,omitempty"`
	Status           *OOBInventoryStatus `json:"status,omitempty"`
	SystemID         string              `json:"systemId"`
}

// OOBMemorySummary represents system memory totals reported by the BMC.
type OOBMemorySummary struct {
	MemoryMirroring                string              `json:"memoryMirroring,omitempty"`
	Status                         *OOBInventoryStatus `json:"status,omitempty"`
	TotalSystemMemoryGiB           *float32            `json:"totalSystemMemoryGiB,omitempty"`
	TotalSystemPersistentMemoryGiB *float32            `json:"totalSystemPersistentMemoryGiB,omitempty"`
}

// OOBNVLinkTopology represents raw NVLink topology identifiers reported by the BMC.
type OOBNVLinkTopology struct {
	ChassisSerialNumber string `json:"chassisSerialNumber,omitempty"`
	HostID              *int   `json:"hostId,omitempty"`
	// IBGUID retains raw Redfish hexadecimal text; NMX-C matching requires little-endian decoding.
	IBGUID    string `json:"ibGuid,omitempty"`
	LSBIBGUID string `json:"lsbIbGuid,omitempty"`
	ModuleID  *int   `json:"moduleId,omitempty"`
	PeerType  string `json:"peerType,omitempty"`
	// SystemGUID retains raw Redfish hexadecimal text without byte-order conversion.
	SystemGUID string `json:"systemGuid,omitempty"`
	// TraySerialNumber retains raw Redfish text; "NA" means not reported.
	TraySerialNumber string `json:"traySerialNumber,omitempty"`
	TraySlotIndex    *int   `json:"traySlotIndex,omitempty"`
	TraySlotNumber   *int   `json:"traySlotNumber,omitempty"`
}

// OOBNvidiaGPU represents NVIDIA GPU capabilities and topology reported by the BMC.
type OOBNvidiaGPU struct {
	CCDevModeEnabled          *bool              `json:"ccDevModeEnabled,omitempty"`
	CCModeEnabled             *bool              `json:"ccModeEnabled,omitempty"`
	DefaultBoostClockSpeedMHz *int               `json:"defaultBoostClockSpeedMhz,omitempty"`
	EGMModeEnabled            *bool              `json:"egmModeEnabled,omitempty"`
	MaxBoostClockSpeedMHz     *int               `json:"maxBoostClockSpeedMhz,omitempty"`
	MIGModeEnabled            *bool              `json:"migModeEnabled,omitempty"`
	NVLinkTopology            *OOBNVLinkTopology `json:"nvlinkTopology,omitempty"`
	ProtectedPCIeModeEnabled  *bool              `json:"protectedPcieModeEnabled,omitempty"`
	TotalNVLinks              *int               `json:"totalNvLinks,omitempty"`
}

// OOBNvidiaSwitch represents NVIDIA switch capabilities reported by the BMC.
type OOBNvidiaSwitch struct {
	DeviceID                  string `json:"deviceId,omitempty"`
	PCIeReferenceClockEnabled *bool  `json:"pcieReferenceClockEnabled,omitempty"`
	ProtectedPCIeModeEnabled  *bool  `json:"protectedPcieModeEnabled,omitempty"`
	SwitchIsolationMode       string `json:"switchIsolationMode,omitempty"`
	VendorID                  string `json:"vendorId,omitempty"`
}

// OOBPartLocation represents a component position reported by the BMC.
type OOBPartLocation struct {
	LocationOrdinalValue *int   `json:"locationOrdinalValue,omitempty"`
	LocationType         string `json:"locationType,omitempty"`
	Orientation          string `json:"orientation,omitempty"`
	Reference            string `json:"reference,omitempty"`
	ServiceLabel         string `json:"serviceLabel,omitempty"`
}

// OOBPlacement represents a physical rack placement reported by the BMC.
type OOBPlacement struct {
	AdditionalInfo  string `json:"additionalInfo,omitempty"`
	FacilityName    string `json:"facilityName,omitempty"`
	Rack            string `json:"rack,omitempty"`
	RackOffset      *int   `json:"rackOffset,omitempty"`
	RackOffsetUnits string `json:"rackOffsetUnits,omitempty"`
	Room            string `json:"room,omitempty"`
	Row             string `json:"row,omitempty"`
}

// OOBPowerSupply represents a chassis power supply reported by the BMC.
type OOBPowerSupply struct {
	ChassisID          string              `json:"chassisId"`
	FirmwareVersion    string              `json:"firmwareVersion,omitempty"`
	ID                 string              `json:"id"`
	Model              string              `json:"model,omitempty"`
	ODataID            string              `json:"odataId,omitempty"`
	PartNumber         string              `json:"partNumber,omitempty"`
	PowerCapacityWatts *float32            `json:"powerCapacityWatts,omitempty"`
	SerialNumber       string              `json:"serialNumber,omitempty"`
	Status             *OOBInventoryStatus `json:"status,omitempty"`
}

// OOBProcessorSummary represents system processor totals reported by the BMC.
type OOBProcessorSummary struct {
	CoreCount             *int                `json:"coreCount,omitempty"`
	Count                 *int                `json:"count,omitempty"`
	LogicalProcessorCount *int                `json:"logicalProcessorCount,omitempty"`
	Model                 string              `json:"model,omitempty"`
	Status                *OOBInventoryStatus `json:"status,omitempty"`
	ThreadingEnabled      *bool               `json:"threadingEnabled,omitempty"`
}

// OOBStorage represents a storage controller and its drives reported by the BMC.
type OOBStorage struct {
	Drives   []OOBDrive `json:"drives,omitempty"`
	ID       string     `json:"id"`
	ODataID  string     `json:"odataId,omitempty"`
	SystemID string     `json:"systemId"`
}

// OOBSwitch represents a fabric switch reported by the BMC.
type OOBSwitch struct {
	Description     string              `json:"description,omitempty"`
	FirmwareVersion string              `json:"firmwareVersion,omitempty"`
	ID              string              `json:"id"`
	Links           OOBInventoryLinks   `json:"links,omitempty"`
	Location        *OOBChassisLocation `json:"location,omitempty"`
	Manufacturer    string              `json:"manufacturer,omitempty"`
	Model           string              `json:"model,omitempty"`
	Name            string              `json:"name"`
	NvidiaSwitch    *OOBNvidiaSwitch    `json:"nvidiaSwitch,omitempty"`
	ODataID         string              `json:"odataId,omitempty"`
	PartNumber      string              `json:"partNumber,omitempty"`
	SerialNumber    string              `json:"serialNumber,omitempty"`
	SKU             string              `json:"sku,omitempty"`
	Status          *OOBInventoryStatus `json:"status,omitempty"`
	SwitchType      string              `json:"switchType,omitempty"`
	UUID            string              `json:"uuid,omitempty"`
}

func oobInventoryFromGenerated(inventory *fleetapi.ModelsOobInventory) *OOBInventory {
	if inventory == nil {
		return nil
	}

	out := &OOBInventory{
		Fabrics:         mapOOBItems(inventory.Fabrics, oobFabricFromGenerated),
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
		MAC:            source.Mac,
		RedfishVersion: stringValue(source.RedfishVersion),
		ServiceUUID:    stringValue(source.ServiceUuid),
		SourceType:     source.SourceType,
		Vendor:         stringValue(source.Vendor),
	}
}

func oobSystemFromGenerated(system fleetapi.ModelsOobSystem) OOBSystem {
	out := OOBSystem{
		Description:       stringValue(system.Description),
		Links:             oobInventoryLinksFromGenerated(system.Links),
		Memory:            mapOOBItems(system.Memory, oobMemoryFromGenerated),
		MemorySummary:     oobMemorySummaryFromGenerated(system.MemorySummary),
		Name:              stringValue(system.Name),
		ProcessorSummary:  oobProcessorSummaryFromGenerated(system.ProcessorSummary),
		Storage:           mapOOBItems(system.Storage, oobStorageFromGenerated),
		AssetTag:          stringValue(system.AssetTag),
		BIOSVersion:       stringValue(system.BiosVersion),
		Hostname:          stringValue(system.HostName),
		ID:                system.Id,
		Manufacturer:      stringValue(system.Manufacturer),
		Model:             stringValue(system.Model),
		ODataID:           stringValue(system.OdataId),
		PowerState:        stringValue(system.PowerState),
		SecureBootEnabled: cloneBool(system.SecureBootEnabled),
		SerialNumber:      stringValue(system.SerialNumber),
		SKU:               stringValue(system.Sku),
		Status:            oobInventoryStatusFromGenerated(system.Status),
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
		Description:           stringValue(processor.Description),
		Links:                 oobInventoryLinksFromGenerated(processor.Links),
		Location:              oobChassisLocationFromGenerated(processor.Location),
		Name:                  stringValue(processor.Name),
		NvidiaGPU:             oobNvidiaGPUFromGenerated(processor.NvidiaGpu),
		PartNumber:            stringValue(processor.PartNumber),
		SerialNumber:          stringValue(processor.SerialNumber),
		UUID:                  stringValue(processor.Uuid),
		ID:                    processor.Id,
		InstructionSet:        stringValue(processor.InstructionSet),
		Manufacturer:          stringValue(processor.Manufacturer),
		MaxSpeedMHz:           cloneInt(processor.MaxSpeedMhz),
		Model:                 stringValue(processor.Model),
		ODataID:               stringValue(processor.OdataId),
		ProcessorArchitecture: stringValue(processor.ProcessorArchitecture),
		ProcessorType:         stringValue(processor.ProcessorType),
		Socket:                stringValue(processor.Socket),
		Status:                oobInventoryStatusFromGenerated(processor.Status),
		TotalCores:            cloneInt(processor.TotalCores),
		TotalThreads:          cloneInt(processor.TotalThreads),
	}
}

func oobManagerFromGenerated(manager fleetapi.ModelsOobManager) OOBManager {
	return OOBManager{
		Description:     stringValue(manager.Description),
		Links:           oobInventoryLinksFromGenerated(manager.Links),
		Name:            stringValue(manager.Name),
		FirmwareVersion: stringValue(manager.FirmwareVersion),
		ID:              manager.Id,
		ManagerType:     stringValue(manager.ManagerType),
		Model:           stringValue(manager.Model),
		ODataID:         stringValue(manager.OdataId),
		Status:          oobInventoryStatusFromGenerated(manager.Status),
		UUID:            stringValue(manager.Uuid),
	}
}

func oobChassisFromGenerated(chassis fleetapi.ModelsOobChassis) OOBChassis {
	out := OOBChassis{
		Description:   stringValue(chassis.Description),
		Links:         oobInventoryLinksFromGenerated(chassis.Links),
		Name:          stringValue(chassis.Name),
		PowerSupplies: mapOOBItems(chassis.PowerSupplies, oobPowerSupplyFromGenerated),
		AssetTag:      stringValue(chassis.AssetTag),
		ChassisType:   stringValue(chassis.ChassisType),
		ID:            chassis.Id,
		Location:      oobChassisLocationFromGenerated(chassis.Location),
		Manufacturer:  stringValue(chassis.Manufacturer),
		Model:         stringValue(chassis.Model),
		ODataID:       stringValue(chassis.OdataId),
		PartNumber:    stringValue(chassis.PartNumber),
		PowerState:    stringValue(chassis.PowerState),
		SerialNumber:  stringValue(chassis.SerialNumber),
		SKU:           stringValue(chassis.Sku),
		Status:        oobInventoryStatusFromGenerated(chassis.Status),
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
		PartLocation:        oobPartLocationFromGenerated(location.PartLocation),
		PartLocationContext: stringValue(location.PartLocationContext),
		Placement:           oobPlacementFromGenerated(location.Placement),
	}
}

func oobPCIeDeviceFromGenerated(device fleetapi.ModelsOobPcieDevice) OOBPCIeDevice {
	return OOBPCIeDevice{
		Description:     stringValue(device.Description),
		Links:           oobInventoryLinksFromGenerated(device.Links),
		Name:            stringValue(device.Name),
		DeviceType:      stringValue(device.DeviceType),
		FirmwareVersion: stringValue(device.FirmwareVersion),
		ID:              device.Id,
		Manufacturer:    stringValue(device.Manufacturer),
		Model:           stringValue(device.Model),
		ODataID:         stringValue(device.OdataId),
		PartNumber:      stringValue(device.PartNumber),
		SerialNumber:    stringValue(device.SerialNumber),
		SKU:             stringValue(device.Sku),
		Status:          oobInventoryStatusFromGenerated(device.Status),
		UUID:            stringValue(device.Uuid),
	}
}

func oobFirmwareFromGenerated(firmware fleetapi.ModelsOobFirmware) OOBFirmware {
	return OOBFirmware{
		Description: stringValue(firmware.Description),
		Links:       oobInventoryLinksFromGenerated(firmware.Links),
		RelatedItem: mapOOBItems(firmware.RelatedItem, oobInventoryReferenceFromGenerated),
		ID:          firmware.Id,
		Name:        firmware.Name,
		ODataID:     stringValue(firmware.OdataId),
		ReleaseDate: stringValue(firmware.ReleaseDate),
		ServiceID:   firmware.ServiceId,
		Status:      oobInventoryStatusFromGenerated(firmware.Status),
		Version:     stringValue(firmware.Version),
	}
}

func nodeKindFromGenerated(kind *fleetapi.ModelsNodeKind) NodeKind {
	if kind == nil {
		return ""
	}
	return NodeKind(*kind)
}

func oobInventoryStatusFromGenerated(status *fleetapi.ModelsOobInventoryStatus) *OOBInventoryStatus {
	if status == nil {
		return nil
	}
	out := &OOBInventoryStatus{
		Health:       stringValue(status.Health),
		HealthRollup: stringValue(status.HealthRollup),
		State:        stringValue(status.State),
	}
	if status.Conditions != nil {
		out.Conditions = make([]OOBInventoryCondition, 0, len(*status.Conditions))
		for _, condition := range *status.Conditions {
			out.Conditions = append(out.Conditions, oobInventoryConditionFromGenerated(condition))
		}
	}
	return out
}

func oobInventoryConditionFromGenerated(condition fleetapi.ModelsOobInventoryCondition) OOBInventoryCondition {
	out := OOBInventoryCondition{
		ConditionType: stringValue(condition.ConditionType),
		Message:       stringValue(condition.Message),
		MessageID:     condition.MessageId,
		Resolution:    stringValue(condition.Resolution),
		Severity:      stringValue(condition.Severity),
		Timestamp:     stringValue(condition.Timestamp),
	}
	if condition.MessageArgs != nil {
		out.MessageArgs = append([]string(nil), (*condition.MessageArgs)...)
	}
	if condition.OriginOfCondition != nil {
		out.OriginOfCondition = &OOBInventoryReference{ODataID: condition.OriginOfCondition.OdataId}
	}
	return out
}

func oobDomainErrorFromGenerated(domainError fleetapi.ModelsOobDomainError) OOBDomainError {
	return OOBDomainError{
		Domain:   domainError.Domain,
		Message:  domainError.Message,
		Resource: stringValue(domainError.Resource),
	}
}

func oobDriveFromGenerated(item fleetapi.ModelsOobDrive) OOBDrive {
	return OOBDrive{
		CapacityBytes: cloneInt(item.CapacityBytes),
		ID:            item.Id,
		MediaType:     stringValue(item.MediaType),
		Model:         stringValue(item.Model),
		ODataID:       stringValue(item.OdataId),
		Protocol:      stringValue(item.Protocol),
		Revision:      stringValue(item.Revision),
		SerialNumber:  stringValue(item.SerialNumber),
		Status:        oobInventoryStatusFromGenerated(item.Status),
		StorageID:     item.StorageId,
		SystemID:      item.SystemId,
	}
}

func oobFabricFromGenerated(item fleetapi.ModelsOobFabric) OOBFabric {
	return OOBFabric{
		Description: stringValue(item.Description),
		FabricType:  stringValue(item.FabricType),
		ID:          item.Id,
		Links:       oobInventoryLinksFromGenerated(item.Links),
		Name:        item.Name,
		ODataID:     stringValue(item.OdataId),
		Status:      oobInventoryStatusFromGenerated(item.Status),
		Switches:    mapOOBItems(item.Switches, oobSwitchFromGenerated),
		UUID:        stringValue(item.Uuid),
	}
}

func oobMemoryFromGenerated(item fleetapi.ModelsOobMemory) OOBMemory {
	return OOBMemory{
		CapacityMiB:      cloneInt(item.CapacityMib),
		DeviceLocator:    stringValue(item.DeviceLocator),
		ID:               item.Id,
		Location:         oobChassisLocationFromGenerated(item.Location),
		Manufacturer:     stringValue(item.Manufacturer),
		MemoryDeviceType: stringValue(item.MemoryDeviceType),
		ODataID:          stringValue(item.OdataId),
		PartNumber:       stringValue(item.PartNumber),
		SerialNumber:     stringValue(item.SerialNumber),
		Status:           oobInventoryStatusFromGenerated(item.Status),
		SystemID:         item.SystemId,
	}
}

func oobMemorySummaryFromGenerated(item *fleetapi.ModelsOobMemorySummary) *OOBMemorySummary {
	if item == nil {
		return nil
	}
	return &OOBMemorySummary{
		MemoryMirroring:                stringValue(item.MemoryMirroring),
		Status:                         oobInventoryStatusFromGenerated(item.Status),
		TotalSystemMemoryGiB:           cloneFloat32(item.TotalSystemMemoryGiB),
		TotalSystemPersistentMemoryGiB: cloneFloat32(item.TotalSystemPersistentMemoryGiB),
	}
}

func oobNVLinkTopologyFromGenerated(item *fleetapi.ModelsOobNVLinkTopology) *OOBNVLinkTopology {
	if item == nil {
		return nil
	}
	return &OOBNVLinkTopology{
		ChassisSerialNumber: stringValue(item.ChassisSerialNumber),
		HostID:              cloneInt(item.HostId),
		IBGUID:              stringValue(item.IbGuid),
		LSBIBGUID:           stringValue(item.LsbIbGuid),
		ModuleID:            cloneInt(item.ModuleId),
		PeerType:            stringValue(item.PeerType),
		SystemGUID:          stringValue(item.SystemGuid),
		TraySerialNumber:    stringValue(item.TraySerialNumber),
		TraySlotIndex:       cloneInt(item.TraySlotIndex),
		TraySlotNumber:      cloneInt(item.TraySlotNumber),
	}
}

func oobNvidiaGPUFromGenerated(item *fleetapi.ModelsOobNvidiaGPU) *OOBNvidiaGPU {
	if item == nil {
		return nil
	}
	return &OOBNvidiaGPU{
		CCDevModeEnabled:          cloneBool(item.CcDevModeEnabled),
		CCModeEnabled:             cloneBool(item.CcModeEnabled),
		DefaultBoostClockSpeedMHz: cloneInt(item.DefaultBoostClockSpeedMhz),
		EGMModeEnabled:            cloneBool(item.EgmModeEnabled),
		MaxBoostClockSpeedMHz:     cloneInt(item.MaxBoostClockSpeedMhz),
		MIGModeEnabled:            cloneBool(item.MigModeEnabled),
		NVLinkTopology:            oobNVLinkTopologyFromGenerated(item.NvlinkTopology),
		ProtectedPCIeModeEnabled:  cloneBool(item.ProtectedPcieModeEnabled),
		TotalNVLinks:              cloneInt(item.TotalNvLinks),
	}
}

func oobNvidiaSwitchFromGenerated(item *fleetapi.ModelsOobNvidiaSwitch) *OOBNvidiaSwitch {
	if item == nil {
		return nil
	}
	return &OOBNvidiaSwitch{
		DeviceID:                  stringValue(item.DeviceId),
		PCIeReferenceClockEnabled: cloneBool(item.PcieReferenceClockEnabled),
		ProtectedPCIeModeEnabled:  cloneBool(item.ProtectedPcieModeEnabled),
		SwitchIsolationMode:       stringValue(item.SwitchIsolationMode),
		VendorID:                  stringValue(item.VendorId),
	}
}

func oobPartLocationFromGenerated(item *fleetapi.ModelsOobPartLocation) *OOBPartLocation {
	if item == nil {
		return nil
	}
	return &OOBPartLocation{
		LocationOrdinalValue: cloneInt(item.LocationOrdinalValue),
		LocationType:         stringValue(item.LocationType),
		Orientation:          stringValue(item.Orientation),
		Reference:            stringValue(item.Reference),
		ServiceLabel:         stringValue(item.ServiceLabel),
	}
}

func oobPlacementFromGenerated(item *fleetapi.ModelsOobPlacement) *OOBPlacement {
	if item == nil {
		return nil
	}
	return &OOBPlacement{
		AdditionalInfo:  stringValue(item.AdditionalInfo),
		FacilityName:    stringValue(item.FacilityName),
		Rack:            stringValue(item.Rack),
		RackOffset:      cloneInt(item.RackOffset),
		RackOffsetUnits: stringValue(item.RackOffsetUnits),
		Room:            stringValue(item.Room),
		Row:             stringValue(item.Row),
	}
}

func oobPowerSupplyFromGenerated(item fleetapi.ModelsOobPowerSupply) OOBPowerSupply {
	return OOBPowerSupply{
		ChassisID:          item.ChassisId,
		FirmwareVersion:    stringValue(item.FirmwareVersion),
		ID:                 item.Id,
		Model:              stringValue(item.Model),
		ODataID:            stringValue(item.OdataId),
		PartNumber:         stringValue(item.PartNumber),
		PowerCapacityWatts: cloneFloat32(item.PowerCapacityWatts),
		SerialNumber:       stringValue(item.SerialNumber),
		Status:             oobInventoryStatusFromGenerated(item.Status),
	}
}

func oobProcessorSummaryFromGenerated(item *fleetapi.ModelsOobProcessorSummary) *OOBProcessorSummary {
	if item == nil {
		return nil
	}
	return &OOBProcessorSummary{
		CoreCount:             cloneInt(item.CoreCount),
		Count:                 cloneInt(item.Count),
		LogicalProcessorCount: cloneInt(item.LogicalProcessorCount),
		Model:                 stringValue(item.Model),
		Status:                oobInventoryStatusFromGenerated(item.Status),
		ThreadingEnabled:      cloneBool(item.ThreadingEnabled),
	}
}

func oobStorageFromGenerated(item fleetapi.ModelsOobStorage) OOBStorage {
	return OOBStorage{
		Drives:   mapOOBItems(item.Drives, oobDriveFromGenerated),
		ID:       item.Id,
		ODataID:  stringValue(item.OdataId),
		SystemID: item.SystemId,
	}
}

func oobSwitchFromGenerated(item fleetapi.ModelsOobSwitch) OOBSwitch {
	return OOBSwitch{
		Description:     stringValue(item.Description),
		FirmwareVersion: stringValue(item.FirmwareVersion),
		ID:              item.Id,
		Links:           oobInventoryLinksFromGenerated(item.Links),
		Location:        oobChassisLocationFromGenerated(item.Location),
		Manufacturer:    stringValue(item.Manufacturer),
		Model:           stringValue(item.Model),
		Name:            item.Name,
		NvidiaSwitch:    oobNvidiaSwitchFromGenerated(item.NvidiaSwitch),
		ODataID:         stringValue(item.OdataId),
		PartNumber:      stringValue(item.PartNumber),
		SerialNumber:    stringValue(item.SerialNumber),
		SKU:             stringValue(item.Sku),
		Status:          oobInventoryStatusFromGenerated(item.Status),
		SwitchType:      stringValue(item.SwitchType),
		UUID:            stringValue(item.Uuid),
	}
}

func mapOOBItems[S, T any](items *[]S, convert func(S) T) []T {
	if items == nil {
		return nil
	}
	out := make([]T, len(*items))
	for i, item := range *items {
		out[i] = convert(item)
	}
	return out
}

func oobInventoryReferenceFromGenerated(item fleetapi.ModelsOobInventoryReference) OOBInventoryReference {
	return OOBInventoryReference{ODataID: item.OdataId}
}

func oobInventoryLinksFromGenerated(links *fleetapi.ModelsOobInventoryLinks) OOBInventoryLinks {
	if links == nil {
		return nil
	}
	out := make(OOBInventoryLinks, len(*links))
	for relation, refs := range *links {
		if refs != nil {
			out[relation] = mapOOBItems(&refs, oobInventoryReferenceFromGenerated)
		} else {
			out[relation] = nil
		}
	}
	return out
}
