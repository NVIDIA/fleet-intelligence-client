// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

// Package cmdutil is the glue between cobra and the nvfleetint SDK that the
// resource command packages share: the common flags every command registers,
// the client those flags build, and the parsing, pagination, and error
// rendering that would otherwise be restated once per command.
//
// It is deliberately the only shared internal package that imports both cobra
// and the SDK. Command packages may import both because they define cobra
// commands and execute SDK calls, but shared glue lives here. Formatting that
// needs neither belongs in internal/output; rules about what the API accepts
// belong in the SDK.
package cmdutil
