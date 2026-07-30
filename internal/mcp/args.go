package mcp

import (
	"fmt"
	"strconv"
)

// ArgError is a structured argument validation error produced when an MCP
// tool argument has an unsupported type or a malformed value. It implements
// the error interface and identifies the field, the rejected value, and a
// human-readable reason.
type ArgError struct {
	Field  string      // The argument field name (e.g. "max_chars")
	Value  interface{} // The rejected value as received
	Reason string      // Human-readable explanation
}

func (e *ArgError) Error() string {
	return fmt.Sprintf("invalid argument %q: %s (got %v)", e.Field, e.Reason, e.Value)
}

// intArg extracts an integer argument from the args map.
//
// It accepts:
//   - canonical JSON numbers (decoded as float64 by encoding/json)
//   - stringified decimal integer literals (e.g. "42", "0", "-1")
//
// When the key is absent it returns (0, false, nil).
// Malformed or unsupported values return (0, true, *ArgError).
func intArg(args map[string]interface{}, key string) (int, bool, error) {
	v, ok := args[key]
	if !ok {
		return 0, false, nil
	}
	switch val := v.(type) {
	case float64:
		return int(val), true, nil
	case string:
		n, err := strconv.Atoi(val)
		if err != nil {
			return 0, true, &ArgError{Field: key, Value: v, Reason: "not a valid integer"}
		}
		return n, true, nil
	default:
		return 0, true, &ArgError{Field: key, Value: v, Reason: fmt.Sprintf("unsupported type %T (expected number)", v)}
	}
}

// boolArg extracts a boolean argument from the args map.
//
// It accepts:
//   - canonical JSON booleans
//   - stringified boolean literals "true" and "false" (case-sensitive)
//
// When the key is absent it returns (false, false, nil).
// Malformed or unsupported values return (false, true, *ArgError).
func boolArg(args map[string]interface{}, key string) (bool, bool, error) {
	v, ok := args[key]
	if !ok {
		return false, false, nil
	}
	switch val := v.(type) {
	case bool:
		return val, true, nil
	case string:
		switch val {
		case "true":
			return true, true, nil
		case "false":
			return false, true, nil
		default:
			return false, true, &ArgError{Field: key, Value: v, Reason: `not a valid boolean (expected "true" or "false")`}
		}
	default:
		return false, true, &ArgError{Field: key, Value: v, Reason: fmt.Sprintf("unsupported type %T (expected boolean)", v)}
	}
}

// stringArg extracts a string argument from the args map.
//
// When the key is absent it returns ("", false, nil).
// Non-string values return ("", true, *ArgError).
func stringArg(args map[string]interface{}, key string) (string, bool, error) {
	v, ok := args[key]
	if !ok {
		return "", false, nil
	}
	s, ok := v.(string)
	if !ok {
		return "", true, &ArgError{Field: key, Value: v, Reason: fmt.Sprintf("unsupported type %T (expected string)", v)}
	}
	return s, true, nil
}
