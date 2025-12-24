package sdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

const (
	customerMetadataMaxBytes     = 10 * 1024
	customerMetadataMaxDepth     = 5
	customerMetadataMaxKeyLength = 40
)

type CustomerMetadataValueKind string

const (
	CustomerMetadataValueNull   CustomerMetadataValueKind = "null"
	CustomerMetadataValueString CustomerMetadataValueKind = "string"
	CustomerMetadataValueBool   CustomerMetadataValueKind = "bool"
	CustomerMetadataValueNumber CustomerMetadataValueKind = "number"
	CustomerMetadataValueObject CustomerMetadataValueKind = "object"
	CustomerMetadataValueArray  CustomerMetadataValueKind = "array"
)

// CustomerMetadataValue represents a typed metadata value without using interface{}.
type CustomerMetadataValue struct {
	kind        CustomerMetadataValueKind
	stringValue string
	boolValue   bool
	numberValue json.Number
	objectValue CustomerMetadata
	arrayValue  []CustomerMetadataValue
}

func (v CustomerMetadataValue) Kind() CustomerMetadataValueKind {
	return v.kind
}

func CustomerMetadataNull() CustomerMetadataValue {
	return CustomerMetadataValue{kind: CustomerMetadataValueNull}
}

func CustomerMetadataString(val string) CustomerMetadataValue {
	return CustomerMetadataValue{kind: CustomerMetadataValueString, stringValue: val}
}

func CustomerMetadataBool(val bool) CustomerMetadataValue {
	return CustomerMetadataValue{kind: CustomerMetadataValueBool, boolValue: val}
}

func CustomerMetadataNumber(val json.Number) CustomerMetadataValue {
	return CustomerMetadataValue{kind: CustomerMetadataValueNumber, numberValue: val}
}

func CustomerMetadataObject(val CustomerMetadata) CustomerMetadataValue {
	return CustomerMetadataValue{kind: CustomerMetadataValueObject, objectValue: val}
}

func CustomerMetadataArray(val []CustomerMetadataValue) CustomerMetadataValue {
	return CustomerMetadataValue{kind: CustomerMetadataValueArray, arrayValue: val}
}

func (v CustomerMetadataValue) StringValue() (string, error) {
	if v.kind == CustomerMetadataValueString {
		return v.stringValue, nil
	}
	return "", CustomerMetadataError{Type: CustomerMetadataErrorInvalidType, Detail: "metadata value is not a string"}
}

func (v CustomerMetadataValue) BoolValue() (bool, error) {
	if v.kind == CustomerMetadataValueBool {
		return v.boolValue, nil
	}
	return false, CustomerMetadataError{Type: CustomerMetadataErrorInvalidType, Detail: "metadata value is not a bool"}
}

func (v CustomerMetadataValue) NumberValue() (json.Number, error) {
	if v.kind == CustomerMetadataValueNumber {
		return v.numberValue, nil
	}
	return json.Number(""), CustomerMetadataError{Type: CustomerMetadataErrorInvalidType, Detail: "metadata value is not a number"}
}

func (v CustomerMetadataValue) ObjectValue() (CustomerMetadata, error) {
	if v.kind == CustomerMetadataValueObject {
		return v.objectValue, nil
	}
	return nil, CustomerMetadataError{Type: CustomerMetadataErrorInvalidType, Detail: "metadata value is not an object"}
}

func (v CustomerMetadataValue) ArrayValue() ([]CustomerMetadataValue, error) {
	if v.kind == CustomerMetadataValueArray {
		return v.arrayValue, nil
	}
	return nil, CustomerMetadataError{Type: CustomerMetadataErrorInvalidType, Detail: "metadata value is not an array"}
}

func (v CustomerMetadataValue) MarshalJSON() ([]byte, error) {
	switch v.kind {
	case CustomerMetadataValueNull:
		return []byte("null"), nil
	case CustomerMetadataValueString:
		return json.Marshal(v.stringValue)
	case CustomerMetadataValueBool:
		return json.Marshal(v.boolValue)
	case CustomerMetadataValueNumber:
		return json.Marshal(v.numberValue)
	case CustomerMetadataValueObject:
		return json.Marshal(v.objectValue)
	case CustomerMetadataValueArray:
		return json.Marshal(v.arrayValue)
	default:
		return nil, CustomerMetadataError{Type: CustomerMetadataErrorInvalidType, Detail: "metadata value has unknown kind"}
	}
}

