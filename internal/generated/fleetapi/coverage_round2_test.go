// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package fleetapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRound2ErrorGraphInvalidTimestampMerge(t *testing.T) {
	start := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	errorName := "xid_154"
	values := "[[1716153600,5]]"
	result := []ModelsErrorTimeSeriesByError{{
		Label:  &ModelsErrorLabelByError{Error: &errorName},
		Values: &values,
	}}
	original := ModelsErrorReportGraphByErrorResponse{
		Result:    &result,
		TimeRange: &ModelsTimeRange{Start: &start, End: &end},
	}
	var union GetV1ReportsError200JSONResponseBody
	if err := union.FromModelsErrorReportGraphByErrorResponse(original); err != nil {
		t.Fatalf("From: %v", err)
	}

	invalidEnd := time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
	invalid := ModelsErrorReportGraphByErrorResponse{TimeRange: &ModelsTimeRange{End: &invalidEnd}}
	err := union.MergeModelsErrorReportGraphByErrorResponse(invalid)
	var marshalerError *json.MarshalerError
	if !errors.As(err, &marshalerError) {
		t.Errorf("Merge error = %v, want *json.MarshalerError", err)
	}
	got, err := union.AsModelsErrorReportGraphByErrorResponse()
	if err != nil {
		t.Fatalf("As: %v", err)
	}
	if !reflect.DeepEqual(got, original) {
		t.Errorf("model after failed merge = %+v, want %+v", got, original)
	}
}

func testRound2ResourceUnionNonfiniteMerge[M any, U any](t *testing.T, original, invalid M,
	from func(*U, M) error, merge func(*U, M) error, as func(*U) (M, error)) {
	t.Helper()
	var union U
	if err := from(&union, original); err != nil {
		t.Fatalf("From: %v", err)
	}
	err := merge(&union, invalid)
	var unsupported *json.UnsupportedValueError
	if !errors.As(err, &unsupported) {
		t.Errorf("Merge error = %v, want *json.UnsupportedValueError", err)
	}
	got, err := as(&union)
	if err != nil {
		t.Fatalf("As: %v", err)
	}
	if !reflect.DeepEqual(got, original) {
		t.Errorf("model after failed merge = %+v, want %+v", got, original)
	}
}

func TestRound2ResourceUnionNonfiniteMergeContracts(t *testing.T) {
	finite := float32(42.5)
	nonfinite := float32(math.NaN())
	t.Run("compute_zones", func(t *testing.T) {
		originalZones := []ModelsComputeZoneOverview{{Id: "zone-1", HealthPercentage: &finite}}
		invalidZones := []ModelsComputeZoneOverview{{Id: "zone-1", HealthPercentage: &nonfinite}}
		testRound2ResourceUnionNonfiniteMerge(t,
			ModelsComputeZonesResponse{Computezones: &originalZones}, ModelsComputeZonesResponse{Computezones: &invalidZones},
			(*GetV1Computezones200JSONResponseBody).FromModelsComputeZonesResponse,
			(*GetV1Computezones200JSONResponseBody).MergeModelsComputeZonesResponse,
			(*GetV1Computezones200JSONResponseBody).AsModelsComputeZonesResponse)
	})
	t.Run("node_groups", func(t *testing.T) {
		originalGroups := []ModelsNodeGroupOverview{{Id: "group-1", HealthPercentage: &finite}}
		invalidGroups := []ModelsNodeGroupOverview{{Id: "group-1", HealthPercentage: &nonfinite}}
		testRound2ResourceUnionNonfiniteMerge(t,
			ModelsNodeGroupsResponse{NodeGroups: &originalGroups}, ModelsNodeGroupsResponse{NodeGroups: &invalidGroups},
			(*GetV1Nodegroups200JSONResponseBody).FromModelsNodeGroupsResponse,
			(*GetV1Nodegroups200JSONResponseBody).MergeModelsNodeGroupsResponse,
			(*GetV1Nodegroups200JSONResponseBody).AsModelsNodeGroupsResponse)
	})
	t.Run("inband_node", func(t *testing.T) {
		original := ModelsInbandNodeDetailsResponse{NodeUUID: "node-1", DataSource: "inband", Location: &ModelsLocation{Latitude: &finite}}
		invalid := ModelsInbandNodeDetailsResponse{NodeUUID: "node-1", DataSource: "inband", Location: &ModelsLocation{Latitude: &nonfinite}}
		testRound2ResourceUnionNonfiniteMerge(t, original, invalid,
			(*GetV1NodesNodeUuid200JSONResponseBody).FromModelsInbandNodeDetailsResponse,
			(*GetV1NodesNodeUuid200JSONResponseBody).MergeModelsInbandNodeDetailsResponse,
			(*GetV1NodesNodeUuid200JSONResponseBody).AsModelsInbandNodeDetailsResponse)
	})
	t.Run("oob_node", func(t *testing.T) {
		original := ModelsOobNodeDetailsResponse{NodeUUID: "node-2", DataSource: "oob", Location: &ModelsLocation{Latitude: &finite}}
		invalid := ModelsOobNodeDetailsResponse{NodeUUID: "node-2", DataSource: "oob", Location: &ModelsLocation{Latitude: &nonfinite}}
		testRound2ResourceUnionNonfiniteMerge(t, original, invalid,
			(*GetV1NodesNodeUuid200JSONResponseBody).FromModelsOobNodeDetailsResponse,
			(*GetV1NodesNodeUuid200JSONResponseBody).MergeModelsOobNodeDetailsResponse,
			(*GetV1NodesNodeUuid200JSONResponseBody).AsModelsOobNodeDetailsResponse)
	})
}

func TestRound2ListUnionNonfiniteMergeContracts(t *testing.T) {
	finite := float32(39.0458)
	nonfinite := float32(math.NaN())
	t.Run("compute_zones", func(t *testing.T) {
		originalZones := []ModelsSimpleComputeZone{{Id: "zone-1", Location: &ModelsLocation{Latitude: &finite}}}
		invalidZones := []ModelsSimpleComputeZone{{Id: "zone-1", Location: &ModelsLocation{Latitude: &nonfinite}}}
		testRound2ResourceUnionNonfiniteMerge(t,
			ModelsListComputeZonesResponse{Computezones: &originalZones}, ModelsListComputeZonesResponse{Computezones: &invalidZones},
			(*GetV1Computezones200JSONResponseBody).FromModelsListComputeZonesResponse,
			(*GetV1Computezones200JSONResponseBody).MergeModelsListComputeZonesResponse,
			(*GetV1Computezones200JSONResponseBody).AsModelsListComputeZonesResponse)
	})
	t.Run("inband_nodes", func(t *testing.T) {
		originalEar := map[string]interface{}{"value": float64(39.0458)}
		invalidEar := map[string]interface{}{"value": math.NaN()}
		originalNodes := []ModelsInbandNode{{NodeUUID: "node-1", DataSource: "inband", VerificationCheckExtraInfo: &ModelsVerificationCheckExtraInfo{Ear: &originalEar}}}
		invalidNodes := []ModelsInbandNode{{NodeUUID: "node-1", DataSource: "inband", VerificationCheckExtraInfo: &ModelsVerificationCheckExtraInfo{Ear: &invalidEar}}}
		testRound2ResourceUnionNonfiniteMerge(t,
			ModelsInbandNodesResponse{Nodes: &originalNodes}, ModelsInbandNodesResponse{Nodes: &invalidNodes},
			(*GetV1Nodes200JSONResponseBody).FromModelsInbandNodesResponse,
			(*GetV1Nodes200JSONResponseBody).MergeModelsInbandNodesResponse,
			(*GetV1Nodes200JSONResponseBody).AsModelsInbandNodesResponse)
	})
	t.Run("oob_nodes", func(t *testing.T) {
		originalEar := map[string]interface{}{"value": float64(39.0458)}
		invalidEar := map[string]interface{}{"value": math.NaN()}
		originalNodes := []ModelsOobNode{{NodeUUID: "node-2", DataSource: "oob", VerificationCheckExtraInfo: &ModelsVerificationCheckExtraInfo{Ear: &originalEar}}}
		invalidNodes := []ModelsOobNode{{NodeUUID: "node-2", DataSource: "oob", VerificationCheckExtraInfo: &ModelsVerificationCheckExtraInfo{Ear: &invalidEar}}}
		testRound2ResourceUnionNonfiniteMerge(t,
			ModelsOobNodesResponse{Nodes: &originalNodes}, ModelsOobNodesResponse{Nodes: &invalidNodes},
			(*GetV1Nodes200JSONResponseBody).FromModelsOobNodesResponse,
			(*GetV1Nodes200JSONResponseBody).MergeModelsOobNodesResponse,
			(*GetV1Nodes200JSONResponseBody).AsModelsOobNodesResponse)
	})
}

func TestRound2InventoryUnionNonfiniteMergeContracts(t *testing.T) {
	finite := float32(39.0458)
	nonfinite := float32(math.NaN())
	t.Run("inband", func(t *testing.T) {
		originalNodes := []ModelsInbandInventoryNode{{Location: &ModelsLocation{Latitude: &finite}}}
		invalidNodes := []ModelsInbandInventoryNode{{Location: &ModelsLocation{Latitude: &nonfinite}}}
		testRound2ResourceUnionNonfiniteMerge(t,
			ModelsInbandInventoryReportResponse{Nodes: &originalNodes}, ModelsInbandInventoryReportResponse{Nodes: &invalidNodes},
			(*GetV1ReportsInventory200JSONResponseBody).FromModelsInbandInventoryReportResponse,
			(*GetV1ReportsInventory200JSONResponseBody).MergeModelsInbandInventoryReportResponse,
			(*GetV1ReportsInventory200JSONResponseBody).AsModelsInbandInventoryReportResponse)
	})
	t.Run("oob", func(t *testing.T) {
		originalNodes := []ModelsOobInventoryNode{{Location: &ModelsLocation{Latitude: &finite}}}
		invalidNodes := []ModelsOobInventoryNode{{Location: &ModelsLocation{Latitude: &nonfinite}}}
		testRound2ResourceUnionNonfiniteMerge(t,
			ModelsOobInventoryReportResponse{Nodes: &originalNodes}, ModelsOobInventoryReportResponse{Nodes: &invalidNodes},
			(*GetV1ReportsInventory200JSONResponseBody).FromModelsOobInventoryReportResponse,
			(*GetV1ReportsInventory200JSONResponseBody).MergeModelsOobInventoryReportResponse,
			(*GetV1ReportsInventory200JSONResponseBody).AsModelsOobInventoryReportResponse)
	})
}

type round2Enum interface {
	~string
	Valid() bool
}

type round2EnumValue[T round2Enum] struct {
	constant T
	literal  string
}

func testRound2EnumContract[T round2Enum](t *testing.T, values []round2EnumValue[T], wrongCase string) {
	t.Helper()
	for _, value := range values {
		t.Run(value.literal, func(t *testing.T) {
			if got := string(value.constant); got != value.literal {
				t.Errorf("constant = %q, want schema value %q", got, value.literal)
			}
			if !value.constant.Valid() {
				t.Errorf("constant %q is not valid", value.constant)
			}
			if literal := T(value.literal); !literal.Valid() {
				t.Errorf("schema literal %q is not valid when converted to enum", value.literal)
			}
		})
	}
	for _, invalid := range []string{"", "definitely_unknown_value", wrongCase} {
		if T(invalid).Valid() {
			t.Errorf("invalid value %q was accepted", invalid)
		}
	}
}

func TestRound2ModelEnumValidContracts(t *testing.T) {
	// Values and constant names are specified by the models schemas in api/openapi/openapi.yaml.
	t.Run("ActionPersona", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsActionPersona]{
			{ActionPersonaTenant, "tenant"}, {ActionPersonaDCAdmin, "dc_admin"},
		}, "Tenant")
	})
	t.Run("ActionType", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsActionType]{
			{ActionTypeImmediate, "immediate"}, {ActionTypeInvestigatory, "investigatory"},
		}, "Immediate")
	})
	t.Run("AgentStatus", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsAgentStatus]{
			{AgentStatusOnline, "Online"}, {AgentStatusOffline, "Offline"},
		}, "online")
	})
	t.Run("Aggregation", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsAggregation]{
			{AggregationAverage, "average"}, {AggregationSum, "sum"}, {AggregationMin, "min"},
			{AggregationMax, "max"}, {AggregationNone, "none"},
		}, "Average")
	})
	t.Run("AlertAgentStatus", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsAlertAgentStatus]{
			{AlertAgentStatusOnline, "online"}, {AlertAgentStatusOffline, "offline"},
			{AlertAgentStatusNotEnrolled, "not_enrolled"},
		}, "Online")
	})
	t.Run("AlertEventType", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsAlertEventType]{
			{AlertEventDetected, "detected"}, {AlertEventTriggered, "triggered"},
			{AlertEventResolved, "resolved"}, {AlertEventRetriggered, "retriggered"},
			{AlertEventSeverityChanged, "severity_changed"}, {AlertEventMetadataUpdated, "metadata_updated"},
			{AlertEventNotifySent, "notify_sent"}, {AlertEventNotifyFailed, "notify_failed"},
			{AlertEventMuted, "muted"}, {AlertEventUnmuted, "unmuted"},
			{AlertEventBurstAnalyzed, "burst_analyzed"},
		}, "Detected")
	})
	t.Run("AlertSeverity", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsAlertSeverity]{
			{AlertSeverityCritical, "Critical"}, {AlertSeverityWarning, "Warning"},
		}, "critical")
	})
	t.Run("AlertState", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsAlertState]{
			{AlertStateDetected, "Detected"}, {AlertStateTriggered, "Triggered"}, {AlertStateResolved, "Resolved"},
		}, "detected")
	})
	t.Run("Audience", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsAudience]{
			{AudienceDefault, "default"}, {AudienceExternalCloudProvider, "external_cloud_provider"},
			{AudienceForbidden, "forbidden"},
		}, "Default")
	})
	t.Run("DeliveryStatus", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsDeliveryStatus]{
			{DeliveryStatusSent, "sent"}, {DeliveryStatusFailed, "failed"},
		}, "Sent")
	})
	t.Run("HealthState", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsHealthState]{
			{HealthStateHealthy, "Healthy"}, {HealthStateDegraded, "Degraded"},
			{HealthStateUnhealthy, "Unhealthy"}, {HealthStateUnknown, "Unknown"},
			{HealthStateUnsupported, "Unsupported"}, {HealthStatePending, "Pending"},
		}, "healthy")
	})
	t.Run("NotificationChannel", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsNotificationChannel]{
			{NotificationChannelEmail, "email"}, {NotificationChannelSlack, "slack"},
			{NotificationChannelWebhook, "webhook"},
		}, "Email")
	})
	t.Run("NotificationScopeType", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsNotificationScopeType]{
			{ScopeAll, "all"}, {ScopeComputeZones, "compute_zones"}, {ScopeNodeGroups, "node_groups"},
			{ScopeTags, "tags"}, {ScopeNodes, "nodes"},
		}, "All")
	})
}

func TestRound2QueryEnumValidContracts(t *testing.T) {
	// Literals come from the operation query schemas in api/openapi/openapi.yaml.
	t.Run("inventory_sort_by", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ReportsInventoryParamsSortBy]{
			{GetV1ReportsInventoryParamsSortByHostname, "hostname"},
			{GetV1ReportsInventoryParamsSortByNodeUUID, "nodeUUID"},
			{GetV1ReportsInventoryParamsSortByNodeGroup, "nodeGroup"},
			{GetV1ReportsInventoryParamsSortByComputeZone, "computeZone"},
			{GetV1ReportsInventoryParamsSortByGpuType, "gpuType"},
			{GetV1ReportsInventoryParamsSortByGpuCount, "gpuCount"},
			{GetV1ReportsInventoryParamsSortByPublicIP, "publicIP"},
			{GetV1ReportsInventoryParamsSortByPrivateIP, "privateIP"},
			{GetV1ReportsInventoryParamsSortByNodeName, "nodeName"},
			{GetV1ReportsInventoryParamsSortByBmcMac, "bmcMac"},
			{GetV1ReportsInventoryParamsSortByBmcIP, "bmcIP"},
			{GetV1ReportsInventoryParamsSortByVerificationCheck, "verificationCheck"},
			{GetV1ReportsInventoryParamsSortByLocation, "location"},
			{GetV1ReportsInventoryParamsSortByIntegrityCheck, "integrityCheck"},
			{GetV1ReportsInventoryParamsSortByGeoLocation, "geoLocation"},
		}, "Hostname")
	})
	t.Run("security_vulnerabilities_sort_by", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ReportsSecurityVulnerabilitiesParamsSortBy]{
			{GetV1ReportsSecurityVulnerabilitiesParamsSortByVulnerability, "vulnerability"},
			{GetV1ReportsSecurityVulnerabilitiesParamsSortBySeverity, "severity"},
			{GetV1ReportsSecurityVulnerabilitiesParamsSortByAffectedNodes, "affectedNodes"},
			{GetV1ReportsSecurityVulnerabilitiesParamsSortByComponent, "component"},
		}, "Vulnerability")
	})
	t.Run("stats_group_by", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1StatsParamsGroupBy]{
			{GetV1StatsParamsGroupByCustomer, "customer"},
			{GetV1StatsParamsGroupByNodegroup, "nodegroup"},
			{GetV1StatsParamsGroupByComputezone, "computezone"},
		}, "Customer")
	})
	t.Run("xid_bursts_time_mode", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1XIDBurstsParamsTimeMode]{
			{GetV1XIDBurstsParamsTimeModeAbsolute, "absolute"},
			{GetV1XIDBurstsParamsTimeModeRelative, "relative"},
		}, "Absolute")
	})
	t.Run("xid_burst_options_time_mode", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1XIDBurstOptionsParamsTimeMode]{
			{GetV1XIDBurstOptionsParamsTimeModeAbsolute, "absolute"},
			{GetV1XIDBurstOptionsParamsTimeModeRelative, "relative"},
		}, "Absolute")
	})
	t.Run("xid_burst_options_data_source", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1XIDBurstOptionsParamsDataSource]{
			{GetV1XIDBurstOptionsParamsDataSourceInband, "inband"},
			{GetV1XIDBurstOptionsParamsDataSourceOob, "oob"},
			{GetV1XIDBurstOptionsParamsDataSourceAll, "all"},
		}, "Inband")
	})
	t.Run("v2_metrics_group_by", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV2MetricsParamsGroupBy]{
			{GetV2MetricsParamsGroupByCustomer, "customer"},
			{GetV2MetricsParamsGroupByNodegroup, "nodegroup"},
			{GetV2MetricsParamsGroupByComputezone, "computezone"},
			{GetV2MetricsParamsGroupByNode, "node"},
		}, "Customer")
	})
	t.Run("v2_metrics_time_mode", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV2MetricsParamsTimeMode]{
			{GetV2MetricsParamsTimeModeAbsolute, "absolute"},
			{GetV2MetricsParamsTimeModeRelative, "relative"},
		}, "Absolute")
	})
}

type round2ResponseMetadata interface {
	GetBody() []byte
	Status() string
	StatusCode() int
	ContentType() string
}

func assertRound2ResponseMetadata(t *testing.T, response round2ResponseMetadata, body []byte, status string, code int, contentType string) {
	t.Helper()
	if body == nil && response.GetBody() != nil {
		t.Errorf("GetBody() = %v, want nil", response.GetBody())
	} else if !bytes.Equal(response.GetBody(), body) {
		t.Errorf("GetBody() = %q, want %q", response.GetBody(), body)
	}
	if got := response.Status(); got != status {
		t.Errorf("Status() = %q, want %q", got, status)
	}
	if got := response.StatusCode(); got != code {
		t.Errorf("StatusCode() = %d, want %d", got, code)
	}
	if got := response.ContentType(); got != contentType {
		t.Errorf("ContentType() = %q, want %q", got, contentType)
	}
}

func assertRound2ResponsePointer[T any](t *testing.T, name string, got, want *T) {
	t.Helper()
	if got != want {
		t.Errorf("%s() = %p, want %p", name, got, want)
	}
}

func TestRound2ResponseAccessorContracts(t *testing.T) {
	t.Run("signing_key", func(t *testing.T) {
		var zero GetWellKnownSigningKeyPubResponse
		assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
		assertRound2ResponsePointer(t, "GetJSON499", zero.GetJSON499(), nil)
		assertRound2ResponsePointer(t, "GetJSON503", zero.GetJSON503(), nil)
		body := []byte("signing_key response body")
		httpResponse := &http.Response{Status: "403 Forbidden", StatusCode: 403, Header: http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}}
		value499 := &ModelsErrorResponse{}
		value503 := &ModelsErrorResponse{}
		populated := GetWellKnownSigningKeyPubResponse{Body: body, HTTPResponse: httpResponse,
			JSON499: value499,
			JSON503: value503,
		}
		assertRound2ResponseMetadata(t, populated, body, "403 Forbidden", 403, "application/json; charset=utf-8")
		assertRound2ResponsePointer(t, "GetJSON499", populated.GetJSON499(), value499)
		assertRound2ResponsePointer(t, "GetJSON503", populated.GetJSON503(), value503)
		singleError := GetWellKnownSigningKeyPubResponse{JSON499: value499}
		assertRound2ResponsePointer(t, "GetJSON499", singleError.GetJSON499(), value499)
		assertRound2ResponsePointer(t, "GetJSON503", singleError.GetJSON503(), nil)
	})
	t.Run("delete", func(t *testing.T) {
		var zero DeleteV1AlertMuteRulesResponse
		assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
		assertRound2ResponsePointer(t, "GetJSON400", zero.GetJSON400(), nil)
		assertRound2ResponsePointer(t, "GetJSON401", zero.GetJSON401(), nil)
		assertRound2ResponsePointer(t, "GetJSON403", zero.GetJSON403(), nil)
		assertRound2ResponsePointer(t, "GetJSON404", zero.GetJSON404(), nil)
		assertRound2ResponsePointer(t, "GetJSON499", zero.GetJSON499(), nil)
		assertRound2ResponsePointer(t, "GetJSON500", zero.GetJSON500(), nil)
		body := []byte("delete response body")
		httpResponse := &http.Response{Status: "403 Forbidden", StatusCode: 403, Header: http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}}
		value400 := &ModelsErrorResponse{}
		value401 := &ModelsErrorResponse{}
		value403 := &ModelsErrorResponse{}
		value404 := &ModelsErrorResponse{}
		value499 := &ModelsErrorResponse{}
		value500 := &ModelsErrorResponse{}
		populated := DeleteV1AlertMuteRulesResponse{Body: body, HTTPResponse: httpResponse,
			JSON400: value400,
			JSON401: value401,
			JSON403: value403,
			JSON404: value404,
			JSON499: value499,
			JSON500: value500,
		}
		assertRound2ResponseMetadata(t, populated, body, "403 Forbidden", 403, "application/json; charset=utf-8")
		assertRound2ResponsePointer(t, "GetJSON400", populated.GetJSON400(), value400)
		assertRound2ResponsePointer(t, "GetJSON401", populated.GetJSON401(), value401)
		assertRound2ResponsePointer(t, "GetJSON403", populated.GetJSON403(), value403)
		assertRound2ResponsePointer(t, "GetJSON404", populated.GetJSON404(), value404)
		assertRound2ResponsePointer(t, "GetJSON499", populated.GetJSON499(), value499)
		assertRound2ResponsePointer(t, "GetJSON500", populated.GetJSON500(), value500)
		singleError := DeleteV1AlertMuteRulesResponse{JSON403: value403}
		assertRound2ResponsePointer(t, "GetJSON400", singleError.GetJSON400(), nil)
		assertRound2ResponsePointer(t, "GetJSON401", singleError.GetJSON401(), nil)
		assertRound2ResponsePointer(t, "GetJSON403", singleError.GetJSON403(), value403)
		assertRound2ResponsePointer(t, "GetJSON404", singleError.GetJSON404(), nil)
		assertRound2ResponsePointer(t, "GetJSON499", singleError.GetJSON499(), nil)
		assertRound2ResponsePointer(t, "GetJSON500", singleError.GetJSON500(), nil)
	})
	t.Run("list", func(t *testing.T) {
		var zero GetV1AlertMuteRulesResponse
		assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
		assertRound2ResponsePointer(t, "GetJSON200", zero.GetJSON200(), nil)
		assertRound2ResponsePointer(t, "GetJSON401", zero.GetJSON401(), nil)
		assertRound2ResponsePointer(t, "GetJSON403", zero.GetJSON403(), nil)
		assertRound2ResponsePointer(t, "GetJSON499", zero.GetJSON499(), nil)
		assertRound2ResponsePointer(t, "GetJSON500", zero.GetJSON500(), nil)
		body := []byte("list response body")
		httpResponse := &http.Response{Status: "403 Forbidden", StatusCode: 403, Header: http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}}
		value200 := &ModelsListMuteRulesResponse{}
		value401 := &ModelsErrorResponse{}
		value403 := &ModelsErrorResponse{}
		value499 := &ModelsErrorResponse{}
		value500 := &ModelsErrorResponse{}
		populated := GetV1AlertMuteRulesResponse{Body: body, HTTPResponse: httpResponse,
			JSON200: value200,
			JSON401: value401,
			JSON403: value403,
			JSON499: value499,
			JSON500: value500,
		}
		assertRound2ResponseMetadata(t, populated, body, "403 Forbidden", 403, "application/json; charset=utf-8")
		assertRound2ResponsePointer(t, "GetJSON200", populated.GetJSON200(), value200)
		assertRound2ResponsePointer(t, "GetJSON401", populated.GetJSON401(), value401)
		assertRound2ResponsePointer(t, "GetJSON403", populated.GetJSON403(), value403)
		assertRound2ResponsePointer(t, "GetJSON499", populated.GetJSON499(), value499)
		assertRound2ResponsePointer(t, "GetJSON500", populated.GetJSON500(), value500)
		singleError := GetV1AlertMuteRulesResponse{JSON403: value403}
		assertRound2ResponsePointer(t, "GetJSON200", singleError.GetJSON200(), nil)
		assertRound2ResponsePointer(t, "GetJSON401", singleError.GetJSON401(), nil)
		assertRound2ResponsePointer(t, "GetJSON403", singleError.GetJSON403(), value403)
		assertRound2ResponsePointer(t, "GetJSON499", singleError.GetJSON499(), nil)
		assertRound2ResponsePointer(t, "GetJSON500", singleError.GetJSON500(), nil)
	})
	t.Run("put", func(t *testing.T) {
		var zero PutV1AlertMuteRulesResponse
		assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
		assertRound2ResponsePointer(t, "GetJSON400", zero.GetJSON400(), nil)
		assertRound2ResponsePointer(t, "GetJSON401", zero.GetJSON401(), nil)
		assertRound2ResponsePointer(t, "GetJSON403", zero.GetJSON403(), nil)
		assertRound2ResponsePointer(t, "GetJSON499", zero.GetJSON499(), nil)
		assertRound2ResponsePointer(t, "GetJSON500", zero.GetJSON500(), nil)
		body := []byte("put response body")
		httpResponse := &http.Response{Status: "403 Forbidden", StatusCode: 403, Header: http.Header{"Content-Type": []string{"application/json; charset=utf-8"}}}
		value400 := &ModelsErrorResponse{}
		value401 := &ModelsErrorResponse{}
		value403 := &ModelsErrorResponse{}
		value499 := &ModelsErrorResponse{}
		value500 := &ModelsErrorResponse{}
		populated := PutV1AlertMuteRulesResponse{Body: body, HTTPResponse: httpResponse,
			JSON400: value400,
			JSON401: value401,
			JSON403: value403,
			JSON499: value499,
			JSON500: value500,
		}
		assertRound2ResponseMetadata(t, populated, body, "403 Forbidden", 403, "application/json; charset=utf-8")
		assertRound2ResponsePointer(t, "GetJSON400", populated.GetJSON400(), value400)
		assertRound2ResponsePointer(t, "GetJSON401", populated.GetJSON401(), value401)
		assertRound2ResponsePointer(t, "GetJSON403", populated.GetJSON403(), value403)
		assertRound2ResponsePointer(t, "GetJSON499", populated.GetJSON499(), value499)
		assertRound2ResponsePointer(t, "GetJSON500", populated.GetJSON500(), value500)
		singleError := PutV1AlertMuteRulesResponse{JSON403: value403}
		assertRound2ResponsePointer(t, "GetJSON400", singleError.GetJSON400(), nil)
		assertRound2ResponsePointer(t, "GetJSON401", singleError.GetJSON401(), nil)
		assertRound2ResponsePointer(t, "GetJSON403", singleError.GetJSON403(), value403)
		assertRound2ResponsePointer(t, "GetJSON499", singleError.GetJSON499(), nil)
		assertRound2ResponsePointer(t, "GetJSON500", singleError.GetJSON500(), nil)
	})
}

