// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// CountResult is an exact count response without loading matching rows.
type CountResult struct {
	Total   int    `json:"total"`
	RawJSON []byte `json:"-"`
}

func decodeCount(status int, statusText string, body []byte) (CountResult, error) {
	if status != http.StatusOK {
		return CountResult{}, newAPIError(status, statusText, body)
	}
	var data struct {
		Total *int `json:"total"`
	}
	if err := json.Unmarshal(body, &data); err != nil {
		return CountResult{}, err
	}
	if data.Total == nil || *data.Total < 0 {
		return CountResult{}, fmt.Errorf("count response must contain a non-negative total")
	}
	return CountResult{Total: *data.Total, RawJSON: append([]byte(nil), body...)}, nil
}

func withExactTotal(body []byte, total int) ([]byte, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(body, &fields); err != nil {
		return nil, err
	}
	if fields == nil {
		return nil, fmt.Errorf("page response must be an object")
	}
	value, err := json.Marshal(total)
	if err != nil {
		return nil, err
	}
	fields["total"] = value
	return json.Marshal(fields)
}

// AbsoluteTimeRange freezes a relative window once so page/count and all-page
// requests share identical bounds. It does not create a database snapshot.
func AbsoluteTimeRange(window, start, end string) (string, string, error) {
	rangeInfo, err := normalizeEventTimeRange(window, start, end)
	if err != nil {
		return "", "", err
	}
	if rangeInfo.timeMode == eventTimeModeAbsolute {
		return rangeInfo.startTime, rangeInfo.endTime, nil
	}
	duration, err := time.ParseDuration(rangeInfo.window)
	if err != nil {
		return "", "", err
	}
	until := time.Now().UTC()
	return until.Add(-duration).Format(time.RFC3339Nano), until.Format(time.RFC3339Nano), nil
}
