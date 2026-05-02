package scalars

import (
	"encoding/json"
	"reflect"
)

// MarshalJSON double-encodes a value into an escaped JSON string for
// transport over Massdriver's `JSON`/`Map` GraphQL scalars.
//
// Empty/nil maps return an empty byte slice rather than the encoded literal
// "null" so genqlient's `omitempty` on the wrapping json.RawMessage drops the
// field entirely. Without this, an unset attributes/payload field would be
// sent as the JSON string `"null"`, which the server unmarshals into a real
// `null` and then rejects (the schema accepts the field's omission but not an
// explicit null).
func MarshalJSON(v any) ([]byte, error) {
	if isEmpty(v) {
		return []byte{}, nil
	}
	bytes, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return json.Marshal(string(bytes))
}

// UnmarshalJSON unmarshals raw JSON bytes into the provided map.
func UnmarshalJSON(data []byte, v *map[string]any) error {
	return json.Unmarshal(data, v)
}

func isEmpty(v any) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	for rv.Kind() == reflect.Ptr {
		if rv.IsNil() {
			return true
		}
		rv = rv.Elem()
	}
	switch rv.Kind() {
	case reflect.Map, reflect.Slice, reflect.Array:
		return rv.Len() == 0
	}
	return false
}