type round2AlertTimelineErrorReader struct {
	err    error
	closed bool
}

func (r *round2AlertTimelineErrorReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

func (r *round2AlertTimelineErrorReader) Close() error {
	r.closed = true
	return nil
}

func assertRound2AlertTimelineError(t *testing.T, status int, getters map[int]*ModelsErrorResponse) {
	t.Helper()
	for code, value := range getters {
		if code != status {
			assertRound2ResponsePointer(t, fmt.Sprintf("GetJSON%d", code), value, nil)
			continue
		}
		if value == nil {
			t.Errorf("GetJSON%d() = nil, want decoded error", code)
			continue
		}
		if value.Error == nil || *value.Error != "denied" {
			t.Errorf("GetJSON%d().Error = %v, want denied", code, value.Error)
		}
		if value.Details == nil || *value.Details != "specific detail" {
			t.Errorf("GetJSON%d().Details = %v, want specific detail", code, value.Details)
		}
		if value.RequestStatus != nil {
			t.Errorf("GetJSON%d().RequestStatus = %v, want nil", code, value.RequestStatus)
		}
	}
}

func TestRound2AlertTimelineResponseContracts(t *testing.T) {
	const contentType = "application/json; charset=utf-8"
	body := []byte(`{"error":"denied","details":"specific detail"}`)
	response := func(status int, reader io.ReadCloser) *http.Response {
		return &http.Response{Status: fmt.Sprintf("%d supplied status", status), StatusCode: status,
			Header: http.Header{"Content-Type": []string{contentType}}, Body: reader}
	}

	t.Run("filter_options", func(t *testing.T) {
		var zero GetV1AlertTimelineFilterOptionsResponse
		assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
		assertRound2ResponsePointer(t, "GetJSON200", zero.GetJSON200(), nil)
		assertRound2ResponsePointer(t, "GetJSON401", zero.GetJSON401(), nil)
		assertRound2ResponsePointer(t, "GetJSON499", zero.GetJSON499(), nil)
		assertRound2ResponsePointer(t, "GetJSON500", zero.GetJSON500(), nil)
		for _, status := range []int{499, 500} {
			t.Run(fmt.Sprint(status), func(t *testing.T) {
				rsp, err := ParseGetV1AlertTimelineFilterOptionsResponse(response(status, io.NopCloser(bytes.NewReader(body))))
				if err != nil || rsp == nil {
					t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
				}
				assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, contentType)
				assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
				assertRound2AlertTimelineError(t, status, map[int]*ModelsErrorResponse{401: rsp.GetJSON401(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()})
			})
		}
		t.Run("malformed", func(t *testing.T) {
			rsp, err := ParseGetV1AlertTimelineFilterOptionsResponse(response(499, io.NopCloser(bytes.NewBufferString(`{"error":`))))
			if err == nil || rsp != nil {
				t.Errorf("parse = %v, %v; want nil response and decoding error", rsp, err)
			}
		})
		t.Run("read_error", func(t *testing.T) {
			sentinel := errors.New("filter options read error")
			reader := &round2AlertTimelineErrorReader{err: sentinel}
			rsp, err := ParseGetV1AlertTimelineFilterOptionsResponse(response(499, reader))
			if !errors.Is(err, sentinel) || rsp != nil || !reader.closed {
				t.Errorf("parse = %v, %v; closed = %t; want nil response, sentinel error and closed body", rsp, err, reader.closed)
			}
		})
	})

	t.Run("nodes", func(t *testing.T) {
		var zero GetV1AlertTimelineNodesResponse
		assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
		assertRound2ResponsePointer(t, "GetJSON200", zero.GetJSON200(), nil)
		assertRound2ResponsePointer(t, "GetJSON400", zero.GetJSON400(), nil)
		assertRound2ResponsePointer(t, "GetJSON401", zero.GetJSON401(), nil)
		assertRound2ResponsePointer(t, "GetJSON499", zero.GetJSON499(), nil)
		assertRound2ResponsePointer(t, "GetJSON500", zero.GetJSON500(), nil)
		for _, status := range []int{400, 499, 500} {
			t.Run(fmt.Sprint(status), func(t *testing.T) {
				rsp, err := ParseGetV1AlertTimelineNodesResponse(response(status, io.NopCloser(bytes.NewReader(body))))
				if err != nil || rsp == nil {
					t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
				}
				assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, contentType)
				assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
				assertRound2AlertTimelineError(t, status, map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()})
			})
		}
		t.Run("malformed", func(t *testing.T) {
			rsp, err := ParseGetV1AlertTimelineNodesResponse(response(400, io.NopCloser(bytes.NewBufferString(`{"error":`))))
			if err == nil || rsp != nil {
				t.Errorf("parse = %v, %v; want nil response and decoding error", rsp, err)
			}
		})
		t.Run("read_error", func(t *testing.T) {
			sentinel := errors.New("nodes read error")
			reader := &round2AlertTimelineErrorReader{err: sentinel}
			rsp, err := ParseGetV1AlertTimelineNodesResponse(response(400, reader))
			if !errors.Is(err, sentinel) || rsp != nil || !reader.closed {
				t.Errorf("parse = %v, %v; closed = %t; want nil response, sentinel error and closed body", rsp, err, reader.closed)
			}
		})
	})
}

// testRound2ReportUnion checks the typed conversion and wire representation at
// each stage, including that a partial merge preserves the remaining fields.
func testRound2ReportUnion[M any, U any](t *testing.T, initial, patch, merged, invalid string,
	from func(*U, M) error, as func(*U) (M, error), merge func(*U, M) error) {
	t.Helper()
	decodeModel := func(input string) M {
		t.Helper()
		var model M
		if err := json.Unmarshal([]byte(input), &model); err != nil {
			t.Fatalf("unmarshal model: %v", err)
		}
		return model
	}
	check := func(stage string, union *U, expected string) {
		t.Helper()
		got, err := as(union)
		if err != nil {
			t.Fatalf("%s As: %v", stage, err)
		}
		want := decodeModel(expected)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s decoded model = %+v, want %+v", stage, got, want)
		}
		encoded, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("%s marshal model: %v", stage, err)
		}
		var gotJSON, wantJSON any
		if err := json.Unmarshal(encoded, &gotJSON); err != nil {
			t.Fatalf("%s unmarshal encoded model: %v", stage, err)
		}
		if err := json.Unmarshal([]byte(expected), &wantJSON); err != nil {
			t.Fatalf("%s unmarshal expected JSON: %v", stage, err)
		}
		if !reflect.DeepEqual(gotJSON, wantJSON) {
			t.Errorf("%s fields = %s, want %s", stage, encoded, expected)
		}
	}

	var union U
	if err := from(&union, decodeModel(initial)); err != nil {
		t.Fatalf("From: %v", err)
	}
	check("From", &union, initial)
	if err := merge(&union, decodeModel(patch)); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	check("Merge", &union, merged)
	encoded, err := json.Marshal(union)
	if err != nil {
		t.Fatalf("marshal union: %v", err)
	}
	var roundTrip U
	if err := json.Unmarshal(encoded, &roundTrip); err != nil {
		t.Fatalf("unmarshal union: %v", err)
	}
	check("round trip", &roundTrip, merged)

	var incompatible U
	if err := json.Unmarshal([]byte(invalid), &incompatible); err != nil {
		t.Fatalf("unmarshal incompatible union: %v", err)
	}
	if _, err := as(&incompatible); err == nil {
		t.Error("As accepted an incompatible field type")
	}
}

func TestRound2ReportResponseUnionContracts(t *testing.T) {
	t.Run("inband", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"nodes":[{"nodeUUID":"inband-1","hostname":"host-1"}],"page":2,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":3}`,
			`{"nodes":[{"nodeUUID":"inband-1","hostname":"host-1"}],"page":3,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":"invalid"}`,
			(*GetV1ReportsInventory200JSONResponseBody).FromModelsInbandInventoryReportResponse,
			(*GetV1ReportsInventory200JSONResponseBody).AsModelsInbandInventoryReportResponse,
			(*GetV1ReportsInventory200JSONResponseBody).MergeModelsInbandInventoryReportResponse)
	})
	t.Run("oob", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"nodes":[{"nodeUUID":"oob-1","nodeName":"server-1","bmcMac":"00:11:22:33:44:55"}],"page":2,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":3}`,
			`{"nodes":[{"nodeUUID":"oob-1","nodeName":"server-1","bmcMac":"00:11:22:33:44:55"}],"page":3,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":"invalid"}`,
			(*GetV1ReportsInventory200JSONResponseBody).FromModelsOobInventoryReportResponse,
			(*GetV1ReportsInventory200JSONResponseBody).AsModelsOobInventoryReportResponse,
			(*GetV1ReportsInventory200JSONResponseBody).MergeModelsOobInventoryReportResponse)
	})
	t.Run("by_error", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"errors":[{"name":"xid_154","count":7}],"page":2,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":3}`,
			`{"errors":[{"name":"xid_154","count":7}],"page":3,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":"invalid"}`,
			(*GetV1ReportsError200JSONResponseBody).FromModelsByErrorResponse,
			(*GetV1ReportsError200JSONResponseBody).AsModelsByErrorResponse,
			(*GetV1ReportsError200JSONResponseBody).MergeModelsByErrorResponse)
	})
	t.Run("by_node", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"nodes":[{"nodeUUID":"node-1","errors":["xid_154"]}],"page":2,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":3}`,
			`{"nodes":[{"nodeUUID":"node-1","errors":["xid_154"]}],"page":3,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":"invalid"}`,
			(*GetV1ReportsError200JSONResponseBody).FromModelsByNodeResponse,
			(*GetV1ReportsError200JSONResponseBody).AsModelsByNodeResponse,
			(*GetV1ReportsError200JSONResponseBody).MergeModelsByNodeResponse)
	})
	t.Run("overview", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"totalErrors":12,"totalErrorNodes":4,"totalErrorTypes":3}`,
			`{"totalErrors":13}`,
			`{"totalErrors":13,"totalErrorNodes":4,"totalErrorTypes":3}`,
			`{"totalErrors":"invalid"}`,
			(*GetV1ReportsError200JSONResponseBody).FromModelsErrorOverviewResponse,
			(*GetV1ReportsError200JSONResponseBody).AsModelsErrorOverviewResponse,
			(*GetV1ReportsError200JSONResponseBody).MergeModelsErrorOverviewResponse)
	})
	t.Run("graph", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"result":[{"label":{"error":"xid_154"},"values":"[[1716153600,5]]"}],"timeRange":{"start":"2026-05-01T00:00:00Z","end":"2026-05-02T00:00:00Z"}}`,
			`{"timeRange":{"end":"2026-05-03T00:00:00Z"}}`,
			`{"result":[{"label":{"error":"xid_154"},"values":"[[1716153600,5]]"}],"timeRange":{"start":"2026-05-01T00:00:00Z","end":"2026-05-03T00:00:00Z"}}`,
			`{"result":"invalid"}`,
			(*GetV1ReportsError200JSONResponseBody).FromModelsErrorReportGraphByErrorResponse,
			(*GetV1ReportsError200JSONResponseBody).AsModelsErrorReportGraphByErrorResponse,
			(*GetV1ReportsError200JSONResponseBody).MergeModelsErrorReportGraphByErrorResponse)
	})
}

func TestRound2ResourceResponseUnionContracts(t *testing.T) {
	t.Run("compute_zones", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"computezones":[{"id":"zone-1","name":"Alpha"}],"page":2,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":3}`,
			`{"computezones":[{"id":"zone-1","name":"Alpha"}],"page":3,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":"invalid"}`,
			(*GetV1Computezones200JSONResponseBody).FromModelsComputeZonesResponse,
			(*GetV1Computezones200JSONResponseBody).AsModelsComputeZonesResponse,
			(*GetV1Computezones200JSONResponseBody).MergeModelsComputeZonesResponse)
	})
	t.Run("list_compute_zones", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"computezones":[{"id":"zone-1","name":"Alpha"}],"page":2,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":3}`,
			`{"computezones":[{"id":"zone-1","name":"Alpha"}],"page":3,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":"invalid"}`,
			(*GetV1Computezones200JSONResponseBody).FromModelsListComputeZonesResponse,
			(*GetV1Computezones200JSONResponseBody).AsModelsListComputeZonesResponse,
			(*GetV1Computezones200JSONResponseBody).MergeModelsListComputeZonesResponse)
	})
	t.Run("node_groups", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"nodeGroups":[{"id":"group-1","name":"Alpha"}],"page":2,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":3}`,
			`{"nodeGroups":[{"id":"group-1","name":"Alpha"}],"page":3,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":"invalid"}`,
			(*GetV1Nodegroups200JSONResponseBody).FromModelsNodeGroupsResponse,
			(*GetV1Nodegroups200JSONResponseBody).AsModelsNodeGroupsResponse,
			(*GetV1Nodegroups200JSONResponseBody).MergeModelsNodeGroupsResponse)
	})
	t.Run("list_node_groups", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"nodeGroups":[{"id":"group-1","name":"Alpha"}],"page":2,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":3}`,
			`{"nodeGroups":[{"id":"group-1","name":"Alpha"}],"page":3,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":"invalid"}`,
			(*GetV1Nodegroups200JSONResponseBody).FromModelsListNodeGroupsResponse,
			(*GetV1Nodegroups200JSONResponseBody).AsModelsListNodeGroupsResponse,
			(*GetV1Nodegroups200JSONResponseBody).MergeModelsListNodeGroupsResponse)
	})
	t.Run("inband", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"nodeUUID":"node-1","dataSource":"inband","hostname":"host-1","gpuCount":4}`,
			`{"nodeUUID":"node-1","dataSource":"inband","hostname":"host-2"}`,
			`{"nodeUUID":"node-1","dataSource":"inband","hostname":"host-2","gpuCount":4}`,
			`{"gpuCount":"invalid"}`,
			(*GetV1NodesNodeUuid200JSONResponseBody).FromModelsInbandNodeDetailsResponse,
			(*GetV1NodesNodeUuid200JSONResponseBody).AsModelsInbandNodeDetailsResponse,
			(*GetV1NodesNodeUuid200JSONResponseBody).MergeModelsInbandNodeDetailsResponse)
	})
	t.Run("oob", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"nodeUUID":"node-1","dataSource":"oob","nodeName":"machine-1","bmcIP":"192.0.2.1"}`,
			`{"nodeUUID":"node-1","dataSource":"oob","nodeName":"machine-2"}`,
			`{"nodeUUID":"node-1","dataSource":"oob","nodeName":"machine-2","bmcIP":"192.0.2.1"}`,
			`{"gpuCount":"invalid"}`,
			(*GetV1NodesNodeUuid200JSONResponseBody).FromModelsOobNodeDetailsResponse,
			(*GetV1NodesNodeUuid200JSONResponseBody).AsModelsOobNodeDetailsResponse,
			(*GetV1NodesNodeUuid200JSONResponseBody).MergeModelsOobNodeDetailsResponse)
	})
}

func TestRound2NodesAndSecurityResponseUnionContracts(t *testing.T) {
	t.Run("nodes_inband", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"nodes":[{"nodeUUID":"node-inband","dataSource":"inband","hostname":"host-1","gpuCount":0}],"page":2,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":3}`,
			`{"nodes":[{"nodeUUID":"node-inband","dataSource":"inband","hostname":"host-1","gpuCount":0}],"page":3,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":"invalid"}`,
			(*GetV1Nodes200JSONResponseBody).FromModelsInbandNodesResponse,
			(*GetV1Nodes200JSONResponseBody).AsModelsInbandNodesResponse,
			(*GetV1Nodes200JSONResponseBody).MergeModelsInbandNodesResponse)
	})
	t.Run("nodes_oob", func(t *testing.T) {
		testRound2ReportUnion(t,
			`{"nodes":[{"nodeUUID":"node-oob","dataSource":"oob","nodeName":"rack-1","bmcIP":"192.0.2.1","gpuCount":8}],"page":2,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":3}`,
			`{"nodes":[{"nodeUUID":"node-oob","dataSource":"oob","nodeName":"rack-1","bmcIP":"192.0.2.1","gpuCount":8}],"page":3,"pageSize":25,"total":51,"hasMore":true}`,
			`{"page":"invalid"}`,
			(*GetV1Nodes200JSONResponseBody).FromModelsOobNodesResponse,
			(*GetV1Nodes200JSONResponseBody).AsModelsOobNodesResponse,
			(*GetV1Nodes200JSONResponseBody).MergeModelsOobNodesResponse)
	})
	t.Run("security_full", func(t *testing.T) {
		const initial = `{"vulnerabilities":[{"vulnerability":"CVE-2026-1234","vulnerabilityUrl":"https://example.com/CVE-2026-1234","component":"gpuDriver","severity":"CRITICAL","affectedNodes":7,"fixVersions":["550.1","560.2"],"description":"Driver issue"}],"summary":{"affectedNodes":9,"distinctComponentCount":2,"totalNodes":20,"severityCounts":{"critical":3,"high":4,"medium":5,"low":6}},"page":0,"pageSize":25,"total":30,"hasMore":true}`
		const merged = `{"vulnerabilities":[{"vulnerability":"CVE-2026-1234","vulnerabilityUrl":"https://example.com/CVE-2026-1234","component":"gpuDriver","severity":"CRITICAL","affectedNodes":7,"fixVersions":["550.1","560.2"],"description":"Driver issue"}],"summary":{"affectedNodes":9,"distinctComponentCount":2,"totalNodes":20,"severityCounts":{"critical":3,"high":4,"medium":5,"low":6}},"page":0,"pageSize":25,"total":31,"hasMore":true}`
		testRound2ReportUnion(t, initial, merged, merged, `{"total":"invalid"}`,
			(*GetV1ReportsSecurityVulnerabilities200JSONResponseBody).FromModelsSecurityVulnerabilitiesResponse,
			(*GetV1ReportsSecurityVulnerabilities200JSONResponseBody).AsModelsSecurityVulnerabilitiesResponse,
			(*GetV1ReportsSecurityVulnerabilities200JSONResponseBody).MergeModelsSecurityVulnerabilitiesResponse)
	})
	t.Run("security_count", func(t *testing.T) {
		testRound2ReportUnion(t, `{"total":3}`, `{"total":4}`, `{"total":4}`, `{"total":"invalid"}`,
			(*GetV1ReportsSecurityVulnerabilities200JSONResponseBody).FromGetV1ReportsSecurityVulnerabilities200JSONResponseBody1,
			(*GetV1ReportsSecurityVulnerabilities200JSONResponseBody).AsGetV1ReportsSecurityVulnerabilities200JSONResponseBody1,
			(*GetV1ReportsSecurityVulnerabilities200JSONResponseBody).MergeGetV1ReportsSecurityVulnerabilities200JSONResponseBody1)
	})
}

func testRound2AlertTimelineNodeDetailResponse[T any, R interface {
	comparable
	round2ResponseMetadata
	GetJSON200() *T
	GetJSON400() *ModelsErrorResponse
	GetJSON401() *ModelsErrorResponse
	GetJSON404() *ModelsErrorResponse
	GetJSON499() *ModelsErrorResponse
	GetJSON500() *ModelsErrorResponse
}](t *testing.T, zero R, parse func(*http.Response) (R, error), successBody string, checkSuccess func(*testing.T, *T)) {
	t.Helper()
	const jsonType = "application/json; charset=utf-8"
	makeResponse := func(status int, contentType string, body io.ReadCloser) *http.Response {
		return &http.Response{Status: fmt.Sprintf("%d supplied status", status), StatusCode: status,
			Header: http.Header{"Content-Type": []string{contentType}}, Body: body}
	}
	errorsFor := func(rsp R) map[int]*ModelsErrorResponse {
		return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 404: rsp.GetJSON404(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
	}
	assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
	assertRound2ResponsePointer(t, "GetJSON200", zero.GetJSON200(), nil)
	assertRound2AlertTimelineError(t, 0, errorsFor(zero))

	body := []byte(`{"error":"denied","details":"specific detail"}`)
	for _, status := range []int{400, 401, 404, 499, 500} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			rsp, err := parse(makeResponse(status, jsonType, io.NopCloser(bytes.NewReader(body))))
			var nilResponse R
			if err != nil || rsp == nilResponse {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, jsonType)
			assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
			assertRound2AlertTimelineError(t, status, errorsFor(rsp))
		})
	}
	t.Run("success", func(t *testing.T) {
		body := []byte(successBody)
		rsp, err := parse(makeResponse(http.StatusOK, jsonType, io.NopCloser(bytes.NewReader(body))))
		var nilResponse R
		if err != nil || rsp == nilResponse {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, body, "200 supplied status", 200, jsonType)
		assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
		checkSuccess(t, rsp.GetJSON200())
	})
	t.Run("malformed", func(t *testing.T) {
		rsp, err := parse(makeResponse(400, jsonType, io.NopCloser(bytes.NewBufferString(`{"error":`))))
		var nilResponse R
		if err == nil || rsp != nilResponse {
			t.Errorf("parse = %v, %v; want nil response and decoding error", rsp, err)
		}
	})
	t.Run("read_error", func(t *testing.T) {
		sentinel := errors.New("node detail read error")
		reader := &round2AlertTimelineErrorReader{err: sentinel}
		rsp, err := parse(makeResponse(400, jsonType, reader))
		var nilResponse R
		if !errors.Is(err, sentinel) || rsp != nilResponse || !reader.closed {
			t.Errorf("parse = %v, %v; closed = %t; want nil response, sentinel error and closed body", rsp, err, reader.closed)
		}
	})
	t.Run("text_plain", func(t *testing.T) {
		rsp, err := parse(makeResponse(404, "text/plain", io.NopCloser(bytes.NewReader(body))))
		var nilResponse R
		if err != nil || rsp == nilResponse {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, body, "404 supplied status", 404, "text/plain")
		assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
		assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
	})
}

func TestRound2AlertTimelineNodeDetailResponseContracts(t *testing.T) {
	t.Run("node_alerts", func(t *testing.T) {
		testRound2AlertTimelineNodeDetailResponse(t, &GetV1AlertTimelineNodesNodeUuidAlertsResponse{}, ParseGetV1AlertTimelineNodesNodeUuidAlertsResponse,
			`{"nodeUuid":"node-1","hostname":"host-1","alerts":[{"alertUuid":"alert-1","component":"gpu","dataSource":"inband","alertStatus":"Critical"}],"page":1,"pageSize":10,"total":12,"hasMore":true}`,
			func(t *testing.T, result *ModelsAlertTimelineNodeAlertsResponse) {
				if result == nil || result.Alerts == nil || len(*result.Alerts) != 1 {
					t.Fatalf("missing alerts: %+v", result)
				}
				alert := (*result.Alerts)[0]
				if result.NodeUuid != "node-1" || result.Hostname == nil || *result.Hostname != "host-1" || alert.AlertUuid != "alert-1" || alert.Component == nil || *alert.Component != "gpu" || alert.DataSource == nil || *alert.DataSource != "inband" || alert.AlertStatus == nil || *alert.AlertStatus != "Critical" {
					t.Errorf("node alerts = %+v, alert = %+v", result, alert)
				}
				if result.Page == nil || *result.Page != 1 || result.PageSize == nil || *result.PageSize != 10 || result.Total == nil || *result.Total != 12 || result.HasMore == nil || !*result.HasMore {
					t.Errorf("node alerts pagination = %+v", result)
				}
			})
	})
	t.Run("alert_detail", func(t *testing.T) {
		testRound2AlertTimelineNodeDetailResponse(t, &GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidResponse{}, ParseGetV1AlertTimelineNodesNodeUuidAlertsAlertUuidResponse,
			`{"alertUuid":"alert-1","nodeUuid":"node-1","component":"gpu","page":1,"pageSize":10,"total":12,"hasMore":true,"timeline":[{"eventType":"triggered","message":"GPU critical","extraInfo":{"gpu":"0"}}]}`,
			func(t *testing.T, result *ModelsAlertTimelineDetailResponse) {
				if result == nil || result.Timeline == nil || len(*result.Timeline) != 1 {
					t.Fatalf("missing timeline: %+v", result)
				}
				event := (*result.Timeline)[0]
				if result.AlertUuid != "alert-1" || result.NodeUuid != "node-1" || result.Component == nil || *result.Component != "gpu" || event.EventType == nil || string(*event.EventType) != "triggered" || event.Message == nil || *event.Message != "GPU critical" || event.ExtraInfo == nil || !reflect.DeepEqual(*event.ExtraInfo, map[string]interface{}{"gpu": "0"}) {
					t.Errorf("detail = %+v, event = %+v", result, event)
				}
				if result.Page == nil || *result.Page != 1 || result.PageSize == nil || *result.PageSize != 10 || result.Total == nil || *result.Total != 12 || result.HasMore == nil || !*result.HasMore {
					t.Errorf("detail pagination = %+v", result)
				}
			})
	})
}

func testRound2OverviewEventBucketsResponse[T any, R interface {
	comparable
	round2ResponseMetadata
	GetJSON200() *T
	GetJSON400() *ModelsErrorResponse
	GetJSON401() *ModelsErrorResponse
	GetJSON403() *ModelsErrorResponse
	GetJSON499() *ModelsErrorResponse
	GetJSON500() *ModelsErrorResponse
	GetJSON503() *ModelsErrorResponse
}](t *testing.T, zero R, parse func(*http.Response) (R, error), successBody string, checkSuccess func(*testing.T, *T)) {
	t.Helper()
	const jsonType = "application/json; charset=utf-8"
	makeResponse := func(status int, contentType string, body io.ReadCloser) *http.Response {
		return &http.Response{Status: fmt.Sprintf("%d supplied status", status), StatusCode: status,
			Header: http.Header{"Content-Type": []string{contentType}}, Body: body}
	}
	errorsFor := func(rsp R) map[int]*ModelsErrorResponse {
		return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
	}
	assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
	assertRound2ResponsePointer(t, "GetJSON200", zero.GetJSON200(), nil)
	assertRound2AlertTimelineError(t, 0, errorsFor(zero))

	body := []byte(`{"error":"denied","details":"specific detail"}`)
	for _, status := range []int{400, 401, 403, 499, 500, 503} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			rsp, err := parse(makeResponse(status, jsonType, io.NopCloser(bytes.NewReader(body))))
			var nilResponse R
			if err != nil || rsp == nilResponse {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, jsonType)
			assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
			assertRound2AlertTimelineError(t, status, errorsFor(rsp))
		})
	}
	t.Run("success", func(t *testing.T) {
		body := []byte(successBody)
		rsp, err := parse(makeResponse(200, jsonType, io.NopCloser(bytes.NewReader(body))))
		var nilResponse R
		if err != nil || rsp == nilResponse {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, body, "200 supplied status", 200, jsonType)
		assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
		checkSuccess(t, rsp.GetJSON200())
	})
	for _, status := range []int{200, 400, 401, 403, 499, 500, 503} {
		t.Run(fmt.Sprintf("malformed_%d", status), func(t *testing.T) {
			rsp, err := parse(makeResponse(status, jsonType, io.NopCloser(bytes.NewBufferString(`{"error":`))))
			var nilResponse R
			if err == nil || rsp != nilResponse {
				t.Errorf("parse = %v, %v; want nil response and decoding error", rsp, err)
			}
		})
	}
	t.Run("read_error", func(t *testing.T) {
		sentinel := errors.New("overview or event buckets read error")
		reader := &round2AlertTimelineErrorReader{err: sentinel}
		rsp, err := parse(makeResponse(400, jsonType, reader))
		var nilResponse R
		if !errors.Is(err, sentinel) || rsp != nilResponse || !reader.closed {
			t.Errorf("parse = %v, %v; closed = %t; want nil response, sentinel error and closed body", rsp, err, reader.closed)
		}
	})
	t.Run("text_plain", func(t *testing.T) {
		rsp, err := parse(makeResponse(403, "text/plain", io.NopCloser(bytes.NewReader(body))))
		var nilResponse R
		if err != nil || rsp == nilResponse {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, body, "403 supplied status", 403, "text/plain")
		assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
		assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
	})
}

