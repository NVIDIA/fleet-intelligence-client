// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package event

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

const eventsBody = `{"events":[{"eventId":"e1","nodeUUID":"node-1","component":"GPU","type":"error","name":"xid","message":"boom","timestamp":"2026-05-01T00:00:00Z","createdAt":"2026-05-01T00:00:05Z","suggestedActions":[{"action":"reboot node","code":"R1"}]}],"hasMore":false,"page":0,"pageSize":50,"total":1}`

type eventFailingWriter struct {
	bytes.Buffer
	writes int
	failAt int
	err    error
}

func (w *eventFailingWriter) Write(p []byte) (int, error) {
	w.writes++
	if w.writes == w.failAt {
		return 0, w.err
	}
	return w.Buffer.Write(p)
}
