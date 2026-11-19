// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package alert

import (
	"bytes"
	"testing"
	"time"

	"github.com/NVIDIA/fleet-intelligence-client/internal/cmdutil"
	"github.com/spf13/cobra"
)

func TestRound2AlertSummaryTimelineFlagValidation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flags   alertTimelineFlags
		wantErr string
	}{
		{
			name:    "without PSIRT requires node",
			flags:   alertTimelineFlags{node: "  ", withoutPSIRT: true},
			wantErr: "--without-psirt requires --node",
		},
		{
			name:    "hostname conflicts with node",
			flags:   alertTimelineFlags{node: " node-1 ", hostname: " host-1 "},
			wantErr: "--hostname cannot be used with --node",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			cmd := &cobra.Command{}
			cmd.SetOut(&out)
			err := runAlertSummary(cmd, tc.flags, cmdutil.Resolved{Output: "table", Timeout: time.Second})
			if err == nil || err.Error() != tc.wantErr {
				t.Errorf("error = %v, want %q", err, tc.wantErr)
			}
			if out.Len() != 0 {
				t.Errorf("output = %q, want empty", out.String())
			}
		})
	}

	if err := validateAlertTimelineFlags(alertTimelineFlags{node: " node-1 ", hostname: "   "}); err != nil {
		t.Errorf("blank hostname validation error = %v, want nil", err)
	}
}