func TestRound2OverviewAndEventBucketsResponseContracts(t *testing.T) {
	t.Run("overview", func(t *testing.T) {
		testRound2OverviewEventBucketsResponse(t, &GetV1OverviewResponse{}, ParseGetV1OverviewResponse,
			`{"nodesCount":12,"computeZoneCount":2}`, func(t *testing.T, result *ModelsOverviewResponse) {
				if result == nil || result.NodesCount == nil || *result.NodesCount != 12 || result.ComputeZoneCount == nil || *result.ComputeZoneCount != 2 {
					t.Errorf("GetJSON200() = %+v; want nodesCount 12 and computeZoneCount 2", result)
				}
			})
	})
	t.Run("event_buckets", func(t *testing.T) {
		testRound2OverviewEventBucketsResponse(t, &GetV1EventsBucketsResponse{}, ParseGetV1EventsBucketsResponse,
			`{"bucketInterval":"1h","buckets":[{"count":3}]}`, func(t *testing.T, result *ModelsEventBucketsResponse) {
				if result == nil || result.BucketInterval == nil || *result.BucketInterval != "1h" || result.Buckets == nil || len(*result.Buckets) != 1 || (*result.Buckets)[0].Count == nil || *(*result.Buckets)[0].Count != 3 {
					t.Errorf("GetJSON200() = %+v; want bucketInterval 1h and one bucket with count 3", result)
				}
			})
	})
}

func testRound2AlertsResponse[T any, R interface {
	comparable
	round2ResponseMetadata
	GetJSON200() *T
}](t *testing.T, zero R, parse func(*http.Response) (R, error), statuses []int, errorsFor func(R) map[int]*ModelsErrorResponse, successBody string, checkSuccess func(*testing.T, *T)) {
	t.Helper()
	const jsonType = "application/json; charset=utf-8"
	makeResponse := func(status int, contentType string, body io.ReadCloser) *http.Response {
		return &http.Response{Status: fmt.Sprintf("%d supplied status", status), StatusCode: status,
			Header: http.Header{"Content-Type": []string{contentType}}, Body: body}
	}
	assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
	assertRound2ResponsePointer(t, "GetJSON200", zero.GetJSON200(), nil)
	assertRound2AlertTimelineError(t, 0, errorsFor(zero))

	body := []byte(`{"error":"denied","details":"specific detail"}`)
	for _, status := range statuses {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			rsp, err := parse(makeResponse(status, jsonType, io.NopCloser(bytes.NewReader(body))))
			var nilResponse R
			if err != nil || rsp == nilResponse {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, jsonType)
			assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
			assertRound2AlertTimelineError(t, status, errorsFor(rsp))
		})
	}
	t.Run("success", func(t *testing.T) {
		body := []byte(successBody)
		rsp, err := parse(makeResponse(200, jsonType, io.NopCloser(bytes.NewReader(body))))
		var nilResponse R
		if err != nil || rsp == nilResponse {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, body, "200 supplied status", 200, jsonType)
		assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
		checkSuccess(t, rsp.GetJSON200())
	})
	t.Run("malformed", func(t *testing.T) {
		rsp, err := parse(makeResponse(400, jsonType, io.NopCloser(bytes.NewBufferString(`{"error":`))))
		var nilResponse R
		if err == nil || rsp != nilResponse {
			t.Errorf("parse = %v, %v; want nil response and decoding error", rsp, err)
		}
	})
	t.Run("read_error", func(t *testing.T) {
		sentinel := errors.New("alerts read error")
		reader := &round2AlertTimelineErrorReader{err: sentinel}
		rsp, err := parse(makeResponse(400, jsonType, reader))
		var nilResponse R
		if !errors.Is(err, sentinel) || rsp != nilResponse || !reader.closed {
			t.Errorf("parse = %v, %v; closed = %t; want nil response, sentinel error and closed body", rsp, err, reader.closed)
		}
	})
	t.Run("text_plain", func(t *testing.T) {
		rsp, err := parse(makeResponse(400, "text/plain", io.NopCloser(bytes.NewReader(body))))
		var nilResponse R
		if err != nil || rsp == nilResponse {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, body, "400 supplied status", 400, "text/plain")
		assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
		assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
	})
}

func TestRound2AlertsResponseContracts(t *testing.T) {
	// Statuses are declared by the /v1/alerts and /v1/alerts/{nodeUUID} OpenAPI responses.
	t.Run("alerts", func(t *testing.T) {
		testRound2AlertsResponse(t, &GetV1AlertsResponse{}, ParseGetV1AlertsResponse, []int{400, 499, 500},
			func(rsp *GetV1AlertsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			}, `{"alerts":[{"alertUUID":"alert-1","nodeUUID":"node-1"}],"page":2,"total":12}`,
			func(t *testing.T, result *ModelsAlertResponse) {
				t.Helper()
				if result == nil || result.Alerts == nil || len(*result.Alerts) != 1 {
					t.Fatalf("GetJSON200() = %+v; want one alert", result)
				}
				alert := (*result.Alerts)[0]
				if alert.AlertUUID != "alert-1" || alert.NodeUUID != "node-1" || result.Page == nil || *result.Page != 2 || result.Total == nil || *result.Total != 12 {
					t.Errorf("GetJSON200() = %+v, alert = %+v; want alert-1, node-1, page 2 and total 12", result, alert)
				}
			})
	})
	t.Run("node_alerts", func(t *testing.T) {
		testRound2AlertsResponse(t, &GetV1AlertsNodeUuidResponse{}, ParseGetV1AlertsNodeUuidResponse, []int{400, 404, 499, 500},
			func(rsp *GetV1AlertsNodeUuidResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 404: rsp.GetJSON404(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			}, `{"nodeUUID":"node-1","alert":[{"component":"gpu","data":[{"alertUUID":"alert-1","nodeUUID":"node-1"}]}]}`,
			func(t *testing.T, result *ModelsNodeAlertResponse) {
				t.Helper()
				if result == nil || result.Alert == nil || len(*result.Alert) != 1 {
					t.Fatalf("GetJSON200() = %+v; want one alert group", result)
				}
				group := (*result.Alert)[0]
				if group.Data == nil || len(*group.Data) != 1 {
					t.Fatalf("alert group = %+v; want one alert", group)
				}
				alert := (*group.Data)[0]
				if result.NodeUUID != "node-1" || group.Component == nil || *group.Component != "gpu" || alert.AlertUUID != "alert-1" || alert.NodeUUID != "node-1" {
					t.Errorf("GetJSON200() = %+v, group = %+v, alert = %+v; want node-1, gpu and alert-1", result, group, alert)
				}
			})
	})
}

func TestRound2NodeAndNodeGroupResponseContracts(t *testing.T) {
	t.Run("nodegroups", func(t *testing.T) {
		testRound2AlertsResponse(t, &GetV1NodegroupsResponse{}, ParseGetV1NodegroupsResponse, []int{400, 401, 403, 499, 500, 503},
			func(rsp *GetV1NodegroupsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			}, `{"nodeGroups":[{"id":"group-1","name":"Alpha","computeZoneId":"zone-1","nodesCount":7}],"page":2,"pageSize":50,"total":101,"hasMore":true}`,
			func(t *testing.T, result *GetV1Nodegroups200JSONResponseBody) {
				t.Helper()
				if result == nil {
					t.Fatal("GetJSON200() = nil, want node groups")
				}
				groups, err := result.AsModelsNodeGroupsResponse()
				if err != nil {
					t.Fatalf("AsModelsNodeGroupsResponse() error = %v", err)
				}
				if groups.NodeGroups == nil || len(*groups.NodeGroups) != 1 {
					t.Fatalf("node groups = %+v, want one group", groups)
				}
				group := (*groups.NodeGroups)[0]
				if group.Id != "group-1" || group.Name == nil || *group.Name != "Alpha" || group.ComputeZoneId == nil || *group.ComputeZoneId != "zone-1" || group.NodesCount == nil || *group.NodesCount != 7 || groups.Page == nil || *groups.Page != 2 || groups.PageSize == nil || *groups.PageSize != 50 || groups.Total == nil || *groups.Total != 101 || groups.HasMore == nil || !*groups.HasMore {
					t.Errorf("node groups = %+v, group = %+v; want group-1, Alpha, zone-1, 7 nodes, page 2, page size 50, total 101 and hasMore true", groups, group)
				}
			})
	})
	t.Run("nodes", func(t *testing.T) {
		testRound2AlertsResponse(t, &GetV1NodesResponse{}, ParseGetV1NodesResponse, []int{400, 401, 403, 499, 500, 503},
			func(rsp *GetV1NodesResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			}, `{"nodes":[{"nodeUUID":"node-inband","dataSource":"inband","agentUUID":"agent-1","hostname":"host-1","gpuCount":0}],"page":2,"pageSize":50,"total":101,"hasMore":true}`,
			func(t *testing.T, result *GetV1Nodes200JSONResponseBody) {
				t.Helper()
				if result == nil {
					t.Fatal("GetJSON200() = nil, want nodes")
				}
				nodes, err := result.AsModelsInbandNodesResponse()
				if err != nil {
					t.Fatalf("AsModelsInbandNodesResponse() error = %v", err)
				}
				if nodes.Nodes == nil || len(*nodes.Nodes) != 1 {
					t.Fatalf("nodes = %+v, want one node", nodes)
				}
				node := (*nodes.Nodes)[0]
				if node.NodeUUID != "node-inband" || node.DataSource != "inband" || node.AgentUUID == nil || *node.AgentUUID != "agent-1" || node.Hostname == nil || *node.Hostname != "host-1" || node.GpuCount == nil || *node.GpuCount != 0 || nodes.Page == nil || *nodes.Page != 2 || nodes.PageSize == nil || *nodes.PageSize != 50 || nodes.Total == nil || *nodes.Total != 101 || nodes.HasMore == nil || !*nodes.HasMore {
					t.Errorf("nodes = %+v, node = %+v; want node-inband, inband, agent-1, host-1, 0 GPUs, page 2, page size 50, total 101 and hasMore true", nodes, node)
				}
			})
	})
}

func TestRound2AuthAndComputeZoneResponseContracts(t *testing.T) {
	t.Run("get_computezones", func(t *testing.T) {
		testRound2AlertsResponse(t, &GetV1ComputezonesResponse{}, ParseGetV1ComputezonesResponse, []int{400, 401, 403, 499, 500, 503},
			func(rsp *GetV1ComputezonesResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			}, `{"computezones":[{"id":"zone-1","name":"Alpha"}],"page":2,"pageSize":50,"total":101,"hasMore":true}`,
			func(t *testing.T, result *GetV1Computezones200JSONResponseBody) {
				t.Helper()
				if result == nil {
					t.Fatal("GetJSON200() = nil, want compute zones")
				}
				zones, err := result.AsModelsComputeZonesResponse()
				if err != nil {
					t.Fatalf("AsModelsComputeZonesResponse() error = %v", err)
				}
				if zones.Computezones == nil || len(*zones.Computezones) != 1 {
					t.Fatalf("compute zones = %+v, want one zone", zones)
				}
				zone := (*zones.Computezones)[0]
				if zone.Id != "zone-1" || zone.Name == nil || *zone.Name != "Alpha" || zones.Page == nil || *zones.Page != 2 || zones.PageSize == nil || *zones.PageSize != 50 || zones.Total == nil || *zones.Total != 101 || zones.HasMore == nil || !*zones.HasMore {
					t.Errorf("compute zones = %+v, zone = %+v; want zone-1, Alpha, page 2, page size 50, total 101 and hasMore true", zones, zone)
				}
			})
	})
	t.Run("put_computezones", func(t *testing.T) {
		testRound2AlertsResponse(t, &PutV1ComputezonesResponse{}, ParsePutV1ComputezonesResponse, []int{400, 401, 403, 499, 500},
			func(rsp *PutV1ComputezonesResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			}, `{"id":"550e8400-e29b-41d4-a716-446655440000"}`,
			func(t *testing.T, result *ModelsUpdateComputeZoneResponse) {
				t.Helper()
				if result == nil || result.Id != "550e8400-e29b-41d4-a716-446655440000" {
					t.Errorf("GetJSON200() = %+v, want update ID 550e8400-e29b-41d4-a716-446655440000", result)
				}
			})
	})
	t.Run("auth_status", func(t *testing.T) {
		const jsonType = "application/json; charset=utf-8"
		makeResponse := func(status int, contentType string, body io.ReadCloser) *http.Response {
			return &http.Response{Status: fmt.Sprintf("%d supplied status", status), StatusCode: status,
				Header: http.Header{"Content-Type": []string{contentType}}, Body: body}
		}
		errorsFor := func(rsp *GetV1AuthStatusResponse) map[int]*ModelsErrorResponse {
			return map[int]*ModelsErrorResponse{401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499()}
		}
		zero := &GetV1AuthStatusResponse{}
		assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
		assertRound2ResponsePointer(t, "GetJSON200", zero.GetJSON200(), nil)
		assertRound2AlertTimelineError(t, 0, errorsFor(zero))

		t.Run("success", func(t *testing.T) {
			body := []byte(`{"authenticated":true}`)
			rsp, err := ParseGetV1AuthStatusResponse(makeResponse(200, jsonType, io.NopCloser(bytes.NewReader(body))))
			if err != nil || rsp == nil {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, "200 supplied status", 200, jsonType)
			assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
			result := rsp.GetJSON200()
			if result == nil || result.Authenticated == nil || !*result.Authenticated {
				t.Errorf("GetJSON200() = %+v, want authenticated true", result)
			}
		})
		body := []byte(`{"error":"denied","details":"specific detail"}`)
		for _, status := range []int{401, 403, 499} {
			t.Run(fmt.Sprint(status), func(t *testing.T) {
				rsp, err := ParseGetV1AuthStatusResponse(makeResponse(status, jsonType, io.NopCloser(bytes.NewReader(body))))
				if err != nil || rsp == nil {
					t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
				}
				assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, jsonType)
				assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
				assertRound2AlertTimelineError(t, status, errorsFor(rsp))
			})
		}
		for _, status := range []int{200, 401, 403, 499} {
			t.Run(fmt.Sprintf("malformed_%d", status), func(t *testing.T) {
				rsp, err := ParseGetV1AuthStatusResponse(makeResponse(status, jsonType, io.NopCloser(bytes.NewBufferString(`{"error":`))))
				if err == nil || rsp != nil {
					t.Errorf("parse = %v, %v; want nil response and decoding error", rsp, err)
				}
			})
		}
		t.Run("read_error", func(t *testing.T) {
			sentinel := errors.New("auth status read error")
			reader := &round2AlertTimelineErrorReader{err: sentinel}
			rsp, err := ParseGetV1AuthStatusResponse(makeResponse(403, jsonType, reader))
			if !errors.Is(err, sentinel) || rsp != nil || !reader.closed {
				t.Errorf("parse = %v, %v; closed = %t; want nil response, sentinel error and closed body", rsp, err, reader.closed)
			}
		})
		t.Run("text_plain", func(t *testing.T) {
			rsp, err := ParseGetV1AuthStatusResponse(makeResponse(403, "text/plain", io.NopCloser(bytes.NewReader(body))))
			if err != nil || rsp == nil {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, "403 supplied status", 403, "text/plain")
			assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
			assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
		})
	})
}

func TestRound2EventsOptionsAndInfoResponseContracts(t *testing.T) {
	t.Run("events", func(t *testing.T) {
		testRound2AlertsResponse(t, &GetV1EventsResponse{}, ParseGetV1EventsResponse, []int{400, 401, 403, 499, 500, 503},
			func(rsp *GetV1EventsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			}, `{"events":[{"eventId":"event-1","nodeUUID":"node-1","name":"overheat"}],"page":0,"total":42}`,
			func(t *testing.T, result *ModelsEventsResponse) {
				t.Helper()
				if result == nil || result.Events == nil || len(*result.Events) != 1 {
					t.Fatalf("GetJSON200() = %+v; want one event", result)
				}
				event := (*result.Events)[0]
				if event.EventId != "event-1" || event.NodeUUID != "node-1" || event.Name == nil || *event.Name != "overheat" || result.Page == nil || *result.Page != 0 || result.Total == nil || *result.Total != 42 {
					t.Errorf("GetJSON200() = %+v, event = %+v; want event-1, node-1, overheat, page 0 and total 42", result, event)
				}
			})
	})
	t.Run("info", func(t *testing.T) {
		testRound2AlertsResponse(t, &GetV1InfoResponse{}, ParseGetV1InfoResponse, []int{400, 499, 500},
			func(rsp *GetV1InfoResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			}, `{"components":{"apiBackend":{"version":"api-1.2","build":"api-build-1"}}}`,
			func(t *testing.T, result *ModelsInfoResponse) {
				t.Helper()
				if result == nil || result.Components == nil || result.Components.ApiBackend == nil {
					t.Fatalf("GetJSON200() = %+v; want API backend", result)
				}
				backend := result.Components.ApiBackend
				if backend.Version == nil || *backend.Version != "api-1.2" || backend.Build == nil || *backend.Build != "api-build-1" {
					t.Errorf("API backend = %+v; want version api-1.2 and build api-build-1", backend)
				}
			})
	})
	t.Run("options", func(t *testing.T) {
		const jsonType = "application/json"
		makeResponse := func(status int, contentType string, body io.ReadCloser) *http.Response {
			return &http.Response{Status: fmt.Sprintf("%d supplied status", status), StatusCode: status,
				Header: http.Header{"Content-Type": []string{contentType}}, Body: body}
		}
		errorsFor := func(rsp *GetV1EventsOptionsResponse) map[int]*ModelsErrorResponse {
			return map[int]*ModelsErrorResponse{401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
		}
		zero := &GetV1EventsOptionsResponse{}
		assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
		assertRound2ResponsePointer(t, "GetJSON200", zero.GetJSON200(), nil)
		assertRound2AlertTimelineError(t, 0, errorsFor(zero))

		body := []byte(`{"error":"denied","details":"specific detail"}`)
		for _, status := range []int{401, 403, 500, 503} {
			t.Run(fmt.Sprint(status), func(t *testing.T) {
				rsp, err := ParseGetV1EventsOptionsResponse(makeResponse(status, jsonType, io.NopCloser(bytes.NewReader(body))))
				if err != nil || rsp == nil {
					t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
				}
				assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, jsonType)
				assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
				assertRound2AlertTimelineError(t, status, errorsFor(rsp))
			})
		}
		t.Run("success", func(t *testing.T) {
			body := []byte(`{"eventNames":["XID","Others"],"dataSources":["inband","oob"]}`)
			rsp, err := ParseGetV1EventsOptionsResponse(makeResponse(200, jsonType, io.NopCloser(bytes.NewReader(body))))
			if err != nil || rsp == nil {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, "200 supplied status", 200, jsonType)
			assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
			result := rsp.GetJSON200()
			if result == nil || result.EventNames == nil || result.DataSources == nil {
				t.Fatalf("GetJSON200() = %+v; want event names and data sources", result)
			}
			if !reflect.DeepEqual(*result.EventNames, []string{"XID", "Others"}) || !reflect.DeepEqual(*result.DataSources, []string{"inband", "oob"}) {
				t.Errorf("GetJSON200() = %+v; want event names [XID Others] and data sources [inband oob]", result)
			}
		})
		t.Run("malformed", func(t *testing.T) {
			rsp, err := ParseGetV1EventsOptionsResponse(makeResponse(401, jsonType, io.NopCloser(bytes.NewBufferString(`{"error":`))))
			if err == nil || rsp != nil {
				t.Errorf("parse = %v, %v; want nil response and decoding error", rsp, err)
			}
		})
		t.Run("read_error", func(t *testing.T) {
			sentinel := errors.New("options read error")
			reader := &round2AlertTimelineErrorReader{err: sentinel}
			rsp, err := ParseGetV1EventsOptionsResponse(makeResponse(401, jsonType, reader))
			if !errors.Is(err, sentinel) || rsp != nil || !reader.closed {
				t.Errorf("parse = %v, %v; closed = %t; want nil response, sentinel error and closed body", rsp, err, reader.closed)
			}
		})
		t.Run("text_plain", func(t *testing.T) {
			rsp, err := ParseGetV1EventsOptionsResponse(makeResponse(401, "text/plain", io.NopCloser(bytes.NewReader(body))))
			if err != nil || rsp == nil {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, "401 supplied status", 401, "text/plain")
			assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
			assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
		})
	})
}

func testRound2MetricsResponse[T any, R interface {
	comparable
	round2ResponseMetadata
	GetJSON200() *T
}](t *testing.T, zero R, parse func(*http.Response) (R, error), statuses []int, supportedStatus int, errorsFor func(R) map[int]*ModelsErrorResponse, successBody string, checkSuccess func(*testing.T, *T)) {
	t.Helper()
	const jsonType = "application/json; charset=utf-8"
	makeResponse := func(status int, contentType string, body io.ReadCloser) *http.Response {
		return &http.Response{Status: fmt.Sprintf("%d supplied status", status), StatusCode: status,
			Header: http.Header{"Content-Type": []string{contentType}}, Body: body}
	}
	assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
	assertRound2ResponsePointer(t, "GetJSON200", zero.GetJSON200(), nil)
	assertRound2AlertTimelineError(t, 0, errorsFor(zero))

	body := []byte(`{"error":"denied","details":"specific detail"}`)
	for _, status := range statuses {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			rsp, err := parse(makeResponse(status, jsonType, io.NopCloser(bytes.NewReader(body))))
			var nilResponse R
			if err != nil || rsp == nilResponse {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, jsonType)
			assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
			assertRound2AlertTimelineError(t, status, errorsFor(rsp))
		})
	}
	t.Run("success", func(t *testing.T) {
		body := []byte(successBody)
		rsp, err := parse(makeResponse(200, jsonType, io.NopCloser(bytes.NewReader(body))))
		var nilResponse R
		if err != nil || rsp == nilResponse {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, body, "200 supplied status", 200, jsonType)
		assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
		checkSuccess(t, rsp.GetJSON200())
	})
	t.Run("malformed", func(t *testing.T) {
		rsp, err := parse(makeResponse(supportedStatus, jsonType, io.NopCloser(bytes.NewBufferString(`{"error":`))))
		var nilResponse R
		if err == nil || rsp != nilResponse {
			t.Errorf("parse = %v, %v; want nil response and decoding error", rsp, err)
		}
	})
	t.Run("read_error", func(t *testing.T) {
		sentinel := errors.New("metrics read error")
		reader := &round2AlertTimelineErrorReader{err: sentinel}
		rsp, err := parse(makeResponse(supportedStatus, jsonType, reader))
		var nilResponse R
		if !errors.Is(err, sentinel) || rsp != nilResponse || !reader.closed {
			t.Errorf("parse = %v, %v; closed = %t; want nil response, sentinel error and closed body", rsp, err, reader.closed)
		}
	})
	t.Run("text_plain", func(t *testing.T) {
		rsp, err := parse(makeResponse(supportedStatus, "text/plain", io.NopCloser(bytes.NewReader(body))))
		var nilResponse R
		if err != nil || rsp == nilResponse {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", supportedStatus), supportedStatus, "text/plain")
		assertRound2ResponsePointer(t, "GetJSON200", rsp.GetJSON200(), nil)
		assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
	})
}

func TestRound2MetricsResponseContracts(t *testing.T) {
	t.Run("GetV1MetricsResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1MetricsResponse{}, ParseGetV1MetricsResponse, []int{401, 403, 499, 500}, 401,
			func(rsp *GetV1MetricsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			}, `{"components":[{"name":"bmc"}]}`, func(t *testing.T, result *ModelsGetMetricsResponse) {
				t.Helper()
				if result == nil || result.Components == nil || len(*result.Components) != 1 {
					t.Fatalf("GetJSON200() = %+v; want one component", result)
				}
				component := (*result.Components)[0]
				if component.Name == nil || *component.Name != "bmc" {
					t.Errorf("component name = %v; want bmc", component.Name)
				}
			})
	})
	t.Run("GetV1MetricsQueryRangeResponse", func(t *testing.T) {
		testRound2AlertsResponse(t, &GetV1MetricsQueryRangeResponse{}, ParseGetV1MetricsQueryRangeResponse, []int{400, 401, 403, 404, 499, 500, 503},
			func(rsp *GetV1MetricsQueryRangeResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 404: rsp.GetJSON404(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			}, `{"result":[],"step":"5m"}`, func(t *testing.T, result *ModelsQueryMetricRangeResponse) {
				t.Helper()
				if result == nil || result.Result == nil || len(*result.Result) != 0 || result.Step == nil || *result.Step != "5m" {
					t.Errorf("GetJSON200() = %+v; want empty result and step 5m", result)
				}
			})
	})
	t.Run("GetV2MetricsResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV2MetricsResponse{}, ParseGetV2MetricsResponse, []int{499}, 499,
			func(rsp *GetV2MetricsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{499: rsp.GetJSON499()}
			}, `{"step":"5m","metrics":[{"name":"gpu_util","aggregation":"average","result":[]}]}`, func(t *testing.T, result *ModelsMetricsAggregationResponse) {
				t.Helper()
				if result == nil || result.Step == nil || *result.Step != "5m" || result.Metrics == nil || len(*result.Metrics) != 1 {
					t.Fatalf("GetJSON200() = %+v; want step 5m and one metric", result)
				}
				metric := (*result.Metrics)[0]
				if metric.Name == nil || *metric.Name != "gpu_util" || metric.Aggregation == nil || *metric.Aggregation != AggregationAverage || metric.Result == nil || len(*metric.Result) != 0 {
					t.Errorf("metric = %+v; want gpu_util, average and empty result", metric)
				}
			})
	})
}

func TestRound2NodeOptionsHistoryMembershipResponseContracts(t *testing.T) {
	t.Run("GetV1NodegroupsOptionsResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1NodegroupsOptionsResponse{}, ParseGetV1NodegroupsOptionsResponse, []int{401, 403, 499, 500}, 401,
			func(rsp *GetV1NodegroupsOptionsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			}, `{"filters":{"fields":[{"name":"computeZones","options":[{"id":"cz-1","value":"East"}]}]}}`, func(t *testing.T, result *ModelsOptions) {
				t.Helper()
				if result == nil || result.Filters == nil || result.Filters.Fields == nil || len(*result.Filters.Fields) != 1 {
					t.Fatalf("GetJSON200() = %+v; want one filter field", result)
				}
				field := (*result.Filters.Fields)[0]
				if field.Name == nil || *field.Name != "computeZones" || field.Options == nil || len(*field.Options) != 1 {
					t.Fatalf("filter field = %+v; want computeZones and one option", field)
				}
				option := (*field.Options)[0]
				if option["id"] != "cz-1" || option["value"] != "East" {
					t.Errorf("option = %+v; want id cz-1 and value East", option)
				}
			})
	})
	t.Run("GetV1NodesOptionsResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1NodesOptionsResponse{}, ParseGetV1NodesOptionsResponse, []int{400, 401, 403, 499, 500}, 400,
			func(rsp *GetV1NodesOptionsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			}, `{"sorting":{"fields":["name","health"],"orders":["asc","desc"],"defaults":{"field":"name","order":"asc"}}}`, func(t *testing.T, result *ModelsOptions) {
				t.Helper()
				if result == nil || result.Sorting == nil {
					t.Fatalf("GetJSON200() = %+v; want sorting options", result)
				}
				sorting := result.Sorting
				if sorting.Fields == nil || !reflect.DeepEqual(*sorting.Fields, []string{"name", "health"}) || sorting.Orders == nil || !reflect.DeepEqual(*sorting.Orders, []string{"asc", "desc"}) || sorting.Defaults == nil || sorting.Defaults.Field == nil || *sorting.Defaults.Field != "name" || sorting.Defaults.Order == nil || *sorting.Defaults.Order != "asc" {
					t.Errorf("sorting = %+v; want fields name, health; orders asc, desc; defaults name, asc", sorting)
				}
			})
	})
	t.Run("GetV1NodeGroupMembershipsResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1NodeGroupMembershipsResponse{}, ParseGetV1NodeGroupMembershipsResponse, []int{400, 401, 403, 499, 500, 503}, 400,
			func(rsp *GetV1NodeGroupMembershipsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			}, `{"memberships":[{"nodeGroupId":"group-1","nodeUUID":"node-1"}],"page":0,"pageSize":20,"total":21,"hasMore":true}`, func(t *testing.T, result *ModelsNodeGroupMembershipsResponse) {
				t.Helper()
				if result == nil || result.Memberships == nil || len(*result.Memberships) != 1 {
					t.Fatalf("GetJSON200() = %+v; want one membership", result)
				}
				membership := (*result.Memberships)[0]
				if membership.NodeGroupId == nil || *membership.NodeGroupId != "group-1" || membership.NodeUUID == nil || *membership.NodeUUID != "node-1" {
					t.Errorf("membership = %+v; want group-1 and node-1", membership)
				}
				if result.Page == nil || *result.Page != 0 || result.PageSize == nil || *result.PageSize != 20 || result.Total == nil || *result.Total != 21 || result.HasMore == nil || !*result.HasMore {
					t.Errorf("pagination = %+v; want page 0, pageSize 20, total 21, hasMore true", result)
				}
			})
	})
	t.Run("GetV1NodesNodeUuidHealthHistoryResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1NodesNodeUuidHealthHistoryResponse{}, ParseGetV1NodesNodeUuidHealthHistoryResponse, []int{400, 401, 404, 499, 500}, 400,
			func(rsp *GetV1NodesNodeUuidHealthHistoryResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 404: rsp.GetJSON404(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			}, `{"enrolledAt":"2026-04-01T00:00:00Z","healthSummary":{"healthyPercentage":99.5,"degradedDurationSeconds":3000}}`, func(t *testing.T, result *ModelsNodeHealthHistoryResponse) {
				t.Helper()
				if result == nil || result.EnrolledAt == nil || *result.EnrolledAt != "2026-04-01T00:00:00Z" || result.HealthSummary == nil || result.HealthSummary.HealthyPercentage == nil || *result.HealthSummary.HealthyPercentage != 99.5 || result.HealthSummary.DegradedDurationSeconds == nil || *result.HealthSummary.DegradedDurationSeconds != 3000 {
					t.Errorf("GetJSON200() = %+v; want enrolledAt and health summary values", result)
				}
			})
	})
}

