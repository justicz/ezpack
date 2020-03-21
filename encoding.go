package encoding

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
)

var ErrOverflow = errors.New("integer overflow during encoding")
var ErrCopyingBytes = errors.New("copying error during encoding")

func Encode(o interface{}) (res []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered panic in Encode: %s", r)
		}
	}()

	// Convert o (should be struct or struct ptr) to PackMap, our
	// internal representation of a msgpack map
	mte, err := structToPackMap(o)
	if err != nil {
		return nil, err
	}

	// Encode PackMap as bytes
	return mte.Encode()
}

func (pv PackUint64) Encode() ([]byte, error) {
	// Allocate enough space
	buf := make([]byte, 9)

	// First byte: type identifier
	buf[0] = PackUint64ID

	// Next 8 bytes: big endian integer value
	binary.BigEndian.PutUint64(buf[1:], pv.Value)

	return buf, nil
}

func (pv PackBytes) Encode() ([]byte, error) {
	// Ensure we don't overflow when allocating space, even on 32-bit systems
	n := len(pv.Bytes) + 5
	if (n <= 0) || (n > math.MaxInt32) || (len(pv.Bytes) > math.MaxInt32) {
		return nil, ErrOverflow
	}

	// Allocate enough space
	buf := make([]byte, n)

	// First byte: type identifier
	buf[0] = PackBytesID

	// Next four bytes: big endian data length
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(pv.Bytes)))

	// Rest of bytes are data
	c := copy(buf[5:], pv.Bytes)

	// Sanity check, should have copied all the bytes
	if c != len(pv.Bytes) {
		return nil, ErrCopyingBytes
	}

	return buf, nil
}

func (pv PackString) Encode() ([]byte, error) {
	// Ensure we don't overflow when allocating space, even on 32-bit systems
	n := len(pv.String) + 5
	if (n <= 0) || (n > math.MaxInt32) || (len(pv.String) > math.MaxInt32) {
		return nil, ErrOverflow
	}

	// Allocate enough space
	buf := make([]byte, n)

	// First byte: type identifier
	buf[0] = PackStringID

	// Next four bytes: big endian data length
	binary.BigEndian.PutUint32(buf[1:5], uint32(len(pv.String)))

	// Rest of bytes are data
	c := copy(buf[5:], pv.String[:])

	// Sanity check, should have copied all the bytes
	if c != len(pv.String) {
		return nil, ErrCopyingBytes
	}

	return buf, nil
}

func (pv PackMap) Encode() ([]byte, error) {
	// Ensure we don't overflow when allocating space, even on 32-bit systems
	n := len(pv.Elements) + 1
	if (n <= 0) || (n > math.MaxInt32) || (len(pv.Elements) > math.MaxInt32) {
		return nil, ErrOverflow
	}

	// Allocate space for all of the encodings + the header
	encodings := make([][]byte, n)

	// Allocate space for map header
	header := make([]byte, 5)

	// First byte: type identifier
	header[0] = PackMapID

	// Next four bytes: big endian map length
	binary.BigEndian.PutUint32(header[1:], uint32(len(pv.Elements)))

	// Append header to slice of all encodings
	encodings = append(encodings, header)

	// Iterate over sorted keys and append encoding of each key, value
	for _, elt := range pv.Elements {
		// Encode the key
		kenc, err := elt.Key.Encode()
		if err != nil {
			return nil, err
		}

		// Append the key encoding
		encodings = append(encodings, kenc)

		// Encode the value
		venc, err := elt.Value.Encode()
		if err != nil {
			return nil, err
		}

		// Append the value encoding
		encodings = append(encodings, venc)
	}

	// Join the encodings together and return
	return bytes.Join(encodings, nil), nil
}

func structToPackMap(o interface{}) (*PackMap, error) {
	// Take the value of the interface{} object
	v := reflect.ValueOf(o)

	// Dereference once if passed pointer
	if v.Kind() == reflect.Ptr {
		// Elem returns the value that the pointer points to
		v = v.Elem()
	}

	// At this point we should have a struct
	if v.Kind() != reflect.Struct {
		return nil, fmt.Errorf("structToPackMap requires struct, not %s", v.Kind())
	}

	// Type returns the user-defined struct type
	t := v.Type()

	// Sanity check that we're not encoding too many fields
	numFields := v.NumField()
	if (numFields <= 0) || (numFields > math.MaxInt32) {
		return nil, fmt.Errorf("cannot encode massive struct or struct with no fields")
	}

	// mapToEncode will contain PackValues for each field in this struct. We will
	// fill this in (potentially recursively) and finish with mapToEncode.Encode()
	var mapToEncode PackMap
	mapToEncode.Elements = make([]PackMapElement, 0, numFields)

	// Sort this struct's fields alphabetically
	parsedFields, err := sortStructFields(t)
	if err != nil {
		return nil, err
	}

	// Iterate over the struct's fields
	for _, parsedField := range parsedFields {
		// Fetch the StructField from the type (info like name, struct tag, etc.)
		structField := t.Field(parsedField.offset)

		// Fetch the value of the field
		fieldValue := v.Field(parsedField.offset)

		// Start building the map element for this field
		mapEl := PackMapElement{
			Key: PackString{
				String: parsedField.parsedStructTag.FieldName,
			},
		}

		// Build our internal representation of the values to encode according to
		// to the types of the underlying fields
		switch kind := structField.Type.Kind(); kind {
		case reflect.Slice:
			// Got a slice, ensure it's []byte or []uint8
			elType := structField.Type.Elem()
			elKind := elType.Kind()
			if elKind != reflect.Uint8 {
				return nil, fmt.Errorf("can only encode slices of byte or uint8, not %s", elKind)
			}

			// Build ezpack struct to be encoded
			mapEl.Value = PackBytes{
				Bytes: fieldValue.Bytes(),
			}
		case reflect.String:
			// Build ezpack struct to be encoded
			mapEl.Value = PackString{
				String: fieldValue.String(),
			}
		case reflect.Uint64:
			// Build ezpack struct to be encoded
			mapEl.Value = PackUint64{
				Value: fieldValue.Uint(),
			}
		case reflect.Struct:
			// Ensure we can convert this field value to an interface
			if !fieldValue.CanInterface() {
				return nil, fmt.Errorf("could not convert %s to interface", structField.Name)
			}

			// Recursively encode this map
			fmap, err := structToPackMap(fieldValue.Interface())
			if err != nil {
				return nil, err
			}

			// Set child map to be encoded
			mapEl.Value = fmap
		default:
			return nil, fmt.Errorf("structToPackMap does not know how to handle %s", kind)
		}

		// Add the new map element
		mapToEncode.Elements = append(mapToEncode.Elements, mapEl)
	}

	return &mapToEncode, nil
}