// CustomerMetadata wraps structured customer metadata.
type CustomerMetadata map[string]CustomerMetadataValue

// Get retrieves a metadata value by key.
func (m CustomerMetadata) Get(key string) (CustomerMetadataValue, bool) {
	if m == nil {
		return CustomerMetadataValue{}, false
	}
	v, ok := m[key]
	return v, ok
}

// GetString retrieves a string value by key.
// Returns a typed error when the key is missing or the value is not a string.
func (m CustomerMetadata) GetString(key string) (string, error) {
	if m == nil {
		return "", CustomerMetadataError{Type: CustomerMetadataErrorMissing, Path: metadataPath("metadata", key)}
	}
	v, ok := m[key]
	if !ok {
		return "", CustomerMetadataError{Type: CustomerMetadataErrorMissing, Path: metadataPath("metadata", key)}
	}
	val, err := v.StringValue()
	if err != nil {
		if metaErr, ok := err.(CustomerMetadataError); ok && metaErr.Path == "" {
			metaErr.Path = metadataPath("metadata", key)
			return "", metaErr
		}
		return "", err
	}
	return val, nil
}

// Set stores a metadata value. Initializes the map if nil.
func (m *CustomerMetadata) Set(key string, value CustomerMetadataValue) {
	if *m == nil {
		*m = make(CustomerMetadata)
	}
	(*m)[key] = value
}

type CustomerMetadataErrorType string

const (
	CustomerMetadataErrorInvalidType CustomerMetadataErrorType = "invalid_type"
	CustomerMetadataErrorKeyTooLong  CustomerMetadataErrorType = "key_too_long"
	CustomerMetadataErrorTooDeep     CustomerMetadataErrorType = "too_deep"
	CustomerMetadataErrorTooLarge    CustomerMetadataErrorType = "too_large"
	CustomerMetadataErrorMissing     CustomerMetadataErrorType = "missing"
)

type CustomerMetadataError struct {
	Type   CustomerMetadataErrorType
	Path   string
	Detail string
}

func (e CustomerMetadataError) Error() string {
	if e.Detail != "" {
		if e.Path != "" {
			return fmt.Sprintf("%s: %s", e.Path, e.Detail)
		}
		return e.Detail
	}
	switch e.Type {
	case CustomerMetadataErrorMissing:
		if e.Path != "" {
			return fmt.Sprintf("%s: metadata key missing", e.Path)
		}
		return "metadata key missing"
	case CustomerMetadataErrorKeyTooLong:
		return fmt.Sprintf("%s: key exceeds %d characters", e.Path, customerMetadataMaxKeyLength)
	case CustomerMetadataErrorTooDeep:
		return fmt.Sprintf("%s: nesting exceeds %d levels", e.Path, customerMetadataMaxDepth)
	case CustomerMetadataErrorTooLarge:
		return fmt.Sprintf("metadata exceeds %d bytes", customerMetadataMaxBytes)
	default:
		if e.Path != "" {
			return fmt.Sprintf("%s: invalid metadata value", e.Path)
		}
		return "invalid metadata value"
	}
}