func testRound2PolicyPutResponse[R interface {
	comparable
	round2ResponseMetadata
}](t *testing.T, zero R, parse func(*http.Response) (R, error), statuses []int, errorsFor func(R) map[int]*ModelsErrorResponse) {
	t.Helper()
	const jsonType = "application/json; charset=utf-8"
	makeResponse := func(status int, contentType string, body io.ReadCloser) *http.Response {
		return &http.Response{Status: fmt.Sprintf("%d supplied status", status), StatusCode: status,
			Header: http.Header{"Content-Type": []string{contentType}}, Body: body}
	}
	assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
	assertRound2AlertTimelineError(t, 0, errorsFor(zero))
	body := []byte(`{"error":"denied","details":"specific detail"}`)
	for _, status := range statuses {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			rsp, err := parse(makeResponse(status, jsonType, io.NopCloser(bytes.NewReader(body))))
			var nilResponse R
			if err != nil || rsp == nilResponse {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, jsonType)
			assertRound2AlertTimelineError(t, status, errorsFor(rsp))
		})
	}
	t.Run("no_content", func(t *testing.T) {
		rsp, err := parse(makeResponse(204, "", io.NopCloser(bytes.NewReader(nil))))
		var nilResponse R
		if err != nil || rsp == nilResponse {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, []byte{}, "204 supplied status", 204, "")
		assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
	})
	t.Run("text_plain", func(t *testing.T) {
		status := statuses[0]
		rsp, err := parse(makeResponse(status, "text/plain", io.NopCloser(bytes.NewReader(body))))
		var nilResponse R
		if err != nil || rsp == nilResponse {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, "text/plain")
		assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
	})
	t.Run("malformed", func(t *testing.T) {
		rsp, err := parse(makeResponse(statuses[0], jsonType, io.NopCloser(bytes.NewBufferString(`{"error":`))))
		var nilResponse R
		if err == nil || rsp != nilResponse {
			t.Errorf("parse = %v, %v; want nil response and decoding error", rsp, err)
		}
	})
	t.Run("read_error", func(t *testing.T) {
		sentinel := errors.New("policy read error")
		reader := &round2AlertTimelineErrorReader{err: sentinel}
		rsp, err := parse(makeResponse(statuses[0], jsonType, reader))
		var nilResponse R
		if err != sentinel || rsp != nilResponse || !reader.closed {
			t.Errorf("parse = %v, %v; closed = %t; want nil response, original error and closed body", rsp, err, reader.closed)
		}
	})
}

func TestRound2PolicyResponseContracts(t *testing.T) {
	t.Run("GetV1NodeRemovalPolicy", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1NodeRemovalPolicyResponse{}, ParseGetV1NodeRemovalPolicyResponse, []int{401, 403, 500, 503}, 401,
			func(rsp *GetV1NodeRemovalPolicyResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			}, `{"enabled":true,"offlineThresholdDays":7,"lastCheckedAt":"2024-01-15T10:30:00Z","updatedBy":"user@example.com"}`, func(t *testing.T, result *ModelsNodeRemovalPolicy) {
				t.Helper()
				if result == nil || result.Enabled == nil || !*result.Enabled || result.OfflineThresholdDays == nil || *result.OfflineThresholdDays != 7 || result.LastCheckedAt == nil || *result.LastCheckedAt != "2024-01-15T10:30:00Z" || result.UpdatedBy == nil || *result.UpdatedBy != "user@example.com" {
					t.Errorf("GetJSON200() = %+v; want exact node-removal policy fields", result)
				}
			})
	})
	t.Run("GetV1RetentionPolicy", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1RetentionPolicyResponse{}, ParseGetV1RetentionPolicyResponse, []int{401, 403, 404, 499, 500, 503}, 401,
			func(rsp *GetV1RetentionPolicyResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 404: rsp.GetJSON404(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			}, `{"retentionDays":90,"createdAt":"2026-01-01T00:00:00Z","updatedBy":"alice"}`, func(t *testing.T, result *ModelsGetRetentionPolicyResponse) {
				t.Helper()
				if result == nil || result.RetentionDays == nil || *result.RetentionDays != 90 || result.CreatedAt == nil || *result.CreatedAt != "2026-01-01T00:00:00Z" || result.UpdatedBy == nil || *result.UpdatedBy != "alice" {
					t.Errorf("GetJSON200() = %+v; want exact retention policy fields", result)
				}
			})
	})
	t.Run("PutV1NodeRemovalPolicy", func(t *testing.T) {
		testRound2PolicyPutResponse(t, &PutV1NodeRemovalPolicyResponse{}, ParsePutV1NodeRemovalPolicyResponse, []int{400, 401, 403, 500, 503},
			func(rsp *PutV1NodeRemovalPolicyResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			})
	})
	t.Run("PutV1RetentionPolicy", func(t *testing.T) {
		testRound2PolicyPutResponse(t, &PutV1RetentionPolicyResponse{}, ParsePutV1RetentionPolicyResponse, []int{400, 401, 403, 499, 500, 503},
			func(rsp *PutV1RetentionPolicyResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			})
	})
}

func TestRound2RankingsAndCommandsResponseContracts(t *testing.T) {
	t.Run("GetV1PredictedHealthRankingsResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1PredictedHealthRankingsResponse{}, ParseGetV1PredictedHealthRankingsResponse, []int{400, 401, 403, 499, 500, 501, 503}, 400,
			func(rsp *GetV1PredictedHealthRankingsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 501: rsp.GetJSON501(), 503: rsp.GetJSON503()}
			}, `{"rankUnit":"rack","complete":true,"items":[],"total":2,"rankedCount":1,"unrankedCount":1,"unmappedNodeCount":0,"page":1,"pageSize":2,"hasMore":false}`, func(t *testing.T, result *ModelsPredictedHealthRankingsResponse) {
				t.Helper()
				if result == nil || result.RankUnit == nil || *result.RankUnit != ModelsPredictedHealthRankingsResponseRankUnitRack || result.Complete == nil || !*result.Complete || result.Items == nil || len(*result.Items) != 0 || result.Total == nil || *result.Total != 2 || result.RankedCount == nil || *result.RankedCount != 1 || result.UnrankedCount == nil || *result.UnrankedCount != 1 || result.UnmappedNodeCount == nil || *result.UnmappedNodeCount != 0 || result.Page == nil || *result.Page != 1 || result.PageSize == nil || *result.PageSize != 2 || result.HasMore == nil || *result.HasMore {
					t.Errorf("GetJSON200() = %+v; want exact ranking fields and empty items", result)
				}
			})
	})
	t.Run("GetV2CommandsResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV2CommandsResponse{}, ParseGetV2CommandsResponse, []int{400, 499, 500}, 400,
			func(rsp *GetV2CommandsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			}, `{"commands":[{"name":"inspect","link":"/commands/inspect","template":"inspect {node}","placeholders":["node"]}]}`, func(t *testing.T, result *ModelsCommandsResponse) {
				t.Helper()
				if result == nil || result.Commands == nil || len(*result.Commands) != 1 {
					t.Fatalf("GetJSON200() = %+v; want one command", result)
				}
				command := (*result.Commands)[0]
				if command.Name == nil || *command.Name != "inspect" || command.Link == nil || *command.Link != "/commands/inspect" || command.Template == nil || *command.Template != "inspect {node}" || command.Placeholders == nil || len(*command.Placeholders) != 1 || (*command.Placeholders)[0] != "node" {
					t.Errorf("command = %+v; want exact inspect command fields", command)
				}
			})
	})
}

func TestRound2TagResponseContracts(t *testing.T) {
	t.Run("GetV1TagsResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1TagsResponse{}, ParseGetV1TagsResponse, []int{400, 401, 403, 499, 500}, 400,
			func(rsp *GetV1TagsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			}, `{"tags":["rack-blue","production"]}`, func(t *testing.T, result *ModelsListTagsResponse) {
				t.Helper()
				if result == nil || result.Tags == nil || len(*result.Tags) != 2 || (*result.Tags)[0] != "rack-blue" || (*result.Tags)[1] != "production" {
					t.Errorf("GetJSON200() = %+v; want tags [rack-blue production]", result)
				}
			})
	})
	t.Run("PutV1NodesNodeUuidTagsResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &PutV1NodesNodeUuidTagsResponse{}, ParsePutV1NodesNodeUuidTagsResponse, []int{400, 404, 499, 500}, 400,
			func(rsp *PutV1NodesNodeUuidTagsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 404: rsp.GetJSON404(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			}, `{"nodeUUID":"node-1","tags":["rack-blue","production"]}`, func(t *testing.T, result *ModelsSetNodeTagsResponse) {
				t.Helper()
				if result == nil || result.NodeUUID == nil || *result.NodeUUID != "node-1" || result.Tags == nil || len(*result.Tags) != 2 || (*result.Tags)[0] != "rack-blue" || (*result.Tags)[1] != "production" {
					t.Errorf("GetJSON200() = %+v; want node-1 and tags [rack-blue production]", result)
				}
			})
	})
	t.Run("PatchV1TagsResponse", func(t *testing.T) {
		testRound2PolicyPutResponse(t, &PatchV1TagsResponse{}, ParsePatchV1TagsResponse, []int{400, 401, 403, 499, 500},
			func(rsp *PatchV1TagsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500()}
			})
	})
}

func TestRound2NotificationResponseContracts(t *testing.T) {
	t.Run("GetV1NotificationDeliveryLogResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1NotificationDeliveryLogResponse{}, ParseGetV1NotificationDeliveryLogResponse, []int{400, 401, 403, 499, 500, 503}, 400,
			func(rsp *GetV1NotificationDeliveryLogResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			}, `{"deliveryLog":[{"alertUUID":"alert-123","component":"GPU"}],"page":0,"pageSize":20,"total":1,"hasMore":false}`, func(t *testing.T, result *ModelsDeliveryLogResponse) {
				t.Helper()
				if result == nil || result.DeliveryLog == nil || len(*result.DeliveryLog) != 1 || (*result.DeliveryLog)[0].AlertUUID == nil || *(*result.DeliveryLog)[0].AlertUUID != "alert-123" || (*result.DeliveryLog)[0].Component == nil || *(*result.DeliveryLog)[0].Component != "GPU" || result.Page == nil || *result.Page != 0 || result.PageSize == nil || *result.PageSize != 20 || result.Total == nil || *result.Total != 1 || result.HasMore == nil || *result.HasMore {
					t.Errorf("GetJSON200() = %+v; want delivery log and pagination values", result)
				}
			})
	})
	t.Run("GetV1NotificationPreferencesResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1NotificationPreferencesResponse{}, ParseGetV1NotificationPreferencesResponse, []int{401, 403, 499, 500, 503}, 401,
			func(rsp *GetV1NotificationPreferencesResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			}, `{"preferences":[{"id":"preference-1","components":["GPU"]}],"total":1,"componentPreferenceCounts":{"GPU":1}}`, func(t *testing.T, result *ModelsListNotificationPreferencesResponse) {
				t.Helper()
				if result == nil || result.Preferences == nil || len(*result.Preferences) != 1 || (*result.Preferences)[0].Id == nil || *(*result.Preferences)[0].Id != "preference-1" || (*result.Preferences)[0].Components == nil || !reflect.DeepEqual(*(*result.Preferences)[0].Components, []string{"GPU"}) || result.Total == nil || *result.Total != 1 || result.ComponentPreferenceCounts == nil || !reflect.DeepEqual(*result.ComponentPreferenceCounts, map[string]int{"GPU": 1}) {
					t.Errorf("GetJSON200() = %+v; want preference, total and component counts", result)
				}
			})
	})
	t.Run("GetV1NotificationPreferenceByIDResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1NotificationPreferenceByIDResponse{}, ParseGetV1NotificationPreferenceByIDResponse, []int{401, 403, 404, 499, 500, 503}, 401,
			func(rsp *GetV1NotificationPreferenceByIDResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 404: rsp.GetJSON404(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			}, `{"id":"preference-1","components":["GPU"],"active":false}`, func(t *testing.T, result *ModelsNotificationPreferenceDetail) {
				t.Helper()
				if result == nil || result.Id == nil || *result.Id != "preference-1" || result.Components == nil || !reflect.DeepEqual(*result.Components, []string{"GPU"}) || result.Active == nil || *result.Active {
					t.Errorf("GetJSON200() = %+v; want preference-1, [GPU], inactive", result)
				}
			})
	})
	t.Run("DeleteV1NotificationPreferenceByIDResponse", func(t *testing.T) {
		testRound2PolicyPutResponse(t, &DeleteV1NotificationPreferenceByIDResponse{}, ParseDeleteV1NotificationPreferenceByIDResponse, []int{400, 401, 403, 404, 499, 500, 503},
			func(rsp *DeleteV1NotificationPreferenceByIDResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 404: rsp.GetJSON404(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			})
	})
	t.Run("PutV1NotificationPreferenceByIDResponse", func(t *testing.T) {
		testRound2PolicyPutResponse(t, &PutV1NotificationPreferenceByIDResponse{}, ParsePutV1NotificationPreferenceByIDResponse, []int{400, 401, 403, 404, 499, 500, 503},
			func(rsp *PutV1NotificationPreferenceByIDResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 404: rsp.GetJSON404(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
			})
	})
	t.Run("PostV1NotificationPreferencesResponse", func(t *testing.T) {
		const jsonType = "application/json; charset=utf-8"
		makeResponse := func(status int, contentType string, body io.ReadCloser) *http.Response {
			return &http.Response{Status: fmt.Sprintf("%d supplied status", status), StatusCode: status,
				Header: http.Header{"Content-Type": []string{contentType}}, Body: body}
		}
		errorsFor := func(rsp *PostV1NotificationPreferencesResponse) map[int]*ModelsErrorResponse {
			return map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
		}
		zero := &PostV1NotificationPreferencesResponse{}
		assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
		assertRound2ResponsePointer(t, "GetJSON201", zero.GetJSON201(), nil)
		assertRound2AlertTimelineError(t, 0, errorsFor(zero))
		t.Run("success", func(t *testing.T) {
			body := []byte(`{"id":"preference-1"}`)
			rsp, err := ParsePostV1NotificationPreferencesResponse(makeResponse(201, jsonType, io.NopCloser(bytes.NewReader(body))))
			if err != nil || rsp == nil {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, "201 supplied status", 201, jsonType)
			assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
			if result := rsp.GetJSON201(); result == nil || result.Id != "preference-1" {
				t.Errorf("GetJSON201() = %+v; want id preference-1", result)
			}
		})
		body := []byte(`{"error":"denied","details":"specific detail"}`)
		for _, status := range []int{400, 401, 403, 499, 500, 503} {
			t.Run(fmt.Sprint(status), func(t *testing.T) {
				rsp, err := ParsePostV1NotificationPreferencesResponse(makeResponse(status, jsonType, io.NopCloser(bytes.NewReader(body))))
				if err != nil || rsp == nil {
					t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
				}
				assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, jsonType)
				assertRound2ResponsePointer(t, "GetJSON201", rsp.GetJSON201(), nil)
				assertRound2AlertTimelineError(t, status, errorsFor(rsp))
			})
		}
		t.Run("text_plain", func(t *testing.T) {
			rsp, err := ParsePostV1NotificationPreferencesResponse(makeResponse(400, "text/plain", io.NopCloser(bytes.NewReader(body))))
			if err != nil || rsp == nil {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, "400 supplied status", 400, "text/plain")
			assertRound2ResponsePointer(t, "GetJSON201", rsp.GetJSON201(), nil)
			assertRound2AlertTimelineError(t, 0, errorsFor(rsp))
		})
		t.Run("malformed", func(t *testing.T) {
			rsp, err := ParsePostV1NotificationPreferencesResponse(makeResponse(201, jsonType, io.NopCloser(bytes.NewBufferString(`{"id":`))))
			if err == nil || rsp != nil {
				t.Errorf("parse = %v, %v; want nil response and decoding error", rsp, err)
			}
		})
		t.Run("read_error", func(t *testing.T) {
			sentinel := errors.New("notification read error")
			reader := &round2AlertTimelineErrorReader{err: sentinel}
			rsp, err := ParsePostV1NotificationPreferencesResponse(makeResponse(201, jsonType, reader))
			if !errors.Is(err, sentinel) || rsp != nil || !reader.closed {
				t.Errorf("parse = %v, %v; closed = %t; want nil response, sentinel error and closed body", rsp, err, reader.closed)
			}
		})
	})
}

func TestRound2StatsSecurityOptionsAndNodeDeletionResponseContracts(t *testing.T) {
	statuses := []int{400, 401, 403, 404, 499, 500, 503}
	t.Run("stats", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1StatsResponse{}, ParseGetV1StatsResponse, statuses, 400,
			func(r *GetV1StatsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: r.GetJSON400(), 401: r.GetJSON401(), 403: r.GetJSON403(), 404: r.GetJSON404(), 499: r.GetJSON499(), 500: r.GetJSON500(), 503: r.GetJSON503()}
			}, `{"timestamps":[1719489600,1719576000,1719662400],"series":[{"name":"Online","values":[7,null,0]}]}`, func(t *testing.T, result *ModelsStatsResponse) {
				t.Helper()
				if result == nil || result.Timestamps == nil || !reflect.DeepEqual(*result.Timestamps, []int{1719489600, 1719576000, 1719662400}) || result.Series == nil || len(*result.Series) != 1 {
					t.Fatalf("stats = %+v; want exact timestamps and one series", result)
				}
				series := (*result.Series)[0]
				seven, zero := 7, 0
				if series.Name == nil || *series.Name != "Online" || series.Values == nil || !reflect.DeepEqual(*series.Values, []*int{&seven, nil, &zero}) {
					t.Errorf("series = %+v; want Online with values [7, null, 0]", series)
				}
			})
	})
	t.Run("security_options", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1ReportsSecurityVulnerabilityOptionsResponse{}, ParseGetV1ReportsSecurityVulnerabilityOptionsResponse, []int{400, 401, 403, 422, 499, 500, 503}, 400,
			func(r *GetV1ReportsSecurityVulnerabilityOptionsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: r.GetJSON400(), 401: r.GetJSON401(), 403: r.GetJSON403(), 422: r.GetJSON422(), 499: r.GetJSON499(), 500: r.GetJSON500(), 503: r.GetJSON503()}
			}, `{"filters":{"computeZoneIds":[{"id":"zone-1","name":"East"}],"nodeGroupIds":[{"id":"group-1","name":"Training"}],"componentTypes":[{"id":"gpuDriver","name":"GPU Driver"},{"id":"vbios","name":"VBIOS"}],"gpuTypes":["H100","A100"],"severities":["CRITICAL","HIGH"]},"sorting":{"fields":["vulnerability","affectedNodes"],"orders":["asc","desc"],"defaults":{"field":"affectedNodes","order":"desc"}}}`, func(t *testing.T, result *ModelsSecurityVulnerabilityOptionsResponse) {
				t.Helper()
				if result == nil {
					t.Fatal("GetJSON200() = nil")
				}
				want := ModelsSecurityVulnerabilityFilterOptions{
					ComputeZoneIds: []ModelsIDNameOption{{Id: "zone-1", Name: "East"}},
					NodeGroupIds:   []ModelsIDNameOption{{Id: "group-1", Name: "Training"}},
					ComponentTypes: []ModelsIDNameOption{{Id: "gpuDriver", Name: "GPU Driver"}, {Id: "vbios", Name: "VBIOS"}},
					GpuTypes:       []string{"H100", "A100"}, Severities: []ModelsSecurityVulnerabilitySeverity{SecurityVulnerabilitySeverityCritical, SecurityVulnerabilitySeverityHigh},
				}
				if !reflect.DeepEqual(result.Filters, want) {
					t.Errorf("filters = %+v; want %+v", result.Filters, want)
				}
				s := result.Sorting
				if s.Fields == nil || !reflect.DeepEqual(*s.Fields, []string{"vulnerability", "affectedNodes"}) || s.Orders == nil || !reflect.DeepEqual(*s.Orders, []string{"asc", "desc"}) || s.Defaults == nil || s.Defaults.Field == nil || *s.Defaults.Field != "affectedNodes" || s.Defaults.Order == nil || *s.Defaults.Order != "desc" {
					t.Errorf("sorting = %+v; want exact fields, orders and defaults", s)
				}
			})
	})
	checkDeletion := func(t *testing.T, result *map[string]interface{}) {
		t.Helper()
		want := map[string]interface{}{"message": "accepted", "count": float64(2)}
		if result == nil || !reflect.DeepEqual(*result, want) {
			t.Errorf("GetJSON200() = %v; want %v", result, want)
		}
	}
	const deletionBody = `{"message":"accepted","count":2}`
	t.Run("delete_nodes", func(t *testing.T) {
		testRound2MetricsResponse(t, &DeleteV1NodesResponse{}, ParseDeleteV1NodesResponse, statuses, 400,
			func(r *DeleteV1NodesResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: r.GetJSON400(), 401: r.GetJSON401(), 403: r.GetJSON403(), 404: r.GetJSON404(), 499: r.GetJSON499(), 500: r.GetJSON500(), 503: r.GetJSON503()}
			}, deletionBody, checkDeletion)
	})
	t.Run("delete_node", func(t *testing.T) {
		testRound2MetricsResponse(t, &DeleteV1NodesNodeUuidResponse{}, ParseDeleteV1NodesNodeUuidResponse, statuses, 400,
			func(r *DeleteV1NodesNodeUuidResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: r.GetJSON400(), 401: r.GetJSON401(), 403: r.GetJSON403(), 404: r.GetJSON404(), 499: r.GetJSON499(), 500: r.GetJSON500(), 503: r.GetJSON503()}
			}, deletionBody, checkDeletion)
	})
}

func TestRound2XIDBurstsAndNodeRemovalHistoryResponseContracts(t *testing.T) {
	t.Run("GetV1XIDBurstsResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1XIDBurstsResponse{}, ParseGetV1XIDBurstsResponse, []int{400, 401, 403, 499, 500}, 400,
			func(r *GetV1XIDBurstsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: r.GetJSON400(), 401: r.GetJSON401(), 403: r.GetJSON403(), 499: r.GetJSON499(), 500: r.GetJSON500()}
			}, `{"items":[{"burstId":"burst-1","nodeUuid":"node-1"}],"page":2,"pageSize":25,"total":42}`, func(t *testing.T, result *ModelsXIDBurstsListResponse) {
				t.Helper()
				if result == nil || result.Items == nil || len(*result.Items) != 1 {
					t.Fatalf("GetJSON200() = %+v; want one item", result)
				}
				item := (*result.Items)[0]
				if item.BurstId != "burst-1" || item.NodeUuid != "node-1" {
					t.Errorf("item = %+v; want burst-1 and node-1", item)
				}
				if result.Page == nil || *result.Page != 2 || result.PageSize == nil || *result.PageSize != 25 || result.Total == nil || *result.Total != 42 {
					t.Errorf("pagination = %+v; want page 2, pageSize 25, total 42", result)
				}
			})
	})
	t.Run("GetV1XIDBurstOptionsResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1XIDBurstOptionsResponse{}, ParseGetV1XIDBurstOptionsResponse, []int{400, 401, 403, 499, 500}, 400,
			func(r *GetV1XIDBurstOptionsResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: r.GetJSON400(), 401: r.GetJSON401(), 403: r.GetJSON403(), 499: r.GetJSON499(), 500: r.GetJSON500()}
			}, `{"dataSource":"all","gpuTypes":["H100","A100"],"xidNumbers":[31,79],"jobDisruption":[true,false]}`, func(t *testing.T, result *ModelsXIDBurstFilterOptionsResponse) {
				t.Helper()
				if result == nil || result.DataSource == nil || *result.DataSource != "all" || result.GpuTypes == nil || !reflect.DeepEqual(*result.GpuTypes, []string{"H100", "A100"}) || result.XidNumbers == nil || !reflect.DeepEqual(*result.XidNumbers, []int{31, 79}) || result.JobDisruption == nil || !reflect.DeepEqual(*result.JobDisruption, []bool{true, false}) {
					t.Errorf("GetJSON200() = %+v; want exact source, GPU types, XID numbers and disruption options", result)
				}
			})
	})
	t.Run("GetV1XIDBurstDetailResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1XIDBurstDetailResponse{}, ParseGetV1XIDBurstDetailResponse, []int{400, 401, 403, 404, 499, 500}, 400,
			func(r *GetV1XIDBurstDetailResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: r.GetJSON400(), 401: r.GetJSON401(), 403: r.GetJSON403(), 404: r.GetJSON404(), 499: r.GetJSON499(), 500: r.GetJSON500()}
			}, `{"burstId":"burst-1","nodeUuid":"node-1","dataSource":"oob","deviceIds":{"0000:01:00.0":[31,79]},"jobDisruption":true}`, func(t *testing.T, result *ModelsXIDBurstDetail) {
				t.Helper()
				if result == nil || result.BurstId != "burst-1" || result.NodeUuid != "node-1" || result.DataSource == nil || *result.DataSource != "oob" || result.DeviceIds == nil || !reflect.DeepEqual(*result.DeviceIds, map[string][]int{"0000:01:00.0": {31, 79}}) || result.JobDisruption == nil || !*result.JobDisruption {
					t.Errorf("GetJSON200() = %+v; want exact IDs, source, device map and disruption", result)
				}
			})
	})
	t.Run("GetV1NodesHistoryResponse", func(t *testing.T) {
		testRound2MetricsResponse(t, &GetV1NodesHistoryResponse{}, ParseGetV1NodesHistoryResponse, []int{400, 401, 403, 500, 503}, 400,
			func(r *GetV1NodesHistoryResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: r.GetJSON400(), 401: r.GetJSON401(), 403: r.GetJSON403(), 500: r.GetJSON500(), 503: r.GetJSON503()}
			}, `{"history":[{"nodeUuid":"node-1","deletedBy":"alice","actorType":"user","hostname":"node-a"}],"page":0,"pageSize":2,"total":3,"hasMore":true}`, func(t *testing.T, result *ModelsNodeHistoryResponse) {
				t.Helper()
				if result == nil || result.History == nil || len(*result.History) != 1 {
					t.Fatalf("GetJSON200() = %+v; want one history entry", result)
				}
				entry := (*result.History)[0]
				if entry.NodeUuid == nil || *entry.NodeUuid != "node-1" || entry.DeletedBy == nil || *entry.DeletedBy != "alice" || entry.ActorType == nil || *entry.ActorType != "user" || entry.Hostname == nil || *entry.Hostname != "node-a" {
					t.Errorf("history entry = %+v; want node-1, alice, user and node-a", entry)
				}
				if result.Page == nil || *result.Page != 0 || result.PageSize == nil || *result.PageSize != 2 || result.Total == nil || *result.Total != 3 || result.HasMore == nil || !*result.HasMore {
					t.Errorf("pagination = %+v; want page 0, pageSize 2, total 3, hasMore true", result)
				}
			})
	})
}

