// SPDX-FileCopyrightText: Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES. All rights reserved.
// SPDX-License-Identifier: Apache-2.0

package nvfleetint

// This file checks response payloads against the constraints the OpenAPI
// contract actually declares, before the SDK maps them into domain types.
//
// The generated client decodes into strong Go types, so a value of the wrong
// JSON kind already fails to unmarshal. What that does not catch is a
// well-formed payload carrying values the contract forbids: an IP field that is
// not an address, a hostname carrying terminal escape sequences, an alert
// severity outside the enum. Those reach the operator as rendered fleet state
// and are what a tampered or compromised backend would use.
//
// Scope, stated plainly: this validates the constraints openapi.yaml declares
// for the fields the threat model names, keyed by field name. It is not a
// general JSON Schema engine bound to each operation's response schema — that
// would need a schema validator dependency and the spec embedded in the binary.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/NVIDIA/fleet-intelligence-client/internal/generated/fleetapi"
)

// ErrInvalidResponse indicates an API response did not match the contract and
// was rejected rather than decoded. Match it with errors.Is.
var ErrInvalidResponse = errors.New("invalid API response")

// maxHostnameLength is the DNS limit on a fully qualified name
const maxHostnameLength = 253

// ResponseValidationError names the response field that failed validation.
// The value is rendered with %q so a hostile value cannot emit raw control
// bytes through an operator's terminal by way of the error message.
type ResponseValidationError struct {
	Field  string
	Reason string
}

// Error renders the field and the reason it was rejected
func (e *ResponseValidationError) Error() string {
	return fmt.Sprintf("%s: field %q %s", ErrInvalidResponse, e.Field, e.Reason)
}

// Unwrap ties the error to ErrInvalidResponse for errors.Is
func (e *ResponseValidationError) Unwrap() error {
	return ErrInvalidResponse
}

// fieldValidators maps a lowercased JSON field name to the check applied
// wherever that name appears in a response. Field names are matched
// case-insensitively because the contract spells the same concept both
// nodeUUID and nodeUuid depending on the endpoint.
//
// Only fields whose shape the spec constrains are listed. alertStatus, for
// example, is declared as a bare string whose description merely gives
// examples, so validating it against a value set would enforce a rule the
// contract does not make.
var fieldValidators = map[string]func(string) error{
	"hostname":  validateResponseHostname,
	"publicip":  validateResponseIP,
	"privateip": validateResponseIP,
	"severity":  validateResponseAlertSeverity,
	"state":     validateResponseAlertState,
}

// Checks a JSON response body against the contract, returning a
// ResponseValidationError for the first field that violates it.
//
// The walk is streaming: it reads tokens rather than decoding the payload into
// an interface tree, so validating a large response does not add a second copy
// of it to memory. A body that is not JSON at all — the CSV and ZIP report
// payloads — is skipped, as is malformed JSON, which the caller's typed
// unmarshal reports with a better message.
func validateResponsePayload(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	// containers records the open nesting, true for an object. expectKey is
	// true when the next string inside the innermost object is a field name
	// rather than a value.
	var containers []bool
	expectKey := false
	key := ""

	for {
		token, err := decoder.Token()
		if err != nil {
			return nil
		}

		switch value := token.(type) {
		case json.Delim:
			switch value {
			case '{':
				containers = append(containers, true)
				expectKey = true
				continue
			case '[':
				containers = append(containers, false)
				expectKey = false
				continue
			default:
				if len(containers) > 0 {
					containers = containers[:len(containers)-1]
				}
			}
		case string:
			if expectKey {
				key = value
				expectKey = false
				continue
			}
			if err := validateResponseField(key, value); err != nil {
				return err
			}
		}

		// A completed value means the innermost object expects its next key.
		// Inside an array, elements keep the array's own field name.
		expectKey = len(containers) > 0 && containers[len(containers)-1]
	}
}

// Applies the universal and field-specific checks to one string value
func validateResponseField(key, value string) error {
	if err := validateNoControlCharacters(key, value); err != nil {
		return err
	}
	validate, ok := fieldValidators[strings.ToLower(key)]
	if !ok {
		return nil
	}
	if err := validate(value); err != nil {
		return &ResponseValidationError{Field: key, Reason: err.Error()}
	}

	return nil
}

// Rejects control characters in any response string. Tab, newline, and return
// are allowed because alert messages legitimately wrap; the table renderer
// collapses them (internal/output.sanitizeTableCell) so they cannot forge rows.
// Everything else in C0 and C1 — most importantly ESC, which starts an ANSI
// sequence that could rewrite an operator's terminal — has no legitimate place
// in fleet data.
func validateNoControlCharacters(key, value string) error {
	for _, r := range value {
		if r == '\t' || r == '\n' || r == '\r' {
			continue
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return &ResponseValidationError{
				Field:  key,
				Reason: fmt.Sprintf("contains control character %U", r),
			}
		}
	}

	return nil
}

// Rejects an address field that is not an IP address. An empty value means the
// backend did not report one, which the contract allows.
func validateResponseIP(value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	if net.ParseIP(trimmed) == nil {
		return fmt.Errorf("is not a valid IP address: %q", value)
	}

	return nil
}

// Rejects a hostname carrying characters that cannot appear in a DNS name.
// This is a charset and length check rather than full RFC 1123 label parsing:
// real fleets carry names that bend the RFC, and rejecting a whole inventory
// listing over a leading digit or an underscore would turn a display concern
// into an outage of the tool. Anything that could carry markup, escapes, or
// whitespace into rendered output is still refused.
func validateResponseHostname(value string) error {
	if value == "" {
		return nil
	}
	if len(value) > maxHostnameLength {
		return fmt.Errorf("exceeds the %d character DNS limit", maxHostnameLength)
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '-', r == '.', r == '_':
		default:
			return fmt.Errorf("contains an invalid hostname character: %q", value)
		}
	}

	return nil
}

// Rejects a severity outside the contract's enum. The allowed set comes from
// the generated types, so regenerating the client from an updated spec widens
// it automatically. This mirrors the SDK's existing rejection of an unknown
// severity on the request side.
func validateResponseAlertSeverity(value string) error {
	if value == "" {
		return nil
	}
	if !fleetapi.ModelsAlertSeverity(value).Valid() {
		return fmt.Errorf("is not a valid alert severity: %q", value)
	}

	return nil
}

// Rejects an alert state outside the contract's enum
func validateResponseAlertState(value string) error {
	if value == "" {
		return nil
	}
	if !fleetapi.ModelsAlertState(value).Valid() {
		return fmt.Errorf("is not a valid alert state: %q", value)
	}

	return nil
}
