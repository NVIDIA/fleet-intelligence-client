// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package xidburst

import (
	"bytes"

	"github.com/spf13/cobra"
)

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

const xidBurstsBody = `{"items":[{"burstId":"burst-1","nodeUuid":"node-1","hostname":"gpu-01","nodeGroup":"ng","nodeGroupId":"ng-1","computeZone":"cz","computeZoneId":"cz-1","startTime":"2026-05-01T00:00:00Z","endTime":"2026-05-01T00:05:00Z","burstDurationSeconds":300,"xidCount":2,"xidNumbers":[{"xidNumber":48,"mnemonic":"DBE","description":"Double Bit ECC"},{"xidNumber":94,"mnemonic":"CE"}],"deviceIds":{"0000:0f:00.0":[48,94]},"jobDisruption":true,"category":"GPU","subcategory":"Memory","stickyXidsSuppressed":1,"suggestedActions":[{"action":"Drain the node","code":"DRAIN","persona":"tenant","type":"immediate"}]}],"page":0,"pageSize":20,"total":1,"hasMore":false}`

type xidBurstFailingWriter struct {
	bytes.Buffer
	failAt int
	calls  int
	err    error
}

func (w *xidBurstFailingWriter) Write(p []byte) (int, error) {
	w.calls++
	if w.calls == w.failAt {
		return 0, w.err
	}
	return w.Buffer.Write(p)
}