func TestRound2XIDBurstFilterRequestContracts(t *testing.T) {
	const server = "https://example.test/"
	start, end := "2026-01-01T00:00:00Z", "2026-01-02T00:00:00Z"
	node := "11111111-1111-4111-8111-111111111111"
	groups := []string{"22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"}
	zones := []string{"44444444-4444-4444-8444-444444444444"}
	excludedGroups := []string{"55555555-5555-4555-8555-555555555555", "66666666-6666-4666-8666-666666666666"}
	excludedZones := []string{"77777777-7777-4777-8777-777777777777", "88888888-8888-4888-8888-888888888888"}
	disruption := true
	hostname, category, subcategory := "rack & gpu", "hardware", "memory"
	tenantAction, tenantInvestigation := "CONTACT", "CHECK"
	dcAction, dcInvestigation := "REPLACE", "INSPECT"
	categories := []string{"hardware", "thermal"}
	subcategories := []string{"memory", "power"}
	tenantActions := []string{"CONTACT_SUPPORT", "REBOOT"}
	tenantInvestigations := []string{"CHECK_LOGS", "INSPECT"}
	dcActions := []string{"REPLACE", "REPAIR"}
	dcInvestigations := []string{"INSPECT", "DIAGNOSE"}

	check := func(t *testing.T, req *http.Request, path string, want url.Values) {
		t.Helper()
		if req.Method != http.MethodGet || req.URL.Path != path {
			t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, path)
		}
		got, err := url.ParseQuery(req.URL.RawQuery)
		if err != nil {
			t.Fatalf("parse query: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("query = %#v, want %#v", got, want)
		}
	}

	t.Run("list_inclusions", func(t *testing.T) {
		req, err := NewGetV1XIDBurstsRequest(server, &GetV1XIDBurstsParams{
			StartTime: &start, EndTime: &end, NodeUUID: &node, NodeGroupIds: &groups, ComputeZoneIds: &zones,
			JobDisruptionDueToPlatformIssue: &disruption, HostnameSearch: &hostname,
			CategorySearch: &category, SubcategorySearch: &subcategory, TenantActionSearch: &tenantAction,
			TenantInvestigationSearch: &tenantInvestigation, DcAdminActionSearch: &dcAction, DcAdminInvestigationSearch: &dcInvestigation,
			Categories: &categories, Subcategories: &subcategories, TenantActions: &tenantActions,
			TenantInvestigations: &tenantInvestigations, DcAdminActions: &dcActions, DcAdminInvestigations: &dcInvestigations,
		})
		if err != nil {
			t.Fatalf("construct list request: %v", err)
		}
		check(t, req, "/v1/xid/bursts", url.Values{
			"startTime": {"2026-01-01T00:00:00Z"}, "endTime": {"2026-01-02T00:00:00Z"},
			"nodeUUID":                        {"11111111-1111-4111-8111-111111111111"},
			"nodeGroupIds":                    {"22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"},
			"computeZoneIds":                  {"44444444-4444-4444-8444-444444444444"},
			"jobDisruptionDueToPlatformIssue": {"true"}, "hostnameSearch": {"rack & gpu"},
			"categorySearch": {"hardware"}, "subcategorySearch": {"memory"},
			"tenantActionSearch": {"CONTACT"}, "tenantInvestigationSearch": {"CHECK"},
			"dcAdminActionSearch": {"REPLACE"}, "dcAdminInvestigationSearch": {"INSPECT"},
			"categories": {"hardware", "thermal"}, "subcategories": {"memory", "power"},
			"tenantActions": {"CONTACT_SUPPORT", "REBOOT"}, "tenantInvestigations": {"CHECK_LOGS", "INSPECT"},
			"dcAdminActions": {"REPLACE", "REPAIR"}, "dcAdminInvestigations": {"INSPECT", "DIAGNOSE"},
		})
	})

	excludedQuery := url.Values{
		"startTime": {"2026-01-01T00:00:00Z"}, "endTime": {"2026-01-02T00:00:00Z"},
		"excludeNodeGroupIds":   {"55555555-5555-4555-8555-555555555555", "66666666-6666-4666-8666-666666666666"},
		"excludeComputeZoneIds": {"77777777-7777-4777-8777-777777777777", "88888888-8888-4888-8888-888888888888"},
	}
	t.Run("list_exclusions", func(t *testing.T) {
		req, err := NewGetV1XIDBurstsRequest(server, &GetV1XIDBurstsParams{
			StartTime: &start, EndTime: &end, ExcludeNodeGroupIds: &excludedGroups, ExcludeComputeZoneIds: &excludedZones,
		})
		if err != nil {
			t.Fatalf("construct list request: %v", err)
		}
		check(t, req, "/v1/xid/bursts", excludedQuery)
	})
	t.Run("options_inclusions", func(t *testing.T) {
		req, err := NewGetV1XIDBurstOptionsRequest(server, &GetV1XIDBurstOptionsParams{
			StartTime: &start, EndTime: &end, NodeGroupIds: &groups, ComputeZoneIds: &zones,
		})
		if err != nil {
			t.Fatalf("construct options request: %v", err)
		}
		check(t, req, "/v1/xid/bursts/options", url.Values{
			"startTime": {"2026-01-01T00:00:00Z"}, "endTime": {"2026-01-02T00:00:00Z"},
			"nodeGroupIds":   {"22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"},
			"computeZoneIds": {"44444444-4444-4444-8444-444444444444"},
		})
	})
	t.Run("options_exclusions", func(t *testing.T) {
		req, err := NewGetV1XIDBurstOptionsRequest(server, &GetV1XIDBurstOptionsParams{
			StartTime: &start, EndTime: &end, ExcludeNodeGroupIds: &excludedGroups, ExcludeComputeZoneIds: &excludedZones,
		})
		if err != nil {
			t.Fatalf("construct options request: %v", err)
		}
		check(t, req, "/v1/xid/bursts/options", excludedQuery)
	})
}

func TestRound2RawPolicyAndNodeRequests(t *testing.T) {
	const contentType = "application/json; charset=utf-8"
	const node = "11111111-1111-4111-8111-111111111111"
	dataSource := DeleteV1NodesParamsDataSource("oob")
	agentType := DeleteV1NodesParamsAgentType("oob")
	params := &DeleteV1NodesParams{DataSource: &dataSource, AgentType: &agentType}
	tests := []struct {
		name, method, path, body, responseBody string
		query                                  url.Values
		status                                 int
		call                                   func(*ClientWithResponses, RequestEditorFn) (interface{}, error)
		construct                              func() (*http.Request, error)
	}{
		{
			name: "node_removal_policy", method: http.MethodPut, path: "/v1/node_removal_policy",
			body: `{"enabled":true,"offlineThresholdDays":14}`, status: http.StatusNoContent,
			call: func(c *ClientWithResponses, editor RequestEditorFn) (interface{}, error) {
				return c.PutV1NodeRemovalPolicyWithBodyWithResponse(context.Background(), contentType, bytes.NewBufferString(`{"enabled":true,"offlineThresholdDays":14}`), editor)
			},
			construct: func() (*http.Request, error) {
				return NewPutV1NodeRemovalPolicyRequestWithBody("http://[::1", contentType, bytes.NewReader(nil))
			},
		},
		{
			name: "retention_policy", method: http.MethodPut, path: "/v1/retention_policy",
			body: `{"retentionDays":365}`, status: http.StatusNoContent,
			call: func(c *ClientWithResponses, editor RequestEditorFn) (interface{}, error) {
				return c.PutV1RetentionPolicyWithBodyWithResponse(context.Background(), contentType, bytes.NewBufferString(`{"retentionDays":365}`), editor)
			},
			construct: func() (*http.Request, error) {
				return NewPutV1RetentionPolicyRequestWithBody("http://[::1", contentType, bytes.NewReader(nil))
			},
		},
		{
			name: "delete_nodes", method: http.MethodDelete, path: "/v1/nodes",
			body: `{"nodeUUIDs":["11111111-1111-4111-8111-111111111111"]}`, responseBody: `{"message":"accepted","count":1}`,
			query: url.Values{"dataSource": {"oob"}, "agentType": {"oob"}}, status: http.StatusOK,
			call: func(c *ClientWithResponses, editor RequestEditorFn) (interface{}, error) {
				return c.DeleteV1NodesWithBodyWithResponse(context.Background(), params, contentType, bytes.NewBufferString(`{"nodeUUIDs":["11111111-1111-4111-8111-111111111111"]}`), editor)
			},
			construct: func() (*http.Request, error) {
				return NewDeleteV1NodesRequestWithBody("http://[::1", params, contentType, bytes.NewReader(nil))
			},
		},
		{
			name: "node_tags", method: http.MethodPut, path: "/v1/nodes/" + node + "/tags",
			body: `{"tags":["gpu-health","node123"]}`, responseBody: `{"nodeUUID":"11111111-1111-4111-8111-111111111111","tags":["gpu-health","node123"]}`, status: http.StatusOK,
			call: func(c *ClientWithResponses, editor RequestEditorFn) (interface{}, error) {
				return c.PutV1NodesNodeUuidTagsWithBodyWithResponse(context.Background(), node, contentType, bytes.NewBufferString(`{"tags":["gpu-health","node123"]}`), editor)
			},
			construct: func() (*http.Request, error) {
				return NewPutV1NodesNodeUuidTagsRequestWithBody("http://[::1", node, contentType, bytes.NewReader(nil))
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			doer := authStatusTestDoer(func(req *http.Request) (*http.Response, error) {
				calls++
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("read request body: %v", err)
				}
				if req.Method != tc.method || req.URL.Path != tc.path || !bytes.Equal(body, []byte(tc.body)) || req.Header.Get("Content-Type") != contentType {
					t.Errorf("request = %s %s, body %q, content type %q; want %s %s, body %q, content type %q", req.Method, req.URL.Path, body, req.Header.Get("Content-Type"), tc.method, tc.path, tc.body, contentType)
				}
				query, err := url.ParseQuery(req.URL.RawQuery)
				if err != nil {
					t.Fatalf("parse query: %v", err)
				}
				wantQuery := tc.query
				if wantQuery == nil {
					wantQuery = url.Values{}
				}
				if !reflect.DeepEqual(query, wantQuery) {
					t.Errorf("query = %v, want %v", query, wantQuery)
				}
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(bytes.NewBufferString(tc.responseBody))}, nil
			})
			client, err := NewClientWithResponses("http://example.test", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := tc.call(client, func(_ context.Context, _ *http.Request) error { return nil })
			if err != nil || response == nil || reflect.ValueOf(response).IsNil() {
				t.Fatalf("response = %v, error = %v", response, err)
			}
			if calls != 1 {
				t.Errorf("transport calls = %d, want 1", calls)
			}
			metadata := response.(round2ResponseMetadata)
			if metadata.StatusCode() != tc.status || !bytes.Equal(metadata.GetBody(), []byte(tc.responseBody)) {
				t.Errorf("status = %d, body = %q; want %d, %q", metadata.StatusCode(), metadata.GetBody(), tc.status, tc.responseBody)
			}
			switch r := response.(type) {
			case *DeleteV1NodesResponse:
				want := map[string]interface{}{"message": "accepted", "count": float64(1)}
				if r.JSON200 == nil || !reflect.DeepEqual(*r.JSON200, want) {
					t.Errorf("decoded deletion = %v, want %v", r.JSON200, want)
				}
			case *PutV1NodesNodeUuidTagsResponse:
				if r.JSON200 == nil || r.JSON200.NodeUUID == nil || *r.JSON200.NodeUUID != node || r.JSON200.Tags == nil || !reflect.DeepEqual(*r.JSON200.Tags, []string{"gpu-health", "node123"}) {
					t.Errorf("decoded tags = %+v", r.JSON200)
				}
			}
			sentinel := errors.New("editor rejected request")
			calls = 0
			response, err = tc.call(client, func(_ context.Context, _ *http.Request) error { return sentinel })
			if !errors.Is(err, sentinel) || response == nil || !reflect.ValueOf(response).IsNil() || calls != 0 {
				t.Errorf("editor failure: response = %v, error = %v, calls = %d; want nil response, sentinel, zero calls", response, err, calls)
			}
			req, err := tc.construct()
			if err == nil || req != nil {
				t.Errorf("malformed URL: request = %v, error = %v; want nil request and error", req, err)
			}
		})
	}
}

func TestRound2RawMuteNotificationAndTagRequests(t *testing.T) {
	const contentType = "application/json; charset=utf-8"
	const id = "11111111-1111-4111-8111-111111111111"
	const muteBody = `{"component":"GPU","severities":["Warning"]}`
	const notificationBody = `{"components":["GPU"],"notificationTargets":[{"channel":"email","config":{"recipients":["ops@example.test"]}}],"notifiedOn":["Critical"]}`
	const tagBody = `{"nodeUUIDs":["11111111-1111-4111-8111-111111111111"],"add":["gpu-health"],"remove":["legacy"]}`
	tests := []struct {
		name, method, path, body, responseBody string
		status                                 int
		call                                   func(*ClientWithResponses, io.Reader, RequestEditorFn) (interface{}, error)
		construct                              func() (*http.Request, error)
	}{
		{
			name: "delete_mute_rules", method: http.MethodDelete, path: "/v1/alert_mute_rules", body: muteBody, status: http.StatusNoContent,
			call: func(c *ClientWithResponses, body io.Reader, editor RequestEditorFn) (interface{}, error) {
				return c.DeleteV1AlertMuteRulesWithBodyWithResponse(context.Background(), contentType, body, editor)
			},
			construct: func() (*http.Request, error) {
				return NewDeleteV1AlertMuteRulesRequestWithBody("http://[::1", contentType, bytes.NewReader(nil))
			},
		},
		{
			name: "put_mute_rules", method: http.MethodPut, path: "/v1/alert_mute_rules", body: muteBody, status: http.StatusNoContent,
			call: func(c *ClientWithResponses, body io.Reader, editor RequestEditorFn) (interface{}, error) {
				return c.PutV1AlertMuteRulesWithBodyWithResponse(context.Background(), contentType, body, editor)
			},
			construct: func() (*http.Request, error) {
				return NewPutV1AlertMuteRulesRequestWithBody("http://[::1", contentType, bytes.NewReader(nil))
			},
		},
		{
			name: "create_notification", method: http.MethodPost, path: "/v1/notification_preferences", body: notificationBody,
			status: http.StatusCreated, responseBody: `{"id":"11111111-1111-4111-8111-111111111111"}`,
			call: func(c *ClientWithResponses, body io.Reader, editor RequestEditorFn) (interface{}, error) {
				return c.PostV1NotificationPreferencesWithBodyWithResponse(context.Background(), contentType, body, editor)
			},
			construct: func() (*http.Request, error) {
				return NewPostV1NotificationPreferencesRequestWithBody("http://[::1", contentType, bytes.NewReader(nil))
			},
		},
		{
			name: "update_notification", method: http.MethodPut, path: "/v1/notification_preferences/" + id, body: notificationBody, status: http.StatusNoContent,
			call: func(c *ClientWithResponses, body io.Reader, editor RequestEditorFn) (interface{}, error) {
				return c.PutV1NotificationPreferenceByIDWithBodyWithResponse(context.Background(), id, contentType, body, editor)
			},
			construct: func() (*http.Request, error) {
				return NewPutV1NotificationPreferenceByIDRequestWithBody("http://[::1", id, contentType, bytes.NewReader(nil))
			},
		},
		{
			name: "patch_tags", method: http.MethodPatch, path: "/v1/tags", body: tagBody, status: http.StatusNoContent,
			call: func(c *ClientWithResponses, body io.Reader, editor RequestEditorFn) (interface{}, error) {
				return c.PatchV1TagsWithBodyWithResponse(context.Background(), contentType, body, editor)
			},
			construct: func() (*http.Request, error) {
				return NewPatchV1TagsRequestWithBody("http://[::1", contentType, bytes.NewReader(nil))
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			doer := authStatusTestDoer(func(req *http.Request) (*http.Response, error) {
				calls++
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatalf("read request body: %v", err)
				}
				if req.Method != tc.method || req.URL.Path != tc.path || req.URL.RawQuery != "" || req.Header.Get("Content-Type") != contentType || !bytes.Equal(body, []byte(tc.body)) {
					t.Errorf("request = %s %s?%s, content type %q, body %q; want %s %s with empty query, content type %q, body %q", req.Method, req.URL.Path, req.URL.RawQuery, req.Header.Get("Content-Type"), body, tc.method, tc.path, contentType, tc.body)
				}
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": []string{contentType}}, Body: io.NopCloser(bytes.NewBufferString(tc.responseBody))}, nil
			})
			client, err := NewClientWithResponses("http://example.test", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			response, err := tc.call(client, bytes.NewBufferString(tc.body), func(_ context.Context, _ *http.Request) error { return nil })
			if err != nil || response == nil || reflect.ValueOf(response).IsNil() {
				t.Fatalf("response = %v, error = %v", response, err)
			}
			if calls != 1 {
				t.Errorf("transport calls = %d, want 1", calls)
			}
			metadata := response.(round2ResponseMetadata)
			if metadata.StatusCode() != tc.status || !bytes.Equal(metadata.GetBody(), []byte(tc.responseBody)) {
				t.Errorf("status = %d, body = %q; want %d, %q", metadata.StatusCode(), metadata.GetBody(), tc.status, tc.responseBody)
			}
			if tc.name == "create_notification" {
				created := response.(*PostV1NotificationPreferencesResponse)
				if created.JSON201 == nil || created.JSON201.Id != id {
					t.Errorf("decoded notification = %+v, want ID %q", created.JSON201, id)
				}
			}
			sentinel := errors.New("editor rejected request")
			calls = 0
			response, err = tc.call(client, bytes.NewBufferString(tc.body), func(_ context.Context, _ *http.Request) error { return sentinel })
			if !errors.Is(err, sentinel) || response == nil || !reflect.ValueOf(response).IsNil() || calls != 0 {
				t.Errorf("editor failure: response = %v, error = %v, calls = %d; want nil response, sentinel, zero calls", response, err, calls)
			}
			req, err := tc.construct()
			if err == nil || req != nil {
				t.Errorf("malformed URL: request = %v, error = %v; want nil request and error", req, err)
			}
		})
	}
}