// UnmarshalJSON enforces typed metadata parsing without interface{} leakage.
func (m *CustomerMetadata) UnmarshalJSON(data []byte) error {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || bytes.Equal(data, []byte("null")) {
		*m = nil
		return nil
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw map[string]any
	if err := dec.Decode(&raw); err != nil {
		return err
	}
	converted, err := convertMetadataMap(raw, "metadata", 1)
	if err != nil {
		return err
	}
	*m = converted
	return nil
}

// Validate enforces JSON-compatible metadata values and guardrails.
func (m CustomerMetadata) Validate() error {
	if m == nil {
		return nil
	}
	if err := validateCustomerMetadataMap(m, "metadata", 1); err != nil {
		return err
	}
	data, err := json.Marshal(m)
	if err != nil {
		return CustomerMetadataError{
			Type:   CustomerMetadataErrorInvalidType,
			Path:   "metadata",
			Detail: "metadata must be valid JSON",
		}
	}
	if len(data) > customerMetadataMaxBytes {
		return CustomerMetadataError{Type: CustomerMetadataErrorTooLarge, Path: "metadata"}
	}
	return nil
}

func convertMetadataMap(values map[string]any, path string, depth int) (CustomerMetadata, error) {
	if depth > customerMetadataMaxDepth {
		return nil, CustomerMetadataError{Type: CustomerMetadataErrorTooDeep, Path: path}
	}
	out := make(CustomerMetadata, len(values))
	for key, value := range values {
		if utf8.RuneCountInString(key) > customerMetadataMaxKeyLength {
			return nil, CustomerMetadataError{Type: CustomerMetadataErrorKeyTooLong, Path: metadataPath(path, key)}
		}
		converted, err := convertMetadataValue(value, metadataPath(path, key), depth+1)
		if err != nil {
			return nil, err
		}
		out[key] = converted
	}
	return out, nil
}

func convertMetadataSlice(values []any, path string, depth int) ([]CustomerMetadataValue, error) {
	if depth > customerMetadataMaxDepth {
		return nil, CustomerMetadataError{Type: CustomerMetadataErrorTooDeep, Path: path}
	}
	out := make([]CustomerMetadataValue, 0, len(values))
	for i, value := range values {
		converted, err := convertMetadataValue(value, metadataIndexPath(path, i), depth+1)
		if err != nil {
			return nil, err
		}
		out = append(out, converted)
	}
	return out, nil
}

func convertMetadataValue(value any, path string, depth int) (CustomerMetadataValue, error) {
	switch v := value.(type) {
	case nil:
		return CustomerMetadataNull(), nil
	case string:
		return CustomerMetadataString(v), nil
	case bool:
		return CustomerMetadataBool(v), nil
	case json.Number:
		return CustomerMetadataNumber(v), nil
	case float64:
		return CustomerMetadataNumber(json.Number(fmt.Sprintf("%v", v))), nil
	case float32:
		return CustomerMetadataNumber(json.Number(fmt.Sprintf("%v", v))), nil
	case int:
		return CustomerMetadataNumber(json.Number(fmt.Sprintf("%d", v))), nil
	case int64:
		return CustomerMetadataNumber(json.Number(fmt.Sprintf("%d", v))), nil
	case int32:
		return CustomerMetadataNumber(json.Number(fmt.Sprintf("%d", v))), nil
	case uint:
		return CustomerMetadataNumber(json.Number(fmt.Sprintf("%d", v))), nil
	case uint64:
		return CustomerMetadataNumber(json.Number(fmt.Sprintf("%d", v))), nil
	case uint32:
		return CustomerMetadataNumber(json.Number(fmt.Sprintf("%d", v))), nil
	case map[string]any:
		converted, err := convertMetadataMap(v, path, depth)
		if err != nil {
			return CustomerMetadataValue{}, err
		}
		return CustomerMetadataObject(converted), nil
	case []any:
		converted, err := convertMetadataSlice(v, path, depth)
		if err != nil {
			return CustomerMetadataValue{}, err
		}
		return CustomerMetadataArray(converted), nil
	default:
		return CustomerMetadataValue{}, CustomerMetadataError{Type: CustomerMetadataErrorInvalidType, Path: path}
	}
}

func validateCustomerMetadataMap(values CustomerMetadata, path string, depth int) error {
	if depth > customerMetadataMaxDepth {
		return CustomerMetadataError{Type: CustomerMetadataErrorTooDeep, Path: path}
	}
	for key, value := range values {
		if utf8.RuneCountInString(key) > customerMetadataMaxKeyLength {
			return CustomerMetadataError{Type: CustomerMetadataErrorKeyTooLong, Path: metadataPath(path, key)}
		}
		if err := validateCustomerMetadataValue(value, metadataPath(path, key), depth+1); err != nil {
			return err
		}
	}
	return nil
}

func validateCustomerMetadataValue(value CustomerMetadataValue, path string, depth int) error {
	switch value.kind {
	case CustomerMetadataValueNull, CustomerMetadataValueString, CustomerMetadataValueBool, CustomerMetadataValueNumber:
		return nil
	case CustomerMetadataValueObject:
		return validateCustomerMetadataMap(value.objectValue, path, depth)
	case CustomerMetadataValueArray:
		if depth > customerMetadataMaxDepth {
			return CustomerMetadataError{Type: CustomerMetadataErrorTooDeep, Path: path}
		}
		for i, item := range value.arrayValue {
			if err := validateCustomerMetadataValue(item, metadataIndexPath(path, i), depth+1); err != nil {
				return err
			}
		}
		return nil
	default:
		return CustomerMetadataError{Type: CustomerMetadataErrorInvalidType, Path: path}
	}
}

func metadataPath(parent, key string) string {
	if parent == "" {
		return key
	}
	return parent + "." + key
}

func metadataIndexPath(parent string, index int) string {
	return fmt.Sprintf("%s[%d]", parent, index)
}