func TestRound2GeneratedGetEditorFailures(t *testing.T) {
	sentinel := errors.New("editor rejected request")
	tests := []struct {
		name string
		path string
		call func(*ClientWithResponses, RequestEditorFn) (bool, error)
	}{
		{
			name: "computezones", path: "/v1/computezones",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1ComputezonesWithResponse(context.Background(), &GetV1ComputezonesParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "events", path: "/v1/events",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1EventsWithResponse(context.Background(), &GetV1EventsParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "nodegroups", path: "/v1/nodegroups",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NodegroupsWithResponse(context.Background(), &GetV1NodegroupsParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "nodes", path: "/v1/nodes",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NodesWithResponse(context.Background(), &GetV1NodesParams{}, editor)
				return response == nil, err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			doer := authStatusTestDoer(func(_ *http.Request) (*http.Response, error) {
				calls++
				return nil, errors.New("unexpected transport call")
			})
			client, err := NewClientWithResponses("http://example.test", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			editorCalls := 0
			responseNil, err := tc.call(client, func(_ context.Context, req *http.Request) error {
				editorCalls++
				if req.Method != http.MethodGet || req.URL.Path != tc.path {
					t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, tc.path)
				}
				return sentinel
			})
			if !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
			if !responseNil {
				t.Error("response is non-nil, want nil")
			}
			if editorCalls != 1 {
				t.Errorf("editor calls = %d, want 1", editorCalls)
			}
			if calls != 0 {
				t.Errorf("transport calls = %d, want 0", calls)
			}

			malformedClient, err := NewClientWithResponses("http://[::1", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new malformed-server client: %v", err)
			}
			editorCalls, calls = 0, 0
			responseNil, err = tc.call(malformedClient, func(_ context.Context, _ *http.Request) error {
				editorCalls++
				return nil
			})
			if err == nil {
				t.Error("malformed server: expected URL construction error")
			}
			if !responseNil {
				t.Error("malformed server: response is non-nil, want nil")
			}
			if editorCalls != 0 || calls != 0 {
				t.Errorf("malformed server: editor calls = %d, transport calls = %d; want 0 and 0", editorCalls, calls)
			}
		})
	}
}

func TestRound2GeneratedReadRequestFailures(t *testing.T) {
	sentinel := errors.New("editor rejected request")
	tests := []struct {
		name string
		path string
		call func(*ClientWithResponses, RequestEditorFn) (bool, error)
	}{
		{
			name: "event buckets", path: "/v1/events/buckets",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				mode, window := GetV1EventsBucketsParamsTimeModeRelative, "24h"
				response, err := c.GetV1EventsBucketsWithResponse(context.Background(), &GetV1EventsBucketsParams{TimeMode: &mode, Window: &window}, editor)
				return response == nil, err
			},
		},
		{
			name: "metrics", path: "/v1/metrics",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1MetricsWithResponse(context.Background(), &GetV1MetricsParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "overview", path: "/v1/overview",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1OverviewWithResponse(context.Background(), &GetV1OverviewParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "inventory", path: "/v1/reports/inventory",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1ReportsInventoryWithResponse(context.Background(), &GetV1ReportsInventoryParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "burst detail", path: "/v1/xid/bursts/burst-1",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1XIDBurstDetailWithResponse(context.Background(), "burst-1", editor)
				return response == nil, err
			},
		},
		{
			name: "commands", path: "/v2/commands",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV2CommandsWithResponse(context.Background(), editor)
				return response == nil, err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			transportCalls := 0
			doer := authStatusTestDoer(func(_ *http.Request) (*http.Response, error) {
				transportCalls++
				return nil, errors.New("unexpected transport call")
			})
			client, err := NewClientWithResponses("http://example.test", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			editorCalls := 0
			responseNil, err := tc.call(client, func(_ context.Context, req *http.Request) error {
				editorCalls++
				if req.Method != http.MethodGet || req.URL.Path != tc.path {
					t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, tc.path)
				}
				return sentinel
			})
			if !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
			if !responseNil {
				t.Error("response is non-nil, want nil")
			}
			if editorCalls != 1 || transportCalls != 0 {
				t.Errorf("editor calls = %d, transport calls = %d; want 1 and 0", editorCalls, transportCalls)
			}

			malformedClient, err := NewClientWithResponses("http://[::1", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new malformed-server client: %v", err)
			}
			editorCalls, transportCalls = 0, 0
			responseNil, err = tc.call(malformedClient, func(_ context.Context, _ *http.Request) error {
				editorCalls++
				return nil
			})
			if err == nil {
				t.Error("malformed server: expected URL construction error")
			}
			if !responseNil {
				t.Error("malformed server: response is non-nil, want nil")
			}
			if editorCalls != 0 || transportCalls != 0 {
				t.Errorf("malformed server: editor calls = %d, transport calls = %d; want 0 and 0", editorCalls, transportCalls)
			}
		})
	}
}

func TestRound2AlertRequestFailures(t *testing.T) {
	sentinel := errors.New("editor rejected request")
	tests := []struct {
		name string
		path string
		call func(*ClientWithResponses, RequestEditorFn) (bool, error)
	}{
		{
			name: "timeline filter options", path: "/v1/alert_timeline/filter_options",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1AlertTimelineFilterOptionsWithResponse(context.Background(), &GetV1AlertTimelineFilterOptionsParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "timeline nodes", path: "/v1/alert_timeline/nodes",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1AlertTimelineNodesWithResponse(context.Background(), &GetV1AlertTimelineNodesParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "timeline node alerts", path: "/v1/alert_timeline/nodes/node-1/alerts",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1AlertTimelineNodesNodeUuidAlertsWithResponse(context.Background(), "node-1", &GetV1AlertTimelineNodesNodeUuidAlertsParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "timeline node alert", path: "/v1/alert_timeline/nodes/node-1/alerts/alert-1",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidWithResponse(context.Background(), "node-1", "alert-1", &GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "alerts", path: "/v1/alerts",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1AlertsWithResponse(context.Background(), &GetV1AlertsParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "node alerts", path: "/v1/alerts/node-1",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1AlertsNodeUuidWithResponse(context.Background(), "node-1", &GetV1AlertsNodeUuidParams{}, editor)
				return response == nil, err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			transportCalls := 0
			doer := authStatusTestDoer(func(_ *http.Request) (*http.Response, error) {
				transportCalls++
				return nil, errors.New("unexpected transport call")
			})
			client, err := NewClientWithResponses("http://example.test", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			editorCalls := 0
			responseNil, err := tc.call(client, func(_ context.Context, req *http.Request) error {
				editorCalls++
				if req.Method != http.MethodGet || req.URL.Path != tc.path {
					t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, tc.path)
				}
				return sentinel
			})
			if !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
			if !responseNil {
				t.Error("response is non-nil, want nil")
			}
			if editorCalls != 1 || transportCalls != 0 {
				t.Errorf("editor calls = %d, transport calls = %d; want 1 and 0", editorCalls, transportCalls)
			}

			malformedClient, err := NewClientWithResponses("http://[::1", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new malformed-server client: %v", err)
			}
			editorCalls, transportCalls = 0, 0
			responseNil, err = tc.call(malformedClient, func(_ context.Context, _ *http.Request) error {
				editorCalls++
				return nil
			})
			if err == nil {
				t.Error("malformed server: expected URL construction error")
			}
			if !responseNil {
				t.Error("malformed server: response is non-nil, want nil")
			}
			if editorCalls != 0 || transportCalls != 0 {
				t.Errorf("malformed server: editor calls = %d, transport calls = %d; want 0 and 0", editorCalls, transportCalls)
			}
		})
	}
}

func TestRound2NotificationReportReadRequestFailures(t *testing.T) {
	sentinel := errors.New("editor rejected request")
	tests := []struct {
		name   string
		method string
		path   string
		call   func(*ClientWithResponses, RequestEditorFn) (bool, error)
	}{
		{
			name: "notification delivery log", method: http.MethodGet, path: "/v1/notification_delivery_log",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NotificationDeliveryLogWithResponse(context.Background(), &GetV1NotificationDeliveryLogParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "notification preferences", method: http.MethodGet, path: "/v1/notification_preferences",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NotificationPreferencesWithResponse(context.Background(), editor)
				return response == nil, err
			},
		},
		{
			name: "notification preference", method: http.MethodGet, path: "/v1/notification_preferences/preference-1",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NotificationPreferenceByIDWithResponse(context.Background(), "preference-1", editor)
				return response == nil, err
			},
		},
		{
			name: "delete notification preference", method: http.MethodDelete, path: "/v1/notification_preferences/preference-1",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.DeleteV1NotificationPreferenceByIDWithResponse(context.Background(), "preference-1", editor)
				return response == nil, err
			},
		},
		{
			name: "predicted health rankings", method: http.MethodGet, path: "/v1/predicted_health_rankings",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1PredictedHealthRankingsWithResponse(context.Background(), &GetV1PredictedHealthRankingsParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "error report", method: http.MethodGet, path: "/v1/reports/error",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1ReportsErrorWithResponse(context.Background(), &GetV1ReportsErrorParams{View: Overview}, editor)
				return response == nil, err
			},
		},
		{
			name: "security vulnerabilities", method: http.MethodGet, path: "/v1/reports/security/vulnerabilities",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1ReportsSecurityVulnerabilitiesWithResponse(context.Background(), &GetV1ReportsSecurityVulnerabilitiesParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "security vulnerability options", method: http.MethodGet, path: "/v1/reports/security/vulnerabilities/options",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1ReportsSecurityVulnerabilityOptionsWithResponse(context.Background(), editor)
				return response == nil, err
			},
		},
		{
			name: "security vulnerability nodes", method: http.MethodGet, path: "/v1/reports/security/vulnerabilities/CVE-2026-1234/nodes",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1ReportsSecurityVulnerabilityNodesWithResponse(context.Background(), "CVE-2026-1234", &GetV1ReportsSecurityVulnerabilityNodesParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "stats", method: http.MethodGet, path: "/v1/stats",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				dataSource := GetV1StatsParamsDataSourceInband
				response, err := c.GetV1StatsWithResponse(context.Background(), &GetV1StatsParams{Name: GetV1StatsParamsNameAgentStatus, DataSource: &dataSource, GroupBy: GetV1StatsParamsGroupByCustomer, Window: "72h"}, editor)
				return response == nil, err
			},
		},
		{
			name: "tags", method: http.MethodGet, path: "/v1/tags",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1TagsWithResponse(context.Background(), &GetV1TagsParams{}, editor)
				return response == nil, err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			transportCalls := 0
			doer := authStatusTestDoer(func(_ *http.Request) (*http.Response, error) {
				transportCalls++
				return nil, errors.New("unexpected transport call")
			})
			client, err := NewClientWithResponses("http://example.test", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			editorCalls := 0
			responseNil, err := tc.call(client, func(_ context.Context, req *http.Request) error {
				editorCalls++
				if req.Method != tc.method || req.URL.Path != tc.path {
					t.Errorf("request = %s %s, want %s %s", req.Method, req.URL.Path, tc.method, tc.path)
				}
				return sentinel
			})
			if !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
			if !responseNil {
				t.Error("response is non-nil, want nil")
			}
			if editorCalls != 1 || transportCalls != 0 {
				t.Errorf("editor calls = %d, transport calls = %d; want 1 and 0", editorCalls, transportCalls)
			}

			malformedClient, err := NewClientWithResponses("http://[::1", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new malformed-server client: %v", err)
			}
			editorCalls, transportCalls = 0, 0
			responseNil, err = tc.call(malformedClient, func(_ context.Context, _ *http.Request) error {
				editorCalls++
				return nil
			})
			if err == nil {
				t.Error("malformed server: expected URL construction error")
			}
			if !responseNil {
				t.Error("malformed server: response is non-nil, want nil")
			}
			if editorCalls != 0 || transportCalls != 0 {
				t.Errorf("malformed server: editor calls = %d, transport calls = %d; want 0 and 0", editorCalls, transportCalls)
			}
		})
	}
}

func TestRound2TypedMutationRequestFailures(t *testing.T) {
	const nodeUUID = "11111111-1111-4111-8111-111111111111"
	const muteFixture = `{"component":"GPU","severities":["Warning"]}`
	const zoneFixture = `{"id":"cz-1"}`
	const removalFixture = `{"enabled":true,"offlineThresholdDays":14}`
	const nodesFixture = `{"nodeUUIDs":["11111111-1111-4111-8111-111111111111"]}`
	const nodeTagsFixture = `{"tags":["gpu-health"]}`
	const notificationFixture = `{"components":["GPU"],"notificationTargets":[{"channel":"email","config":{"recipients":["ops@example.test"]}}],"notifiedOn":["Critical"]}`
	const retentionFixture = `{"retentionDays":365}`
	const batchTagsFixture = `{"nodeUUIDs":["11111111-1111-4111-8111-111111111111"],"add":["gpu-health"]}`
	sentinel := errors.New("editor rejected request")
	tests := []struct {
		name, method, path, fixture string
		call                        func(*testing.T, *ClientWithResponses, RequestEditorFn) (bool, error)
	}{
		{
			name: "delete mute", method: http.MethodDelete, path: "/v1/alert_mute_rules", fixture: muteFixture,
			call: func(t *testing.T, c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				var body DeleteV1AlertMuteRulesJSONRequestBody
				if err := json.Unmarshal([]byte(muteFixture), &body); err != nil {
					t.Fatalf("decode fixture: %v", err)
				}
				response, err := c.DeleteV1AlertMuteRulesWithResponse(context.Background(), body, editor)
				return response == nil, err
			},
		},
		{
			name: "update mute", method: http.MethodPut, path: "/v1/alert_mute_rules", fixture: muteFixture,
			call: func(t *testing.T, c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				var body PutV1AlertMuteRulesJSONRequestBody
				if err := json.Unmarshal([]byte(muteFixture), &body); err != nil {
					t.Fatalf("decode fixture: %v", err)
				}
				response, err := c.PutV1AlertMuteRulesWithResponse(context.Background(), body, editor)
				return response == nil, err
			},
		},
		{
			name: "compute zone", method: http.MethodPut, path: "/v1/computezones", fixture: zoneFixture,
			call: func(t *testing.T, c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				var body PutV1ComputezonesJSONRequestBody
				if err := json.Unmarshal([]byte(zoneFixture), &body); err != nil {
					t.Fatalf("decode fixture: %v", err)
				}
				response, err := c.PutV1ComputezonesWithResponse(context.Background(), body, editor)
				return response == nil, err
			},
		},
		{
			name: "node removal", method: http.MethodPut, path: "/v1/node_removal_policy", fixture: removalFixture,
			call: func(t *testing.T, c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				var body PutV1NodeRemovalPolicyJSONRequestBody
				if err := json.Unmarshal([]byte(removalFixture), &body); err != nil {
					t.Fatalf("decode fixture: %v", err)
				}
				response, err := c.PutV1NodeRemovalPolicyWithResponse(context.Background(), body, editor)
				return response == nil, err
			},
		},
		{
			name: "delete nodes", method: http.MethodDelete, path: "/v1/nodes", fixture: nodesFixture,
			call: func(t *testing.T, c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				var body DeleteV1NodesJSONRequestBody
				if err := json.Unmarshal([]byte(nodesFixture), &body); err != nil {
					t.Fatalf("decode fixture: %v", err)
				}
				response, err := c.DeleteV1NodesWithResponse(context.Background(), &DeleteV1NodesParams{}, body, editor)
				return response == nil, err
			},
		},
		{
			name: "node tags", method: http.MethodPut, path: "/v1/nodes/11111111-1111-4111-8111-111111111111/tags", fixture: nodeTagsFixture,
			call: func(t *testing.T, c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				var body PutV1NodesNodeUuidTagsJSONRequestBody
				if err := json.Unmarshal([]byte(nodeTagsFixture), &body); err != nil {
					t.Fatalf("decode fixture: %v", err)
				}
				response, err := c.PutV1NodesNodeUuidTagsWithResponse(context.Background(), nodeUUID, body, editor)
				return response == nil, err
			},
		},
		{
			name: "create notification", method: http.MethodPost, path: "/v1/notification_preferences", fixture: notificationFixture,
			call: func(t *testing.T, c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				var body PostV1NotificationPreferencesJSONRequestBody
				if err := json.Unmarshal([]byte(notificationFixture), &body); err != nil {
					t.Fatalf("decode fixture: %v", err)
				}
				response, err := c.PostV1NotificationPreferencesWithResponse(context.Background(), body, editor)
				return response == nil, err
			},
		},
		{
			name: "update notification", method: http.MethodPut, path: "/v1/notification_preferences/preference-1", fixture: notificationFixture,
			call: func(t *testing.T, c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				var body PutV1NotificationPreferenceByIDJSONRequestBody
				if err := json.Unmarshal([]byte(notificationFixture), &body); err != nil {
					t.Fatalf("decode fixture: %v", err)
				}
				response, err := c.PutV1NotificationPreferenceByIDWithResponse(context.Background(), "preference-1", body, editor)
				return response == nil, err
			},
		},
		{
			name: "retention", method: http.MethodPut, path: "/v1/retention_policy", fixture: retentionFixture,
			call: func(t *testing.T, c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				var body PutV1RetentionPolicyJSONRequestBody
				if err := json.Unmarshal([]byte(retentionFixture), &body); err != nil {
					t.Fatalf("decode fixture: %v", err)
				}
				response, err := c.PutV1RetentionPolicyWithResponse(context.Background(), body, editor)
				return response == nil, err
			},
		},
		{
			name: "batch tags", method: http.MethodPatch, path: "/v1/tags", fixture: batchTagsFixture,
			call: func(t *testing.T, c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				var body PatchV1TagsJSONRequestBody
				if err := json.Unmarshal([]byte(batchTagsFixture), &body); err != nil {
					t.Fatalf("decode fixture: %v", err)
				}
				response, err := c.PatchV1TagsWithResponse(context.Background(), body, editor)
				return response == nil, err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var expected interface{}
			if err := json.Unmarshal([]byte(tc.fixture), &expected); err != nil {
				t.Fatalf("decode expected fixture: %v", err)
			}
			transportCalls := 0
			doer := authStatusTestDoer(func(_ *http.Request) (*http.Response, error) {
				transportCalls++
				return nil, errors.New("unexpected transport call")
			})
			client, err := NewClientWithResponses("http://example.test", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			editorCalls := 0
			responseNil, err := tc.call(t, client, func(_ context.Context, req *http.Request) error {
				editorCalls++
				if req.Method != tc.method || req.URL.Path != tc.path {
					t.Errorf("request = %s %s, want %s %s", req.Method, req.URL.Path, tc.method, tc.path)
				}
				if got := req.Header.Get("Content-Type"); got != "application/json" {
					t.Errorf("content type = %q, want application/json", got)
				}
				var actual interface{}
				if err := json.NewDecoder(req.Body).Decode(&actual); err != nil {
					t.Errorf("decode request body: %v", err)
				} else if !reflect.DeepEqual(actual, expected) {
					t.Errorf("request body = %#v, want %#v", actual, expected)
				}
				return sentinel
			})
			if !errors.Is(err, sentinel) || !responseNil {
				t.Errorf("editor failure: nil response = %t, error = %v; want nil response and sentinel", responseNil, err)
			}
			if editorCalls != 1 || transportCalls != 0 {
				t.Errorf("editor calls = %d, transport calls = %d; want 1 and 0", editorCalls, transportCalls)
			}

			malformedClient, err := NewClientWithResponses("http://[::1", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new malformed-server client: %v", err)
			}
			editorCalls, transportCalls = 0, 0
			responseNil, err = tc.call(t, malformedClient, func(_ context.Context, _ *http.Request) error {
				editorCalls++
				return nil
			})
			if err == nil || !responseNil {
				t.Errorf("malformed server: nil response = %t, error = %v; want nil response and construction error", responseNil, err)
			}
			if editorCalls != 0 || transportCalls != 0 {
				t.Errorf("malformed server: editor calls = %d, transport calls = %d; want 0 and 0", editorCalls, transportCalls)
			}
		})
	}
}

type round2NodeReportResponse interface {
	comparable
	round2ResponseMetadata
	GetJSON400() *ModelsErrorResponse
	GetJSON401() *ModelsErrorResponse
	GetJSON403() *ModelsErrorResponse
	GetJSON499() *ModelsErrorResponse
	GetJSON500() *ModelsErrorResponse
	GetJSON503() *ModelsErrorResponse
}

type round2NodeReportClosingReader struct {
	io.Reader
	closed bool
}

func (r *round2NodeReportClosingReader) Close() error {
	r.closed = true
	return nil
}

func round2NodeReportErrors[R round2NodeReportResponse](rsp R) map[int]*ModelsErrorResponse {
	result := map[int]*ModelsErrorResponse{400: rsp.GetJSON400(), 401: rsp.GetJSON401(), 403: rsp.GetJSON403(), 499: rsp.GetJSON499(), 500: rsp.GetJSON500(), 503: rsp.GetJSON503()}
	if with404, ok := any(rsp).(interface{ GetJSON404() *ModelsErrorResponse }); ok {
		result[404] = with404.GetJSON404()
	}
	return result
}

func round2NodeReportHTTPResponse(status int, contentType string, body io.ReadCloser) *http.Response {
	return &http.Response{Status: fmt.Sprintf("%d supplied status", status), StatusCode: status,
		Header: http.Header{"Content-Type": []string{contentType}}, Body: body}
}

func testRound2NodeReportResponse[R round2NodeReportResponse](t *testing.T, zero R, parse func(*http.Response) (R, error), checkNoJSON func(*testing.T, R), success func(*testing.T, func(*http.Response) (R, error))) {
	t.Helper()
	const jsonType = "application/json; charset=utf-8"
	assertRound2ResponseMetadata(t, zero, nil, "", 0, "")
	assertRound2AlertTimelineError(t, 0, round2NodeReportErrors(zero))
	checkNoJSON(t, zero)
	body := []byte(`{"error":"denied","details":"specific detail"}`)
	statuses := []int{400, 401, 403, 499, 500, 503}
	if _, ok := round2NodeReportErrors(zero)[404]; ok {
		statuses = []int{400, 401, 403, 404, 499, 500, 503}
	}
	for _, status := range statuses {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			rsp, err := parse(round2NodeReportHTTPResponse(status, jsonType, io.NopCloser(bytes.NewReader(body))))
			var nilResponse R
			if err != nil || rsp == nilResponse {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, fmt.Sprintf("%d supplied status", status), status, jsonType)
			assertRound2AlertTimelineError(t, status, round2NodeReportErrors(rsp))
			checkNoJSON(t, rsp)
		})
	}
	t.Run("malformed", func(t *testing.T) {
		reader := &round2NodeReportClosingReader{Reader: bytes.NewBufferString(`{"error":`)}
		rsp, err := parse(round2NodeReportHTTPResponse(400, jsonType, reader))
		var nilResponse R
		if err == nil || rsp != nilResponse || !reader.closed {
			t.Errorf("parse = %v, %v; closed = %t; want nil response, decoding error and closed body", rsp, err, reader.closed)
		}
	})
	t.Run("read_error", func(t *testing.T) {
		sentinel := errors.New("node report read error")
		reader := &round2AlertTimelineErrorReader{err: sentinel}
		rsp, err := parse(round2NodeReportHTTPResponse(400, jsonType, reader))
		var nilResponse R
		if !errors.Is(err, sentinel) || rsp != nilResponse || !reader.closed {
			t.Errorf("parse = %v, %v; closed = %t; want nil response, sentinel error and closed body", rsp, err, reader.closed)
		}
	})
	t.Run("text_plain", func(t *testing.T) {
		rsp, err := parse(round2NodeReportHTTPResponse(400, "text/plain", io.NopCloser(bytes.NewReader(body))))
		var nilResponse R
		if err != nil || rsp == nilResponse {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, body, "400 supplied status", 400, "text/plain")
		assertRound2AlertTimelineError(t, 0, round2NodeReportErrors(rsp))
		checkNoJSON(t, rsp)
	})
	success(t, parse)
}

func round2NodeReportSuccess[R round2NodeReportResponse](t *testing.T, parse func(*http.Response) (R, error), contentType, body string, headers http.Header) R {
	t.Helper()
	httpResponse := round2NodeReportHTTPResponse(200, contentType, io.NopCloser(bytes.NewBufferString(body)))
	for key, values := range headers {
		httpResponse.Header[key] = values
	}
	rsp, err := parse(httpResponse)
	var nilResponse R
	if err != nil || rsp == nilResponse {
		t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
	}
	assertRound2ResponseMetadata(t, rsp, []byte(body), "200 supplied status", 200, contentType)
	assertRound2AlertTimelineError(t, 0, round2NodeReportErrors(rsp))
	return rsp
}

func round2NodeReportInvalidHeader[R round2NodeReportResponse](t *testing.T, parse func(*http.Response) (R, error)) {
	t.Helper()
	rsp := round2NodeReportHTTPResponse(200, "text/csv", io.NopCloser(bytes.NewBufferString("node\n")))
	rsp.Header.Set("X-Total-Count", "not-an-integer")
	result, err := parse(rsp)
	var nilResponse R
	if err == nil || result != nilResponse {
		t.Errorf("parse = %v, %v; want nil response and header parsing error", result, err)
	}
}

func TestRound2NodeDetailAndReportResponseContracts(t *testing.T) {
	const jsonType = "application/json; charset=utf-8"
	const csvBody = "nodeUUID,hostname\nnode-1,host-1\n"
	headers := http.Header{"Content-Disposition": []string{"attachment; filename=report.csv"}, "X-Total-Count": []string{"3"}}
	t.Run("node_detail", func(t *testing.T) {
		testRound2NodeReportResponse(t, &GetV1NodesNodeUuidResponse{}, ParseGetV1NodesNodeUuidResponse,
			func(t *testing.T, r *GetV1NodesNodeUuidResponse) {
				assertRound2ResponsePointer(t, "GetJSON200", r.GetJSON200(), nil)
			},
			func(t *testing.T, parse func(*http.Response) (*GetV1NodesNodeUuidResponse, error)) {
				r := round2NodeReportSuccess(t, parse, jsonType, `{"nodeUUID":"node-1","dataSource":"inband","hostname":"host-1"}`, nil)
				if r.GetJSON200() == nil {
					t.Fatal("GetJSON200() = nil")
				}
				v, err := r.GetJSON200().AsModelsInbandNodeDetailsResponse()
				if err != nil || v.NodeUUID != "node-1" || v.DataSource != "inband" || v.Hostname == nil || *v.Hostname != "host-1" {
					t.Errorf("node detail = %+v, %v", v, err)
				}
			})
	})
	t.Run("error_report", func(t *testing.T) {
		testRound2NodeReportResponse(t, &GetV1ReportsErrorResponse{}, ParseGetV1ReportsErrorResponse,
			func(t *testing.T, r *GetV1ReportsErrorResponse) {
				assertRound2ResponsePointer(t, "GetJSON200", r.GetJSON200(), nil)
			},
			func(t *testing.T, parse func(*http.Response) (*GetV1ReportsErrorResponse, error)) {
				r := round2NodeReportSuccess(t, parse, jsonType, `{"totalErrors":12,"totalErrorNodes":4,"totalErrorTypes":3}`, nil)
				if r.GetJSON200() == nil {
					t.Fatal("GetJSON200() = nil")
				}
				v, err := r.GetJSON200().AsModelsErrorOverviewResponse()
				if err != nil || v.TotalErrors == nil || *v.TotalErrors != 12 || v.TotalErrorNodes == nil || *v.TotalErrorNodes != 4 || v.TotalErrorTypes == nil || *v.TotalErrorTypes != 3 {
					t.Errorf("error overview = %+v, %v", v, err)
				}
				csv := round2NodeReportSuccess(t, parse, "text/csv", csvBody, nil)
				assertRound2ResponsePointer(t, "GetJSON200", csv.GetJSON200(), nil)
			})
	})
	t.Run("inventory", func(t *testing.T) {
		testRound2NodeReportResponse(t, &GetV1ReportsInventoryResponse{}, ParseGetV1ReportsInventoryResponse,
			func(t *testing.T, r *GetV1ReportsInventoryResponse) {
				assertRound2ResponsePointer(t, "GetJSON200", r.GetJSON200(), nil)
			},
			func(t *testing.T, parse func(*http.Response) (*GetV1ReportsInventoryResponse, error)) {
				r := round2NodeReportSuccess(t, parse, jsonType, `{"nodes":[{"nodeUUID":"inband-1","hostname":"host-1"}],"page":2}`, nil)
				if r.GetJSON200() == nil {
					t.Fatal("GetJSON200() = nil")
				}
				v, err := r.GetJSON200().AsModelsInbandInventoryReportResponse()
				if err != nil || v.Page == nil || *v.Page != 2 || v.Nodes == nil || len(*v.Nodes) != 1 {
					t.Fatalf("inventory = %+v, %v", v, err)
				}
				n := (*v.Nodes)[0]
				if n.NodeUUID == nil || *n.NodeUUID != "inband-1" || n.Hostname == nil || *n.Hostname != "host-1" {
					t.Errorf("inventory node = %+v", n)
				}
				csv := round2NodeReportSuccess(t, parse, "text/csv", csvBody, nil)
				assertRound2ResponsePointer(t, "GetJSON200", csv.GetJSON200(), nil)
			})
	})
	t.Run("security_vulnerabilities", func(t *testing.T) {
		testRound2NodeReportResponse(t, &GetV1ReportsSecurityVulnerabilitiesResponse{}, ParseGetV1ReportsSecurityVulnerabilitiesResponse,
			func(t *testing.T, r *GetV1ReportsSecurityVulnerabilitiesResponse) {
				assertRound2ResponsePointer(t, "GetJSON200", r.GetJSON200(), nil)
			},
			func(t *testing.T, parse func(*http.Response) (*GetV1ReportsSecurityVulnerabilitiesResponse, error)) {
				r := round2NodeReportSuccess(t, parse, jsonType, `{"total":3}`, headers)
				if r.GetJSON200() == nil {
					t.Fatal("GetJSON200() = nil")
				}
				v, err := r.GetJSON200().AsGetV1ReportsSecurityVulnerabilities200JSONResponseBody1()
				if err != nil || v.Total != 3 {
					t.Errorf("security count = %+v, %v", v, err)
				}
				if r.Headers200 == nil || r.Headers200.ContentDisposition == nil || *r.Headers200.ContentDisposition != "attachment; filename=report.csv" || r.Headers200.XTotalCount == nil || *r.Headers200.XTotalCount != 3 {
					t.Errorf("headers = %+v", r.Headers200)
				}
				csv := round2NodeReportSuccess(t, parse, "text/csv", csvBody, headers)
				assertRound2ResponsePointer(t, "GetJSON200", csv.GetJSON200(), nil)
				if csv.Headers200 == nil || csv.Headers200.ContentDisposition == nil || *csv.Headers200.ContentDisposition != "attachment; filename=report.csv" || csv.Headers200.XTotalCount == nil || *csv.Headers200.XTotalCount != 3 {
					t.Errorf("CSV headers = %+v", csv.Headers200)
				}
				round2NodeReportInvalidHeader(t, parse)
			})
	})
	t.Run("vulnerability_nodes", func(t *testing.T) {
		testRound2NodeReportResponse(t, &GetV1ReportsSecurityVulnerabilityNodesResponse{}, ParseGetV1ReportsSecurityVulnerabilityNodesResponse,
			func(_ *testing.T, _ *GetV1ReportsSecurityVulnerabilityNodesResponse) {},
			func(t *testing.T, parse func(*http.Response) (*GetV1ReportsSecurityVulnerabilityNodesResponse, error)) {
				r := round2NodeReportSuccess(t, parse, "text/csv", csvBody, headers)
				if r.Headers200 == nil || r.Headers200.ContentDisposition == nil || *r.Headers200.ContentDisposition != "attachment; filename=report.csv" || r.Headers200.XTotalCount == nil || *r.Headers200.XTotalCount != 3 {
					t.Errorf("headers = %+v", r.Headers200)
				}
				round2NodeReportInvalidHeader(t, parse)
			})
	})
}

func testRound2MuteRuleParser[R interface {
	comparable
	round2ResponseMetadata
}](t *testing.T, statuses []int, noContent bool, parse func(*http.Response) (R, error), errorGetters func(R) map[int]*ModelsErrorResponse, httpResponse func(R) *http.Response) {
	t.Helper()
	const jsonType = "application/json; charset=utf-8"
	body := []byte(`{"error":"denied","details":"specific detail"}`)
	for _, status := range statuses {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			input := round2NodeReportHTTPResponse(status, jsonType, io.NopCloser(bytes.NewReader(body)))
			rsp, err := parse(input)
			var zero R
			if err != nil || rsp == zero {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, body, input.Status, status, jsonType)
			if httpResponse(rsp) != input {
				t.Error("HTTPResponse was not preserved")
			}
			assertRound2AlertTimelineError(t, status, errorGetters(rsp))
		})
		t.Run(fmt.Sprintf("%d_malformed", status), func(t *testing.T) {
			reader := &round2NodeReportClosingReader{Reader: bytes.NewBufferString(`{"error":`)}
			rsp, err := parse(round2NodeReportHTTPResponse(status, jsonType, reader))
			var zero R
			if rsp != zero || err == nil || !reader.closed {
				t.Errorf("parse = %v, %v; closed = %t; want nil response, decoding error and closed body", rsp, err, reader.closed)
			}
		})
	}
	t.Run("read_error", func(t *testing.T) {
		sentinel := errors.New("mute rule read error")
		reader := &round2AlertTimelineErrorReader{err: sentinel}
		rsp, err := parse(round2NodeReportHTTPResponse(statuses[0], jsonType, reader))
		var zero R
		if rsp != zero || !errors.Is(err, sentinel) || !reader.closed {
			t.Errorf("parse = %v, %v; closed = %t; want nil response, sentinel error and closed body", rsp, err, reader.closed)
		}
	})
	t.Run("text_plain", func(t *testing.T) {
		input := round2NodeReportHTTPResponse(statuses[0], "text/plain", io.NopCloser(bytes.NewReader(body)))
		rsp, err := parse(input)
		var zero R
		if err != nil || rsp == zero {
			t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
		}
		assertRound2ResponseMetadata(t, rsp, body, input.Status, input.StatusCode, "text/plain")
		if httpResponse(rsp) != input {
			t.Error("HTTPResponse was not preserved")
		}
		assertRound2AlertTimelineError(t, 0, errorGetters(rsp))
	})
	if noContent {
		t.Run("no_content", func(t *testing.T) {
			input := round2NodeReportHTTPResponse(http.StatusNoContent, jsonType, io.NopCloser(bytes.NewReader(nil)))
			rsp, err := parse(input)
			var zero R
			if err != nil || rsp == zero {
				t.Fatalf("parse = %v, %v; want response and no error", rsp, err)
			}
			assertRound2ResponseMetadata(t, rsp, []byte{}, input.Status, http.StatusNoContent, jsonType)
			if httpResponse(rsp) != input {
				t.Error("HTTPResponse was not preserved")
			}
			assertRound2AlertTimelineError(t, 0, errorGetters(rsp))
		})
	}
}

func TestRound2MuteRuleParserErrorContracts(t *testing.T) {
	t.Run("delete", func(t *testing.T) {
		testRound2MuteRuleParser(t, []int{400, 401, 403, 404, 499, 500}, true, ParseDeleteV1AlertMuteRulesResponse,
			func(r *DeleteV1AlertMuteRulesResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: r.GetJSON400(), 401: r.GetJSON401(), 403: r.GetJSON403(), 404: r.GetJSON404(), 499: r.GetJSON499(), 500: r.GetJSON500()}
			}, func(r *DeleteV1AlertMuteRulesResponse) *http.Response { return r.HTTPResponse })
	})
	t.Run("get", func(t *testing.T) {
		testRound2MuteRuleParser(t, []int{401, 403, 499, 500}, false, ParseGetV1AlertMuteRulesResponse,
			func(r *GetV1AlertMuteRulesResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{401: r.GetJSON401(), 403: r.GetJSON403(), 499: r.GetJSON499(), 500: r.GetJSON500()}
			}, func(r *GetV1AlertMuteRulesResponse) *http.Response { return r.HTTPResponse })
	})
	t.Run("put", func(t *testing.T) {
		testRound2MuteRuleParser(t, []int{400, 401, 403, 499, 500}, true, ParsePutV1AlertMuteRulesResponse,
			func(r *PutV1AlertMuteRulesResponse) map[int]*ModelsErrorResponse {
				return map[int]*ModelsErrorResponse{400: r.GetJSON400(), 401: r.GetJSON401(), 403: r.GetJSON403(), 499: r.GetJSON499(), 500: r.GetJSON500()}
			}, func(r *PutV1AlertMuteRulesResponse) *http.Response { return r.HTTPResponse })
	})
}

func testRound2AlertParserMalformedStatuses[T any](t *testing.T, statuses []int, parse func(*http.Response) (*T, error)) {
	t.Helper()
	for _, status := range statuses {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			reader := &round2NodeReportClosingReader{Reader: bytes.NewBufferString(`{"error":`)}
			rsp, err := parse(round2NodeReportHTTPResponse(status, "application/json", reader))
			if rsp != nil {
				t.Errorf("response = %v, want nil", rsp)
			}
			var syntaxErr *json.SyntaxError
			if !errors.As(err, &syntaxErr) {
				t.Errorf("error = %v, want *json.SyntaxError", err)
			}
			if !reader.closed {
				t.Error("response body was not closed")
			}
		})
	}
}

func TestRound2SigningOverviewBucketsAndV2MetricsMalformedStatuses(t *testing.T) {
	t.Run("signing_key", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{499, 503}, ParseGetWellKnownSigningKeyPubResponse)
	})
	t.Run("overview", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500, 503}, ParseGetV1OverviewResponse)
	})
	t.Run("event_buckets", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500, 503}, ParseGetV1EventsBucketsResponse)
	})
	t.Run("v2_metrics", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200}, ParseGetV2MetricsResponse)
	})
}

func TestRound2AlertParserMalformedStatuses(t *testing.T) {
	t.Run("filter_options", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 500}, ParseGetV1AlertTimelineFilterOptionsResponse)
	})
	t.Run("timeline_nodes", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 499, 500}, ParseGetV1AlertTimelineNodesResponse)
	})
	t.Run("timeline_node_alerts", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 404, 499, 500}, ParseGetV1AlertTimelineNodesNodeUuidAlertsResponse)
	})
	t.Run("timeline_node_alert", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 404, 499, 500}, ParseGetV1AlertTimelineNodesNodeUuidAlertsAlertUuidResponse)
	})
	t.Run("alerts", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 499, 500}, ParseGetV1AlertsResponse)
	})
	t.Run("node_alerts", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 404, 499, 500}, ParseGetV1AlertsNodeUuidResponse)
	})
}

func TestRound2MetricsParserMalformedStatuses(t *testing.T) {
	t.Run("metrics", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 403, 499, 500}, ParseGetV1MetricsResponse)
	})
	t.Run("query_range", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 404, 499, 500, 503}, ParseGetV1MetricsQueryRangeResponse)
	})
}

func TestRound2NodeDetailTagsRankingsMalformedStatuses(t *testing.T) {
	t.Run("node_detail", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 404, 499, 500, 503}, ParseGetV1NodesNodeUuidResponse)
	})
	t.Run("node_tags", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 404, 499, 500}, ParsePutV1NodesNodeUuidTagsResponse)
	})
	t.Run("rankings", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500, 501, 503}, ParseGetV1PredictedHealthRankingsResponse)
	})
}

func TestRound2ResourceListParserMalformedStatuses(t *testing.T) {
	t.Run("get_compute_zones", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{401, 403, 499, 500, 503}, ParseGetV1ComputezonesResponse)
	})
	t.Run("put_compute_zones", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{401, 403, 499, 500}, ParsePutV1ComputezonesResponse)
	})
	t.Run("events", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{401, 403, 499, 500, 503}, ParseGetV1EventsResponse)
	})
	t.Run("node_groups", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{401, 403, 499, 500, 503}, ParseGetV1NodegroupsResponse)
	})
	t.Run("nodes", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{401, 403, 499, 500, 503}, ParseGetV1NodesResponse)
	})
}

func TestRound2ResourceParserMalformedStatuses(t *testing.T) {
	t.Run("event_options", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 403, 500, 503}, ParseGetV1EventsOptionsResponse)
	})
	t.Run("info", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 499, 500}, ParseGetV1InfoResponse)
	})
	t.Run("node_group_memberships", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500, 503}, ParseGetV1NodeGroupMembershipsResponse)
	})
	t.Run("get_node_removal_policy", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 403, 500, 503}, ParseGetV1NodeRemovalPolicyResponse)
	})
	t.Run("put_node_removal_policy", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{401, 403, 500, 503}, ParsePutV1NodeRemovalPolicyResponse)
	})
	t.Run("node_group_options", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 403, 499, 500}, ParseGetV1NodegroupsOptionsResponse)
	})
	t.Run("delete_nodes", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 404, 499, 500, 503}, ParseDeleteV1NodesResponse)
	})
	t.Run("delete_node", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 404, 499, 500, 503}, ParseDeleteV1NodesNodeUuidResponse)
	})
	t.Run("nodes_history", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 500, 503}, ParseGetV1NodesHistoryResponse)
	})
	t.Run("nodes_options", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500}, ParseGetV1NodesOptionsResponse)
	})
	t.Run("node_health_history", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 404, 499, 500}, ParseGetV1NodesNodeUuidHealthHistoryResponse)
	})
}

func TestRound2NotificationParserMalformedStatuses(t *testing.T) {
	t.Run("delivery_log", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500, 503}, ParseGetV1NotificationDeliveryLogResponse)
	})
	t.Run("preferences", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 403, 499, 500, 503}, ParseGetV1NotificationPreferencesResponse)
	})
	t.Run("preference_by_id", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 403, 404, 499, 500, 503}, ParseGetV1NotificationPreferenceByIDResponse)
	})
	t.Run("post_preferences", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{400, 401, 403, 499, 500, 503}, ParsePostV1NotificationPreferencesResponse)
	})
	t.Run("delete_preference_by_id", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{401, 403, 404, 499, 500, 503}, ParseDeleteV1NotificationPreferenceByIDResponse)
	})
	t.Run("put_preference_by_id", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{401, 403, 404, 499, 500, 503}, ParsePutV1NotificationPreferenceByIDResponse)
	})
}

func TestRound2ReportParserMalformedStatuses(t *testing.T) {
	t.Run("error", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500, 503}, ParseGetV1ReportsErrorResponse)
	})
	t.Run("inventory", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500, 503}, ParseGetV1ReportsInventoryResponse)
	})
	t.Run("security_vulnerabilities", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500, 503}, ParseGetV1ReportsSecurityVulnerabilitiesResponse)
	})
	t.Run("security_vulnerability_options", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 422, 499, 500, 503}, ParseGetV1ReportsSecurityVulnerabilityOptionsResponse)
	})
	t.Run("security_vulnerability_nodes", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{401, 403, 404, 499, 500, 503}, ParseGetV1ReportsSecurityVulnerabilityNodesResponse)
	})
}

func TestRound2PolicyTagsXIDParserMalformedStatuses(t *testing.T) {
	t.Run("get_retention_policy", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 403, 404, 499, 500, 503}, ParseGetV1RetentionPolicyResponse)
	})
	t.Run("put_retention_policy", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{401, 403, 499, 500, 503}, ParsePutV1RetentionPolicyResponse)
	})
	t.Run("stats", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 404, 499, 500, 503}, ParseGetV1StatsResponse)
	})
	t.Run("get_tags", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500}, ParseGetV1TagsResponse)
	})
	t.Run("patch_tags", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{401, 403, 499, 500}, ParsePatchV1TagsResponse)
	})
	t.Run("xid_bursts", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500}, ParseGetV1XIDBurstsResponse)
	})
	t.Run("xid_burst_options", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 499, 500}, ParseGetV1XIDBurstOptionsResponse)
	})
	t.Run("xid_burst_detail", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 401, 403, 404, 499, 500}, ParseGetV1XIDBurstDetailResponse)
	})
	t.Run("commands", func(t *testing.T) {
		testRound2AlertParserMalformedStatuses(t, []int{200, 499, 500}, ParseGetV2CommandsResponse)
	})
}

func TestRound2RemainingModelEnumValidContracts(t *testing.T) {
	// Literals come from the corresponding schemas in api/openapi/openapi.yaml.
	t.Run("ErrorSeverity", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsErrorSeverity]{
			{ErrorSeverityFatal, "Fatal"}, {ErrorSeverityCritical, "Critical"}, {ErrorSeverityWarning, "Warning"},
		}, "fatal")
	})
	t.Run("FirmwareCheck", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsFirmwareCheck]{
			{FirmwareCheckPassed, "Passed"}, {FirmwareCheckFailed, "Failed"}, {FirmwareCheckUnknown, "Unknown"},
		}, "passed")
	})
	t.Run("IntegrityCheck", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsIntegrityCheck]{
			{IntegrityCheckFullAttestation, "Full Attestation"}, {IntegrityCheckPartialAttestation, "Partial Attestation"},
			{IntegrityCheckVerificationFailed, "Verification Failed"}, {IntegrityCheckPassed, "Verified"},
			{IntegrityCheckFailed, "Unverified"}, {IntegrityCheckDegraded, "Degraded"},
			{IntegrityCheckPending, "Pending"}, {IntegrityCheckUnsupported, "Unsupported"}, {IntegrityCheckUnknown, "Unknown"},
		}, "verified")
	})
	t.Run("VerificationCheck", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsVerificationCheck]{
			{VerificationCheckFullAttestation, "Full Attestation"}, {VerificationCheckPartialAttestation, "Partial Attestation"},
			{VerificationCheckVerificationFailed, "Verification Failed"}, {VerificationCheckPassed, "Verified"},
			{VerificationCheckFailed, "Unverified"}, {VerificationCheckDegraded, "Degraded"},
			{VerificationCheckPending, "Pending"}, {VerificationCheckUnsupported, "Unsupported"}, {VerificationCheckUnknown, "Unknown"},
		}, "verified")
	})
	t.Run("MetricUnit", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsMetricUnit]{
			{MetricUnitMegahertz, "MHz"}, {MetricUnitCount, "count"}, {MetricUnitDegreesCelsius, "°C"},
			{MetricUnitDegreesFahrenheit, "°F"}, {MetricUnitCelsius, "celsius"}, {MetricUnitWatts, "watts"},
			{MetricUnitBoolean, "boolean"}, {MetricUnitMegabyte, "MB"}, {MetricUnitRatio, "ratio"},
			{MetricUnitMegabytesPerSecond, "MB/s"}, {MetricUnitBitErrorRate, "ber"}, {MetricUnitNanoseconds, "nanoseconds"},
			{MetricUnitMilliseconds, "ms"}, {MetricUnitSeconds, "seconds"}, {MetricUnitSecond, "s"},
			{MetricUnitBytes, "bytes"}, {MetricUnitBytesPerSecond, "bytes/s"}, {MetricUnitMillijoules, "millijoules"},
			{MetricUnitLoad, "load"}, {MetricUnitCores, "cores"}, {MetricUnitPercent, "percent"},
			{MetricUnitRPM, "rpm"}, {MetricUnitVolts, "volts"},
		}, "mhz")
	})
	t.Run("NodeKind", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsNodeKind]{
			{NodeKindUnknown, "unknown"}, {NodeKindCompute, "compute"}, {NodeKindNvSwitch, "nvswitch"}, {NodeKindPowerShelf, "power_shelf"},
		}, "Compute")
	})
	t.Run("PredictionStatus", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsPredictedHealthRankingItemPredictionStatus]{
			{Ranked, "ranked"}, {Unranked, "unranked"},
		}, "Ranked")
	})
	t.Run("RankUnit", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsPredictedHealthRankingsResponseRankUnit]{
			{ModelsPredictedHealthRankingsResponseRankUnitNode, "node"},
			{ModelsPredictedHealthRankingsResponseRankUnitNvlinkDomain, "nvlinkDomain"},
			{ModelsPredictedHealthRankingsResponseRankUnitRack, "rack"},
		}, "Node")
	})
	t.Run("SecurityVulnerabilitySeverity", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsSecurityVulnerabilitySeverity]{
			{SecurityVulnerabilitySeverityLow, "LOW"}, {SecurityVulnerabilitySeverityMedium, "MEDIUM"},
			{SecurityVulnerabilitySeverityHigh, "HIGH"}, {SecurityVulnerabilitySeverityCritical, "CRITICAL"},
		}, "low")
	})
	t.Run("VulnerabilityComponent", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[ModelsVulnerabilityComponent]{
			{VulnerabilityComponentGPUDriver, "gpuDriver"}, {VulnerabilityComponentVBIOS, "vbios"},
		}, "GpuDriver")
	})
	t.Run("VerificationPolicyTier", func(t *testing.T) {
		for _, value := range []struct {
			constant ModelsVerificationPolicyTier
			literal  int
		}{
			{ModelsVerificationPolicyTierN1, 1}, {ModelsVerificationPolicyTierN2, 2}, {ModelsVerificationPolicyTierN3, 3},
		} {
			if int(value.constant) != value.literal {
				t.Errorf("tier constant = %d, want %d", value.constant, value.literal)
			}
			if !value.constant.Valid() {
				t.Errorf("tier constant %d is not valid", value.constant)
			}
			if !ModelsVerificationPolicyTier(value.literal).Valid() {
				t.Errorf("tier schema literal %d is not valid", value.literal)
			}
		}
		for _, invalid := range []ModelsVerificationPolicyTier{0, 4} {
			if invalid.Valid() {
				t.Errorf("invalid tier %d was accepted", invalid)
			}
		}
	})
	t.Run("VerificationPolicyVerdictTier", func(t *testing.T) {
		for _, value := range []struct {
			constant ModelsVerificationPolicyVerdictTier
			literal  int
		}{
			{ModelsVerificationPolicyVerdictTierN1, 1}, {ModelsVerificationPolicyVerdictTierN2, 2}, {ModelsVerificationPolicyVerdictTierN3, 3},
		} {
			if int(value.constant) != value.literal {
				t.Errorf("verdict tier constant = %d, want %d", value.constant, value.literal)
			}
			if !value.constant.Valid() {
				t.Errorf("verdict tier constant %d is not valid", value.constant)
			}
			if !ModelsVerificationPolicyVerdictTier(value.literal).Valid() {
				t.Errorf("verdict tier schema literal %d is not valid", value.literal)
			}
		}
		for _, invalid := range []ModelsVerificationPolicyVerdictTier{0, 4} {
			if invalid.Valid() {
				t.Errorf("invalid verdict tier %d was accepted", invalid)
			}
		}
	})
}

func TestRound2AlertComputezoneQueryEnumContracts(t *testing.T) {
	t.Run("GetV1AlertTimelineNodesParamsSortBy", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1AlertTimelineNodesParamsSortBy]{
			{GetV1AlertTimelineNodesParamsSortByHostname, "hostname"},
			{GetV1AlertTimelineNodesParamsSortByNodeName, "nodeName"},
			{GetV1AlertTimelineNodesParamsSortByBmcMac, "bmcMac"},
			{GetV1AlertTimelineNodesParamsSortByNodeUUID, "nodeUUID"},
			{GetV1AlertTimelineNodesParamsSortByAlert, "alert"},
			{GetV1AlertTimelineNodesParamsSortByGpuType, "gpuType"},
			{GetV1AlertTimelineNodesParamsSortByNodeGroup, "nodeGroup"},
			{GetV1AlertTimelineNodesParamsSortByComputeZone, "computeZone"},
			{GetV1AlertTimelineNodesParamsSortByLastUpdate, "lastUpdate"},
		}, "Hostname")
	})
	t.Run("GetV1AlertTimelineNodesParamsOrder", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1AlertTimelineNodesParamsOrder]{
			{GetV1AlertTimelineNodesParamsOrderAsc, "asc"},
			{GetV1AlertTimelineNodesParamsOrderDesc, "desc"},
		}, "Asc")
	})
	t.Run("GetV1AlertTimelineNodesParamsAlertStates", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1AlertTimelineNodesParamsAlertStates]{
			{GetV1AlertTimelineNodesParamsAlertStatesCritical, "Critical"},
			{GetV1AlertTimelineNodesParamsAlertStatesWarning, "Warning"},
			{GetV1AlertTimelineNodesParamsAlertStatesResolved, "Resolved"},
		}, "critical")
	})
	t.Run("GetV1AlertTimelineNodesParamsDataSources", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1AlertTimelineNodesParamsDataSources]{
			{GetV1AlertTimelineNodesParamsDataSourcesInband, "inband"},
			{GetV1AlertTimelineNodesParamsDataSourcesOob, "oob"},
		}, "Inband")
	})
	t.Run("GetV1AlertTimelineNodesNodeUuidAlertsParamsSortBy", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1AlertTimelineNodesNodeUuidAlertsParamsSortBy]{
			{GetV1AlertTimelineNodesNodeUuidAlertsParamsSortByStartTime, "startTime"},
			{GetV1AlertTimelineNodesNodeUuidAlertsParamsSortByLastUpdate, "lastUpdate"},
			{GetV1AlertTimelineNodesNodeUuidAlertsParamsSortByDataSource, "dataSource"},
		}, "StartTime")
	})
	t.Run("GetV1AlertTimelineNodesNodeUuidAlertsParamsOrder", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1AlertTimelineNodesNodeUuidAlertsParamsOrder]{
			{GetV1AlertTimelineNodesNodeUuidAlertsParamsOrderAsc, "asc"},
			{GetV1AlertTimelineNodesNodeUuidAlertsParamsOrderDesc, "desc"},
		}, "Asc")
	})
	t.Run("GetV1AlertTimelineNodesNodeUuidAlertsParamsAlertStates", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1AlertTimelineNodesNodeUuidAlertsParamsAlertStates]{
			{GetV1AlertTimelineNodesNodeUuidAlertsParamsAlertStatesCritical, "Critical"},
			{GetV1AlertTimelineNodesNodeUuidAlertsParamsAlertStatesWarning, "Warning"},
			{GetV1AlertTimelineNodesNodeUuidAlertsParamsAlertStatesResolved, "Resolved"},
		}, "critical")
	})
	t.Run("GetV1AlertTimelineNodesNodeUuidAlertsParamsDataSources", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1AlertTimelineNodesNodeUuidAlertsParamsDataSources]{
			{GetV1AlertTimelineNodesNodeUuidAlertsParamsDataSourcesInband, "inband"},
			{GetV1AlertTimelineNodesNodeUuidAlertsParamsDataSourcesOob, "oob"},
		}, "Inband")
	})
	t.Run("GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidParamsOrder", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidParamsOrder]{
			{GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidParamsOrderAsc, "asc"},
			{GetV1AlertTimelineNodesNodeUuidAlertsAlertUuidParamsOrderDesc, "desc"},
		}, "Asc")
	})
	t.Run("GetV1ComputezonesParamsSortBy", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ComputezonesParamsSortBy]{
			{GetV1ComputezonesParamsSortByCreatedAt, "createdAt"},
			{GetV1ComputezonesParamsSortByName, "name"},
		}, "CreatedAt")
	})
	t.Run("GetV1ComputezonesParamsOrder", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ComputezonesParamsOrder]{
			{GetV1ComputezonesParamsOrderAsc, "asc"},
			{GetV1ComputezonesParamsOrderDesc, "desc"},
		}, "Asc")
	})
}

func TestRound2EventsQueryEnumContracts(t *testing.T) {
	t.Run("GetV1EventsParamsTimeMode", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1EventsParamsTimeMode]{
			{GetV1EventsParamsTimeModeAbsolute, "absolute"},
			{GetV1EventsParamsTimeModeRelative, "relative"},
		}, "Absolute")
	})
	t.Run("GetV1EventsParamsDataSources", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1EventsParamsDataSources]{
			{GetV1EventsParamsDataSourcesInband, "inband"},
			{GetV1EventsParamsDataSourcesOob, "oob"},
		}, "Inband")
	})
	t.Run("GetV1EventsParamsSortBy", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1EventsParamsSortBy]{
			{GetV1EventsParamsSortByTimestamp, "timestamp"},
			{GetV1EventsParamsSortByEventName, "eventName"},
			{GetV1EventsParamsSortBySeverity, "severity"},
			{GetV1EventsParamsSortByComponent, "component"},
			{GetV1EventsParamsSortByGpuType, "gpuType"},
			{GetV1EventsParamsSortByHostname, "hostname"},
			{GetV1EventsParamsSortByNodeName, "nodeName"},
			{GetV1EventsParamsSortByBmcMac, "bmcMac"},
			{GetV1EventsParamsSortByNodeUUID, "nodeUUID"},
			{GetV1EventsParamsSortByNodeGroup, "nodeGroup"},
			{GetV1EventsParamsSortByComputeZone, "computeZone"},
		}, "Timestamp")
	})
	t.Run("GetV1EventsParamsSortOrder", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1EventsParamsSortOrder]{
			{GetV1EventsParamsSortOrderAsc, "asc"},
			{GetV1EventsParamsSortOrderDesc, "desc"},
		}, "Asc")
	})
	t.Run("GetV1EventsBucketsParamsTimeMode", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1EventsBucketsParamsTimeMode]{
			{GetV1EventsBucketsParamsTimeModeAbsolute, "absolute"},
			{GetV1EventsBucketsParamsTimeModeRelative, "relative"},
		}, "Absolute")
	})
	t.Run("GetV1EventsBucketsParamsDataSources", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1EventsBucketsParamsDataSources]{
			{GetV1EventsBucketsParamsDataSourcesInband, "inband"},
			{GetV1EventsBucketsParamsDataSourcesOob, "oob"},
		}, "Inband")
	})
}

func TestRound2MetricsAndNodeQueryEnumContracts(t *testing.T) {
	t.Run("GetV1MetricsParamsDataSource", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1MetricsParamsDataSource]{
			{GetV1MetricsParamsDataSourceAll, "all"},
			{GetV1MetricsParamsDataSourceInband, "inband"},
			{GetV1MetricsParamsDataSourceOob, "oob"},
		}, "ALL")
	})
	t.Run("GetV1MetricsParamsEntityLevel", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1MetricsParamsEntityLevel]{
			{GetV1MetricsParamsEntityLevelComputezone, "computezone"},
			{GetV1MetricsParamsEntityLevelNode, "node"},
			{GetV1MetricsParamsEntityLevelNodegroup, "nodegroup"},
		}, "COMPUTEZONE")
	})
	t.Run("GetV1MetricsQueryRangeParamsTimeMode", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1MetricsQueryRangeParamsTimeMode]{
			{GetV1MetricsQueryRangeParamsTimeModeAbsolute, "absolute"},
			{GetV1MetricsQueryRangeParamsTimeModeRelative, "relative"},
		}, "ABSOLUTE")
	})
	t.Run("GetV1MetricsQueryRangeParamsEntityLevel", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1MetricsQueryRangeParamsEntityLevel]{
			{GetV1MetricsQueryRangeParamsEntityLevelNode, "node"},
		}, "NODE")
	})
	t.Run("GetV1NodegroupsParamsHealthStatuses", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodegroupsParamsHealthStatuses]{
			{GetV1NodegroupsParamsHealthStatusesHealthy, "Healthy"},
			{GetV1NodegroupsParamsHealthStatusesDegraded, "Degraded"},
			{GetV1NodegroupsParamsHealthStatusesUnhealthy, "Unhealthy"},
			{GetV1NodegroupsParamsHealthStatusesUnknown, "Unknown"},
		}, "hEALTHY")
	})
	t.Run("GetV1NodesParamsHealthStatuses", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodesParamsHealthStatuses]{
			{GetV1NodesParamsHealthStatusesHealthy, "Healthy"},
			{GetV1NodesParamsHealthStatusesDegraded, "Degraded"},
			{GetV1NodesParamsHealthStatusesUnhealthy, "Unhealthy"},
			{GetV1NodesParamsHealthStatusesUnknown, "Unknown"},
		}, "hEALTHY")
	})
	t.Run("GetV1NodegroupsParamsOrder", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodegroupsParamsOrder]{
			{GetV1NodegroupsParamsOrderAsc, "asc"},
			{GetV1NodegroupsParamsOrderDesc, "desc"},
		}, "ASC")
	})
	t.Run("DeleteV1NodesParamsDataSource", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[DeleteV1NodesParamsDataSource]{
			{DeleteV1NodesParamsDataSourceInband, "inband"},
			{DeleteV1NodesParamsDataSourceOob, "oob"},
		}, "INBAND")
	})
	t.Run("DeleteV1NodesParamsAgentType", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[DeleteV1NodesParamsAgentType]{
			{DeleteV1NodesParamsAgentTypeInband, "inband"},
			{DeleteV1NodesParamsAgentTypeOob, "oob"},
		}, "INBAND")
	})
	t.Run("GetV1NodesParamsDataSource", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodesParamsDataSource]{
			{GetV1NodesParamsDataSourceInband, "inband"},
			{GetV1NodesParamsDataSourceOob, "oob"},
		}, "INBAND")
	})
	t.Run("GetV1NodesParamsAgentType", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodesParamsAgentType]{
			{GetV1NodesParamsAgentTypeInband, "inband"},
			{GetV1NodesParamsAgentTypeOob, "oob"},
		}, "INBAND")
	})
	t.Run("GetV1NodesOptionsParamsDataSource", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodesOptionsParamsDataSource]{
			{GetV1NodesOptionsParamsDataSourceInband, "inband"},
			{GetV1NodesOptionsParamsDataSourceOob, "oob"},
		}, "INBAND")
	})
	t.Run("GetV1NodesOptionsParamsAgentType", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodesOptionsParamsAgentType]{
			{GetV1NodesOptionsParamsAgentTypeInband, "inband"},
			{GetV1NodesOptionsParamsAgentTypeOob, "oob"},
		}, "INBAND")
	})
	t.Run("DeleteV1NodesNodeUuidParamsDataSource", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[DeleteV1NodesNodeUuidParamsDataSource]{
			{DeleteV1NodesNodeUuidParamsDataSourceInband, "inband"},
			{DeleteV1NodesNodeUuidParamsDataSourceOob, "oob"},
		}, "INBAND")
	})
	t.Run("DeleteV1NodesNodeUuidParamsAgentType", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[DeleteV1NodesNodeUuidParamsAgentType]{
			{DeleteV1NodesNodeUuidParamsAgentTypeInband, "inband"},
			{DeleteV1NodesNodeUuidParamsAgentTypeOob, "oob"},
		}, "INBAND")
	})
	t.Run("GetV1NodesNodeUuidParamsDataSource", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodesNodeUuidParamsDataSource]{
			{GetV1NodesNodeUuidParamsDataSourceInband, "inband"},
			{GetV1NodesNodeUuidParamsDataSourceOob, "oob"},
		}, "INBAND")
	})
	t.Run("GetV1NodesNodeUuidParamsAgentType", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodesNodeUuidParamsAgentType]{
			{GetV1NodesNodeUuidParamsAgentTypeInband, "inband"},
			{GetV1NodesNodeUuidParamsAgentTypeOob, "oob"},
		}, "INBAND")
	})
	t.Run("GetV1NodesParamsSortBy", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodesParamsSortBy]{
			{GetV1NodesParamsSortByHostname, "hostname"},
			{GetV1NodesParamsSortByNodeName, "nodeName"},
			{GetV1NodesParamsSortByNodeUUID, "nodeUUID"},
			{GetV1NodesParamsSortByHealthStatus, "healthStatus"},
			{GetV1NodesParamsSortByNodegroup, "nodegroup"},
			{GetV1NodesParamsSortByComputezone, "computezone"},
			{GetV1NodesParamsSortByGpuType, "gpuType"},
			{GetV1NodesParamsSortByGpuCount, "gpuCount"},
			{GetV1NodesParamsSortByVerificationCheck, "verificationCheck"},
			{GetV1NodesParamsSortByIntegrityCheck, "integrityCheck"},
			{GetV1NodesParamsSortByAgentStatus, "agentStatus"},
			{GetV1NodesParamsSortByAgentVersion, "agentVersion"},
			{GetV1NodesParamsSortByKernelVersion, "kernelVersion"},
			{GetV1NodesParamsSortByGpuDriverVersion, "gpuDriverVersion"},
			{GetV1NodesParamsSortByGpuFirmwareVersions, "gpuFirmwareVersions"},
			{GetV1NodesParamsSortByBmcMac, "bmcMac"},
		}, "HOSTNAME")
	})
	t.Run("GetV1NodesHistoryParamsActorType", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodesHistoryParamsActorType]{
			{GetV1NodesHistoryParamsActorTypeSystem, "system"},
			{GetV1NodesHistoryParamsActorTypeUser, "user"},
			{GetV1NodesHistoryParamsActorTypeAll, "all"},
		}, "SYSTEM")
	})
	t.Run("GetV1NodesOptionsParamsView", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1NodesOptionsParamsView]{
			{GetV1NodesOptionsParamsViewDetail, "detail"},
			{GetV1NodesOptionsParamsViewBasic, "basic"},
		}, "DETAIL")
	})
}

func TestRound2RankingsReportStatsQueryEnumContracts(t *testing.T) {
	t.Run("GetV1PredictedHealthRankingsParamsRankUnit", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1PredictedHealthRankingsParamsRankUnit]{
			{GetV1PredictedHealthRankingsParamsRankUnitNode, "node"},
			{GetV1PredictedHealthRankingsParamsRankUnitNvlinkDomain, "nvlinkDomain"},
			{GetV1PredictedHealthRankingsParamsRankUnitRack, "rack"},
		}, "NODE")
	})
	t.Run("GetV1PredictedHealthRankingsParamsAggregation", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1PredictedHealthRankingsParamsAggregation]{
			{Max, "max"},
			{NthMax, "nthMax"},
			{Mean, "mean"},
			{CountAbove, "countAbove"},
		}, "MAX")
	})
	t.Run("GetV1PredictedHealthRankingsParamsMissingScorePolicy", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1PredictedHealthRankingsParamsMissingScorePolicy]{
			{Ignore, "ignore"},
			{RequireAll, "requireAll"},
		}, "IGNORE")
	})
	t.Run("GetV1PredictedHealthRankingsParamsTagMatch", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1PredictedHealthRankingsParamsTagMatch]{
			{GetV1PredictedHealthRankingsParamsTagMatchAny, "any"},
			{GetV1PredictedHealthRankingsParamsTagMatchAll, "all"},
		}, "ANY")
	})
	t.Run("GetV1PredictedHealthRankingsParamsHealthStatuses", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1PredictedHealthRankingsParamsHealthStatuses]{
			{GetV1PredictedHealthRankingsParamsHealthStatusesHealthy, "Healthy"},
			{GetV1PredictedHealthRankingsParamsHealthStatusesDegraded, "Degraded"},
			{GetV1PredictedHealthRankingsParamsHealthStatusesUnhealthy, "Unhealthy"},
			{GetV1PredictedHealthRankingsParamsHealthStatusesUnknown, "Unknown"},
		}, "healthy")
	})
	t.Run("GetV1PredictedHealthRankingsParamsOrder", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1PredictedHealthRankingsParamsOrder]{
			{BestFirst, "bestFirst"},
			{WorstFirst, "worstFirst"},
		}, "BESTFIRST")
	})
	t.Run("GetV1ReportsErrorParamsFormat", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ReportsErrorParamsFormat]{
			{GetV1ReportsErrorParamsFormatJson, "json"},
			{GetV1ReportsErrorParamsFormatCsv, "csv"},
		}, "CSV")
	})
	t.Run("GetV1ReportsErrorParamsDataSource", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ReportsErrorParamsDataSource]{
			{GetV1ReportsErrorParamsDataSourceInband, "inband"},
			{GetV1ReportsErrorParamsDataSourceOob, "oob"},
		}, "INBAND")
	})
	t.Run("GetV1ReportsInventoryParamsFormat", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ReportsInventoryParamsFormat]{
			{GetV1ReportsInventoryParamsFormatJson, "json"},
			{GetV1ReportsInventoryParamsFormatCsv, "csv"},
		}, "CSV")
	})
	t.Run("GetV1ReportsInventoryParamsDataSource", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ReportsInventoryParamsDataSource]{
			{GetV1ReportsInventoryParamsDataSourceInband, "inband"},
			{GetV1ReportsInventoryParamsDataSourceOob, "oob"},
		}, "INBAND")
	})
	t.Run("GetV1ReportsInventoryParamsAgentType", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ReportsInventoryParamsAgentType]{
			{GetV1ReportsInventoryParamsAgentTypeInband, "inband"},
			{GetV1ReportsInventoryParamsAgentTypeOob, "oob"},
		}, "INBAND")
	})
	t.Run("GetV1ReportsInventoryParamsOrder", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ReportsInventoryParamsOrder]{
			{GetV1ReportsInventoryParamsOrderAsc, "asc"},
			{GetV1ReportsInventoryParamsOrderDesc, "desc"},
		}, "ASC")
	})
	t.Run("GetV1ReportsSecurityVulnerabilitiesParamsOrder", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ReportsSecurityVulnerabilitiesParamsOrder]{
			{GetV1ReportsSecurityVulnerabilitiesParamsOrderAsc, "asc"},
			{GetV1ReportsSecurityVulnerabilitiesParamsOrderDesc, "desc"},
		}, "ASC")
	})
	t.Run("GetV1ReportsSecurityVulnerabilitiesParamsFormat", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ReportsSecurityVulnerabilitiesParamsFormat]{
			{GetV1ReportsSecurityVulnerabilitiesParamsFormatJson, "json"},
			{GetV1ReportsSecurityVulnerabilitiesParamsFormatCsv, "csv"},
		}, "CSV")
	})
	t.Run("GetV1ReportsSecurityVulnerabilityNodesParamsFormat", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1ReportsSecurityVulnerabilityNodesParamsFormat]{
			{GetV1ReportsSecurityVulnerabilityNodesParamsFormatCsv, "csv"},
		}, "CSV")
	})
	t.Run("GetV1StatsParamsName", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1StatsParamsName]{
			{GetV1StatsParamsNameAgentStatus, "agentStatus"},
		}, "AGENTSTATUS")
	})
	t.Run("GetV1StatsParamsDataSource", func(t *testing.T) {
		testRound2EnumContract(t, []round2EnumValue[GetV1StatsParamsDataSource]{
			{GetV1StatsParamsDataSourceInband, "inband"},
			{GetV1StatsParamsDataSourceOob, "oob"},
		}, "INBAND")
	})
}

func TestRound2RemainingReadRequestFailures(t *testing.T) {
	sentinel := errors.New("editor rejected request")
	tests := []struct {
		name string
		path string
		call func(*ClientWithResponses, RequestEditorFn) (bool, error)
	}{
		{
			name: "signing key", path: "/.well-known/signing-key.pub",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetWellKnownSigningKeyPubWithResponse(context.Background(), editor)
				return response == nil, err
			},
		},
		{
			name: "alert mute rules", path: "/v1/alert_mute_rules",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1AlertMuteRulesWithResponse(context.Background(), editor)
				return response == nil, err
			},
		},
		{
			name: "event options", path: "/v1/events/options",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1EventsOptionsWithResponse(context.Background(), editor)
				return response == nil, err
			},
		},
		{
			name: "info", path: "/v1/info",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1InfoWithResponse(context.Background(), editor)
				return response == nil, err
			},
		},
		{
			name: "metrics query range", path: "/v1/metrics/query_range",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				mode, window := GetV1MetricsQueryRangeParamsTimeModeRelative, "1h30m"
				response, err := c.GetV1MetricsQueryRangeWithResponse(context.Background(), &GetV1MetricsQueryRangeParams{TimeMode: &mode, Window: &window, Step: "5m", Component: "gpu", Metric: "gpu_temperature", EntityLevel: GetV1MetricsQueryRangeParamsEntityLevelNode, EntityID: "node-1"}, editor)
				return response == nil, err
			},
		},
		{
			name: "node group memberships", path: "/v1/node_group_memberships",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NodeGroupMembershipsWithResponse(context.Background(), &GetV1NodeGroupMembershipsParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "node removal policy", path: "/v1/node_removal_policy",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NodeRemovalPolicyWithResponse(context.Background(), editor)
				return response == nil, err
			},
		},
		{
			name: "nodegroup options", path: "/v1/nodegroups/options",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NodegroupsOptionsWithResponse(context.Background(), editor)
				return response == nil, err
			},
		},
		{
			name: "node history", path: "/v1/nodes/history",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NodesHistoryWithResponse(context.Background(), &GetV1NodesHistoryParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "node options", path: "/v1/nodes/options",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NodesOptionsWithResponse(context.Background(), &GetV1NodesOptionsParams{}, editor)
				return response == nil, err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			transportCalls := 0
			doer := authStatusTestDoer(func(_ *http.Request) (*http.Response, error) {
				transportCalls++
				return nil, errors.New("unexpected transport call")
			})
			client, err := NewClientWithResponses("http://example.test", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			editorCalls := 0
			responseNil, err := tc.call(client, func(_ context.Context, req *http.Request) error {
				editorCalls++
				if req.Method != http.MethodGet || req.URL.Path != tc.path {
					t.Errorf("request = %s %s, want GET %s", req.Method, req.URL.Path, tc.path)
				}
				return sentinel
			})
			if !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
			if !responseNil {
				t.Error("response is non-nil, want nil")
			}
			if editorCalls != 1 || transportCalls != 0 {
				t.Errorf("editor calls = %d, transport calls = %d; want 1 and 0", editorCalls, transportCalls)
			}

			malformedClient, err := NewClientWithResponses("http://[::1", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new malformed-server client: %v", err)
			}
			editorCalls, transportCalls = 0, 0
			responseNil, err = tc.call(malformedClient, func(_ context.Context, _ *http.Request) error {
				editorCalls++
				return nil
			})
			if err == nil {
				t.Error("malformed server: expected URL construction error")
			}
			if !responseNil {
				t.Error("malformed server: response is non-nil, want nil")
			}
			if editorCalls != 0 || transportCalls != 0 {
				t.Errorf("malformed server: editor calls = %d, transport calls = %d; want 0 and 0", editorCalls, transportCalls)
			}
		})
	}
}

func TestRound2RemainingClientConstructionFailures(t *testing.T) {
	const id = "11111111-1111-4111-8111-111111111111"
	sentinel := errors.New("editor rejected request")
	tests := []struct {
		name, method, path string
		call               func(*ClientWithResponses, RequestEditorFn) (bool, error)
	}{
		{
			name: "delete mute rules",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.DeleteV1AlertMuteRulesWithBodyWithResponse(context.Background(), "application/json", strings.NewReader("{}"), editor)
				return response == nil, err
			},
		},
		{
			name: "put mute rules",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.PutV1AlertMuteRulesWithBodyWithResponse(context.Background(), "application/json", strings.NewReader("{}"), editor)
				return response == nil, err
			},
		},
		{
			name: "put compute zones",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.PutV1ComputezonesWithBodyWithResponse(context.Background(), "application/json", strings.NewReader("{}"), editor)
				return response == nil, err
			},
		},
		{
			name: "put node removal policy",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.PutV1NodeRemovalPolicyWithBodyWithResponse(context.Background(), "application/json", strings.NewReader("{}"), editor)
				return response == nil, err
			},
		},
		{
			name: "delete nodes",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.DeleteV1NodesWithBodyWithResponse(context.Background(), &DeleteV1NodesParams{}, "application/json", strings.NewReader("{}"), editor)
				return response == nil, err
			},
		},
		{
			name: "put node tags",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.PutV1NodesNodeUuidTagsWithBodyWithResponse(context.Background(), id, "application/json", strings.NewReader("{}"), editor)
				return response == nil, err
			},
		},
		{
			name: "post notification preferences",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.PostV1NotificationPreferencesWithBodyWithResponse(context.Background(), "application/json", strings.NewReader("{}"), editor)
				return response == nil, err
			},
		},
		{
			name: "put notification preference",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.PutV1NotificationPreferenceByIDWithBodyWithResponse(context.Background(), id, "application/json", strings.NewReader("{}"), editor)
				return response == nil, err
			},
		},
		{
			name: "put retention policy",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.PutV1RetentionPolicyWithBodyWithResponse(context.Background(), "application/json", strings.NewReader("{}"), editor)
				return response == nil, err
			},
		},
		{
			name: "patch tags",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.PatchV1TagsWithBodyWithResponse(context.Background(), "application/json", strings.NewReader("{}"), editor)
				return response == nil, err
			},
		},
		{
			name: "auth status",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1AuthStatusWithResponse(context.Background(), editor)
				return response == nil, err
			},
		},
		{
			name: "delete node", method: http.MethodDelete, path: "/v1/nodes/" + id,
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.DeleteV1NodesNodeUuidWithResponse(context.Background(), id, &DeleteV1NodesNodeUuidParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "node detail", method: http.MethodGet, path: "/v1/nodes/" + id,
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NodesNodeUuidWithResponse(context.Background(), id, &GetV1NodesNodeUuidParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "node health history", method: http.MethodGet, path: "/v1/nodes/" + id + "/health_history",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1NodesNodeUuidHealthHistoryWithResponse(context.Background(), id, &GetV1NodesNodeUuidHealthHistoryParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "retention policy", method: http.MethodGet, path: "/v1/retention_policy",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1RetentionPolicyWithResponse(context.Background(), editor)
				return response == nil, err
			},
		},
		{
			name: "xid bursts", method: http.MethodGet, path: "/v1/xid/bursts",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1XIDBurstsWithResponse(context.Background(), &GetV1XIDBurstsParams{}, editor)
				return response == nil, err
			},
		},
		{
			name: "xid burst options", method: http.MethodGet, path: "/v1/xid/bursts/options",
			call: func(c *ClientWithResponses, editor RequestEditorFn) (bool, error) {
				response, err := c.GetV1XIDBurstOptionsWithResponse(context.Background(), &GetV1XIDBurstOptionsParams{}, editor)
				return response == nil, err
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			transportCalls := 0
			doer := authStatusTestDoer(func(_ *http.Request) (*http.Response, error) {
				transportCalls++
				return nil, errors.New("unexpected transport call")
			})
			client, err := NewClientWithResponses("http://[::1", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new malformed-server client: %v", err)
			}
			editorCalls := 0
			responseNil, err := tc.call(client, func(_ context.Context, _ *http.Request) error {
				editorCalls++
				return nil
			})
			if err == nil {
				t.Error("malformed server: expected URL construction error")
			}
			if !responseNil {
				t.Error("malformed server: response is non-nil, want nil")
			}
			if editorCalls != 0 || transportCalls != 0 {
				t.Errorf("malformed server: editor calls = %d, transport calls = %d; want 0 and 0", editorCalls, transportCalls)
			}

			if tc.method == "" {
				return
			}
			client, err = NewClientWithResponses("http://example.test", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("new client: %v", err)
			}
			editorCalls, transportCalls = 0, 0
			responseNil, err = tc.call(client, func(_ context.Context, req *http.Request) error {
				editorCalls++
				if req.Method != tc.method || req.URL.Path != tc.path {
					t.Errorf("request = %s %s, want %s %s", req.Method, req.URL.Path, tc.method, tc.path)
				}
				return sentinel
			})
			if !errors.Is(err, sentinel) {
				t.Errorf("error = %v, want sentinel", err)
			}
			if !responseNil {
				t.Error("response is non-nil, want nil")
			}
			if editorCalls != 1 || transportCalls != 0 {
				t.Errorf("editor calls = %d, transport calls = %d; want 1 and 0", editorCalls, transportCalls)
			}
		})
	}
}

func TestRound2AlertTimelineCountOnlyRequest(t *testing.T) {
	active, yes, no := true, true, false
	for _, tc := range []struct {
		name      string
		countOnly *bool
		query     url.Values
	}{
		{name: "true", countOnly: &yes, query: url.Values{"active": {"true"}, "countOnly": {"true"}}},
		{name: "false", countOnly: &no, query: url.Values{"active": {"true"}, "countOnly": {"false"}}},
		{name: "nil", query: url.Values{"active": {"true"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req, err := NewGetV1AlertTimelineNodesRequest("http://example.test", &GetV1AlertTimelineNodesParams{Active: &active, CountOnly: tc.countOnly})
			if err != nil {
				t.Fatalf("construct request: %v", err)
			}
			if req.Method != http.MethodGet || req.URL.Path != "/v1/alert_timeline/nodes" {
				t.Errorf("request = %s %s, want GET /v1/alert_timeline/nodes", req.Method, req.URL.Path)
			}
			if got := req.URL.Query(); !reflect.DeepEqual(got, tc.query) {
				t.Errorf("query = %v, want %v", got, tc.query)
			}
		})
	}
}

func TestRound2V2MetricsRequestFailures(t *testing.T) {
	params := &GetV2MetricsParams{GroupBy: GetV2MetricsParamsGroupByNode, MetricNames: []string{"gpu_util"}, Step: "5m"}
	transportCalls := 0
	doer := authStatusTestDoer(func(_ *http.Request) (*http.Response, error) {
		transportCalls++
		return nil, errors.New("unexpected transport call")
	})
	client, err := NewClientWithResponses("http://[::1", WithHTTPClient(doer))
	if err != nil {
		t.Fatalf("new malformed-server client: %v", err)
	}
	editorCalls := 0
	response, err := client.GetV2MetricsWithResponse(context.Background(), params, func(_ context.Context, _ *http.Request) error {
		editorCalls++
		return nil
	})
	if err == nil || response != nil {
		t.Errorf("malformed server: response = %v, error = %v; want nil response and construction error", response, err)
	}
	if editorCalls != 0 || transportCalls != 0 {
		t.Errorf("malformed server: editor calls = %d, transport calls = %d; want 0 and 0", editorCalls, transportCalls)
	}

	client, err = NewClientWithResponses("http://example.test", WithHTTPClient(doer))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	sentinel := errors.New("editor rejected request")
	response, err = client.GetV2MetricsWithResponse(context.Background(), params, func(_ context.Context, req *http.Request) error {
		editorCalls++
		if req.Method != http.MethodGet || req.URL.Path != "/v2/metrics" {
			t.Errorf("request = %s %s, want GET /v2/metrics", req.Method, req.URL.Path)
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) || response != nil {
		t.Errorf("editor failure: response = %v, error = %v; want nil response and sentinel", response, err)
	}
	if editorCalls != 1 || transportCalls != 0 {
		t.Errorf("editor failure: editor calls = %d, transport calls = %d; want 1 and 0", editorCalls, transportCalls)
	}
}

func TestRound2GeneratedIOAndEditorFailurePropagation(t *testing.T) {
	transportCalls := 0
	doer := authStatusTestDoer(func(_ *http.Request) (*http.Response, error) {
		transportCalls++
		return nil, errors.New("unexpected transport call")
	})
	client, err := NewClientWithResponses("http://example.test", WithHTTPClient(doer))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	editorSentinel := errors.New("editor rejected compute zones request")
	editorCalls := 0
	response, err := client.PutV1ComputezonesWithBodyWithResponse(context.Background(), "application/json", strings.NewReader(`{}`), func(_ context.Context, req *http.Request) error {
		editorCalls++
		if req.Method != http.MethodPut || req.URL.Path != "/v1/computezones" {
			t.Errorf("request = %s %s, want PUT /v1/computezones", req.Method, req.URL.Path)
		}
		if got := req.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", got)
		}
		return editorSentinel
	})
	if response != nil || !errors.Is(err, editorSentinel) {
		t.Errorf("editor failure: response = %v, error = %v; want nil response and sentinel", response, err)
	}
	if editorCalls != 1 || transportCalls != 0 {
		t.Errorf("editor failure: editor calls = %d, transport calls = %d; want 1 and 0", editorCalls, transportCalls)
	}

	readSentinel := errors.New("signing key body read failed")
	reader := &round2AlertTimelineErrorReader{err: readSentinel}
	parsed, err := ParseGetWellKnownSigningKeyPubResponse(&http.Response{Body: reader})
	if parsed != nil || !errors.Is(err, readSentinel) {
		t.Errorf("body read failure: response = %v, error = %v; want nil response and sentinel", parsed, err)
	}
	if !reader.closed {
		t.Error("body read failure: response body was not closed")
	}
}

func TestRound2PredictedHealthRankingQueryContracts(t *testing.T) {
	domainUnit := GetV1PredictedHealthRankingsParamsRankUnitNvlinkDomain
	rackUnit := GetV1PredictedHealthRankingsParamsRankUnitRack
	nodeUnit := GetV1PredictedHealthRankingsParamsRankUnitNode
	nthMax := NthMax
	countAbove := CountAbove
	requireAll := RequireAll
	aggregationN := 2
	threshold := float32(0.5)
	maxRank := 5
	nodeGroups := []string{"group-1", "group-2"}
	zones := []string{"zone-1"}
	domains := []string{"domain-1"}
	excludedNodes := []string{"node-3"}
	returnDomains := []string{"domain-1"}
	chassis := []string{"rack-a", "rack-b"}
	returnChassis := []string{"rack-a"}
	deciles := []int{1, 3}
	returnNodes := []string{"node-1", "node-2"}
	returnTags := []string{"team:a", "env:prod"}
	healthStatuses := []GetV1PredictedHealthRankingsParamsHealthStatuses{
		GetV1PredictedHealthRankingsParamsHealthStatusesHealthy,
		GetV1PredictedHealthRankingsParamsHealthStatusesDegraded,
	}

	cases := []struct {
		name   string
		params GetV1PredictedHealthRankingsParams
		query  url.Values
	}{
		{
			name: "domain nth max",
			params: GetV1PredictedHealthRankingsParams{
				RankUnit: &domainUnit, Aggregation: &nthMax, AggregationN: &aggregationN,
				MissingScorePolicy: &requireAll, NodeGroupIds: &nodeGroups, ComputeZoneIds: &zones,
				NvlinkDomainIds: &domains, ExcludeNodeUUIDs: &excludedNodes, ReturnNvlinkDomainIds: &returnDomains,
			},
			query: url.Values{
				"rankUnit": {"nvlinkDomain"}, "aggregation": {"nthMax"}, "aggregationN": {"2"},
				"missingScorePolicy": {"requireAll"}, "nodeGroupIds": {"group-1", "group-2"},
				"computeZoneIds": {"zone-1"}, "nvlinkDomainIds": {"domain-1"},
				"excludeNodeUUIDs": {"node-3"}, "returnNvlinkDomainIds": {"domain-1"},
			},
		},
		{
			name: "rack count above",
			params: GetV1PredictedHealthRankingsParams{
				RankUnit: &rackUnit, Aggregation: &countAbove, Threshold: &threshold,
				ChassisSNs: &chassis, ReturnChassisSNs: &returnChassis, MaxRank: &maxRank, Deciles: &deciles,
			},
			query: url.Values{
				"rankUnit": {"rack"}, "aggregation": {"countAbove"}, "threshold": {"0.5"},
				"chassisSNs": {"rack-a", "rack-b"}, "returnChassisSNs": {"rack-a"},
				"maxRank": {"5"}, "deciles": {"1", "3"},
			},
		},
		{
			name: "node return filters",
			params: GetV1PredictedHealthRankingsParams{
				RankUnit: &nodeUnit, ReturnNodeUUIDs: &returnNodes, ReturnTags: &returnTags, HealthStatuses: &healthStatuses,
			},
			query: url.Values{
				"rankUnit": {"node"}, "returnNodeUUIDs": {"node-1", "node-2"},
				"returnTags": {"team:a", "env:prod"}, "healthStatuses": {"Healthy", "Degraded"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := NewGetV1PredictedHealthRankingsRequest("https://example.test/", &tc.params)
			if err != nil {
				t.Fatalf("construct request: %v", err)
			}
			if req == nil {
				t.Fatal("construct request returned nil request")
			}
			if req.Method != http.MethodGet {
				t.Errorf("method = %q, want GET", req.Method)
			}
			if req.URL.Path != "/v1/predicted_health_rankings" {
				t.Errorf("path = %q, want /v1/predicted_health_rankings", req.URL.Path)
			}
			if got := req.URL.Query(); !reflect.DeepEqual(got, tc.query) {
				t.Errorf("query = %v, want %v", got, tc.query)
			}
		})
	}
}

func TestRound2RemainingQueryFilterContracts(t *testing.T) {
	const server = "https://example.test/"
	yes, no := true, false
	zone := "zone-legacy"
	nodeAgent := GetV1NodesParamsAgentTypeOob
	inventoryAgent := GetV1ReportsInventoryParamsAgentTypeOob
	checks := []ModelsIntegrityCheck{IntegrityCheckPassed, IntegrityCheckFailed}
	groups := []string{"group-1", "group-2"}
	gpuTypes := []string{"H100", "A100"}

	cases := []struct {
		name  string
		path  string
		query url.Values
		call  func() (*http.Request, error)
	}{
		{
			name: "metrics psirt true", path: "/v1/metrics", query: url.Values{"psirtComponent": {"true"}},
			call: func() (*http.Request, error) {
				return NewGetV1MetricsRequest(server, &GetV1MetricsParams{PsirtComponent: &yes})
			},
		},
		{
			name: "metrics psirt false", path: "/v1/metrics", query: url.Values{"psirtComponent": {"false"}},
			call: func() (*http.Request, error) {
				return NewGetV1MetricsRequest(server, &GetV1MetricsParams{PsirtComponent: &no})
			},
		},
		{
			name: "nodegroups legacy zone", path: "/v1/nodegroups", query: url.Values{"computeZoneId": {"zone-legacy"}},
			call: func() (*http.Request, error) {
				return NewGetV1NodegroupsRequest(server, &GetV1NodegroupsParams{ComputeZoneId: &zone})
			},
		},
		{
			name: "nodes agent type", path: "/v1/nodes", query: url.Values{"agentType": {"oob"}},
			call: func() (*http.Request, error) {
				return NewGetV1NodesRequest(server, &GetV1NodesParams{AgentType: &nodeAgent})
			},
		},
		{
			name: "nodes integrity checks", path: "/v1/nodes", query: url.Values{"integrityChecks": {"Verified", "Unverified"}},
			call: func() (*http.Request, error) {
				return NewGetV1NodesRequest(server, &GetV1NodesParams{IntegrityChecks: &checks})
			},
		},
		{
			name: "inventory agent type", path: "/v1/reports/inventory", query: url.Values{"agentType": {"oob"}},
			call: func() (*http.Request, error) {
				return NewGetV1ReportsInventoryRequest(server, &GetV1ReportsInventoryParams{AgentType: &inventoryAgent})
			},
		},
		{
			name: "security vulnerability filters", path: "/v1/reports/security/vulnerabilities",
			query: url.Values{"nodeGroupIds": {"group-1", "group-2"}, "gpuTypes": {"H100", "A100"}},
			call: func() (*http.Request, error) {
				return NewGetV1ReportsSecurityVulnerabilitiesRequest(server, &GetV1ReportsSecurityVulnerabilitiesParams{NodeGroupIds: &groups, GpuTypes: &gpuTypes})
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := tc.call()
			if err != nil {
				t.Fatalf("construct request: %v", err)
			}
			if req == nil {
				t.Fatal("construct request returned nil request")
			}
			if req.Method != http.MethodGet {
				t.Errorf("method = %q, want GET", req.Method)
			}
			if req.URL.Path != tc.path {
				t.Errorf("path = %q, want %q", req.URL.Path, tc.path)
			}
			if got := req.URL.Query(); !reflect.DeepEqual(got, tc.query) {
				t.Errorf("query = %v, want %v", got, tc.query)
			}
		})
	}
}

func TestRound2DeleteNodeAgentTypeRequest(t *testing.T) {
	agentType := DeleteV1NodesNodeUuidParamsAgentTypeOob
	req, err := NewDeleteV1NodesNodeUuidRequest("https://example.test/", "node-1", &DeleteV1NodesNodeUuidParams{AgentType: &agentType})
	if err != nil {
		t.Fatalf("construct request: %v", err)
	}
	if req.Method != http.MethodDelete || req.URL.Path != "/v1/nodes/node-1" {
		t.Errorf("request = %s %s, want DELETE /v1/nodes/node-1", req.Method, req.URL.Path)
	}
	want := url.Values{"agentType": {"oob"}}
	if got := req.URL.Query(); !reflect.DeepEqual(got, want) {
		t.Errorf("query = %v, want %v", got, want)
	}
}

func TestRound2V2MetricsNameFilters(t *testing.T) {
	group := "training group"
	zone := "zone-1"
	cases := []struct {
		name   string
		params GetV2MetricsParams
		query  url.Values
	}{
		{
			name:   "legacy group name",
			params: GetV2MetricsParams{GroupBy: GetV2MetricsParamsGroupByNodegroup, GroupName: &group, MetricNames: []string{"gpu_util"}, Step: "5m"},
			query:  url.Values{"groupBy": {"nodegroup"}, "groupName": {"training group"}, "metricNames": {"gpu_util"}, "step": {"5m"}},
		},
		{
			name:   "node group and compute zone ID",
			params: GetV2MetricsParams{GroupBy: GetV2MetricsParamsGroupByNodegroup, NodeGroupName: &group, ComputeZoneId: &zone, MetricNames: []string{"gpu_util"}, Step: "5m"},
			query:  url.Values{"groupBy": {"nodegroup"}, "nodeGroupName": {"training group"}, "computeZoneId": {"zone-1"}, "metricNames": {"gpu_util"}, "step": {"5m"}},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := NewGetV2MetricsRequest("https://example.test/", &tc.params)
			if err != nil {
				t.Fatalf("construct request: %v", err)
			}
			if req.Method != http.MethodGet || req.URL.Path != "/v2/metrics" {
				t.Errorf("request = %s %s, want GET /v2/metrics", req.Method, req.URL.Path)
			}
			if got := req.URL.Query(); !reflect.DeepEqual(got, tc.query) {
				t.Errorf("query = %v, want %v", got, tc.query)
			}
		})
	}
}

func TestRound2WithBaseURLRequest(t *testing.T) {
	client, err := NewClient("https://original.example/", WithBaseURL("https://replacement.example/api"))
	if err != nil {
		t.Fatalf("construct client: %v", err)
	}
	if client.Server != "https://replacement.example/api/" {
		t.Errorf("server = %q, want https://replacement.example/api/", client.Server)
	}
	req, err := NewGetV1NodesRequest(client.Server, nil)
	if err != nil {
		t.Fatalf("construct request: %v", err)
	}
	if req.Method != http.MethodGet || req.URL.String() != "https://replacement.example/api/v1/nodes" {
		t.Errorf("request = %s %s, want GET https://replacement.example/api/v1/nodes", req.Method, req.URL)
	}
}

func TestRound2ComputeZoneNonfiniteRequestRejection(t *testing.T) {
	for _, coordinate := range []string{"latitude", "longitude"} {
		t.Run(coordinate, func(t *testing.T) {
			latitude, longitude := float32(39.5), float32(-76.25)
			if coordinate == "latitude" {
				latitude = float32(math.NaN())
			} else {
				longitude = float32(math.NaN())
			}
			body := ModelsUpdateComputeZoneRequest{
				Id:       "cz-1",
				Location: &ModelsLocation{Latitude: &latitude, Longitude: &longitude},
			}

			req, err := NewPutV1ComputezonesRequest("https://example.test", body)
			if req != nil {
				t.Errorf("request = %v, want nil", req)
			}
			var unsupported *json.UnsupportedValueError
			if !errors.As(err, &unsupported) {
				t.Errorf("request error = %v, want *json.UnsupportedValueError", err)
			}

			doerCalled, editorCalled := false, false
			doer := authStatusTestDoer(func(_ *http.Request) (*http.Response, error) {
				doerCalled = true
				return nil, errors.New("unexpected transport call")
			})
			client, err := NewClientWithResponses("https://example.test", WithHTTPClient(doer))
			if err != nil {
				t.Fatalf("NewClientWithResponses: %v", err)
			}
			editor := func(_ context.Context, _ *http.Request) error {
				editorCalled = true
				return nil
			}
			response, err := client.PutV1ComputezonesWithResponse(context.Background(), body, editor)
			if response != nil {
				t.Errorf("response = %v, want nil", response)
			}
			unsupported = nil
			if !errors.As(err, &unsupported) {
				t.Errorf("client error = %v, want *json.UnsupportedValueError", err)
			}
			if editorCalled {
				t.Error("request editor called for non-finite coordinate")
			}
			if doerCalled {
				t.Error("HTTP doer called for non-finite coordinate")
			}
		})
	}
}
