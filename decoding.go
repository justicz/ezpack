package ezpack

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"reflect"
)

var ErrBufTooShort = errors.New("buffer too short")

func Decode(data io.Reader, o interface{}) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("recovered panic in Decode: %s", r)
		}
	}()

	return decodeStruct(data, o)
}

func DecodeBytes(data []byte, o interface{}) (err error) {
	buf := bytes.NewBuffer(data)
	return Decode(buf, o)
}

func decodeCommonHeader(data io.Reader, expectedType byte) (length uint32, err error) {
	// Read in the 5 byte header
	var headerBytes [5]byte
	_, err = io.ReadFull(data, headerBytes[:])
	if err != nil {
		return 0, ErrBufTooShort
	}

	// Ensure we got the expected type
	if headerBytes[0] != expectedType {
		return 0, fmt.Errorf("got wrong header byte %x, wanted %x", headerBytes[0], expectedType)
	}

	// Decode the length
	length = binary.BigEndian.Uint32(headerBytes[1:5])
	return
}

func decodeSlice(data io.Reader, maxLen uint32) ([]byte, error) {
	// Decode the header
	length, err := decodeCommonHeader(data, PackBytesID)
	if err != nil {
		return nil, err
	}

	// Ensure the decoded length isn't too long
	if (length > maxLen) || (length > math.MaxInt32) {
		errs := "cannot decode slice of length %d, max is %d"
		if maxLen == 0 {
			errs += " -- did you remember to set a max length in the struct tag?"
		}
		return nil, fmt.Errorf(errs, length, maxLen)
	}

	// Allocate space for the bytes
	out := make([]byte, length)

	// Read the requested number of bytes
	_, err = io.ReadFull(data, out)
	if err != nil {
		return nil, ErrBufTooShort
	}

	return out, nil
}

func decodeString(data io.Reader, maxLen uint32) (string, error) {
	// Decode the header
	length, err := decodeCommonHeader(data, PackStringID)
	if err != nil {
		return "", err
	}

	// Ensure the decoded length isn't too long
	if (length > maxLen) || (length > math.MaxInt32) {
		errs := "cannot decode string of length %d, max is %d"
		if maxLen == 0 {
			errs += " -- did you remember to set a max length in the struct tag?"
		}
		return "", fmt.Errorf(errs, length, maxLen)
	}

	// Allocate space for the bytes
	s := make([]byte, length)

	// Read the requested number of bytes
	_, err = io.ReadFull(data, s[:])
	if err != nil {
		return "", ErrBufTooShort
	}

	return string(s), nil
}

func decodeUint64(data io.Reader) (uint64, error) {
	// Read in the 9-byte encoded uint64
	var encoded [9]byte
	_, err := io.ReadFull(data, encoded[:])
	if err != nil {
		return 0, ErrBufTooShort
	}

	// Ensure we got the expected type
	if encoded[0] != PackUint64ID {
		return 0, fmt.Errorf("got wrong header byte %x, wanted %x", encoded[0], PackUint64ID)
	}

	// Decode the uint64
	return binary.BigEndian.Uint64(encoded[1:]), nil
}

func decodeStruct(data io.Reader, o interface{}) (err error) {
	// Take the value of the interface{} object
	v := reflect.ValueOf(o)

	// Dereference once if passed pointer
	if v.Kind() == reflect.Ptr {
		// Elem returns the value that the pointer points to
		v = v.Elem()
	}

	// At this point we should have a struct
	if v.Kind() != reflect.Struct {
		return fmt.Errorf("Decode requires struct, not %s", v.Kind())
	}

	// Type returns the user-defined struct type
	t := v.Type()

	/*
	 * 1. Decode Map Header
	 * We are decoding a struct, so this should be a map with as many entries
	 * as this struct has fields.
	 */

	// Sanity check that we're not decoding too many fields
	numFields := v.NumField()
	if (numFields <= 0) || (numFields > math.MaxInt32) {
		return fmt.Errorf("cannot decode massive struct or struct with no fields")
	}

	// Decode the header
	mapLen, err := decodeCommonHeader(data, PackMapID)
	if err != nil {
		return err
	}

	// Check that the map has the expected number of entries
	if mapLen != uint32(numFields) {
		return fmt.Errorf("got wrong map size for struct %s when decoding map", t.Name())
	}

	// Sort this struct's fields so we know what order we should expect things
	// on the wire
	parsedFields, err := sortStructFields(t)
	if err != nil {
		return err
	}

	/*
	 * 2. Decode Map key/value pairs and map to struct fields
	 * Decode a string, ensure it matches the name specified in the struct tag,
	 * and attempt to decode a value corresponding to the field type.
	 */

	// Iterate over the struct's fields, and decode an appropriate type for each
	for _, parsedField := range parsedFields {
		// Fetch the StructField from the type (info like name, struct tag, etc.)
		structField := t.Field(parsedField.offset)

		// Fetch the value of the field
		fieldValue := v.Field(parsedField.offset)

		// Ensure we can set this field
		if !fieldValue.CanSet() {
			return fmt.Errorf("Decode cannot set value of %s, did you pass a non-pointer?", structField.Name)
		}

		// Read a string, it should be the name specified in the struct tag
		var allegedName string
		allegedName, err = decodeString(data, maxTagFieldNameLength)
		if err != nil {
			return err
		}

		// Check that the name matches the expected value
		expectedName := parsedField.parsedStructTag.FieldName
		if allegedName != expectedName {
			return fmt.Errorf("got unexpected field name on wire, wanted %s", expectedName)
		}

		// Decode each field type we know about
		switch kind := structField.Type.Kind(); kind {
		case reflect.Slice:
			// Got a slice, ensure it's []byte or []uint8
			elType := structField.Type.Elem()
			elKind := elType.Kind()
			if elKind != reflect.Uint8 {
				return fmt.Errorf("can only decode slices of byte or uint8, not %s", elKind)
			}

			// Decode slice
			var dec []byte
			dec, err = decodeSlice(data, parsedField.parsedStructTag.MaxLen)
			if err != nil {
				err = fmt.Errorf("error decoding '%s': %s", expectedName, err)
				return
			}

			// Set the value to be the decoded bytes
			fieldValue.SetBytes(dec)
		case reflect.String:
			// Decode string
			var dec string
			dec, err = decodeString(data, parsedField.parsedStructTag.MaxLen)
			if err != nil {
				err = fmt.Errorf("error decoding '%s': %s", expectedName, err)
				return
			}

			// Set the value to be the decoded string
			fieldValue.SetString(dec)
		case reflect.Uint64:
			// Decode uint64
			var dec uint64
			dec, err = decodeUint64(data)
			if err != nil {
				err = fmt.Errorf("error decoding '%s': %s", expectedName, err)
				return
			}

			// Set the value to be the decoded uint64
			fieldValue.SetUint(dec)
		case reflect.Struct:
			// Ensure we can make a pointer to this field
			if !fieldValue.CanAddr() {
				return fmt.Errorf("Decode cannot call Addr() on %v", structField.Name)
			}

			// Make a pointer to this field
			valueAddr := fieldValue.Addr()

			// Ensure we can convert the pointer to this field to an interface
			if !valueAddr.CanInterface() {
				return fmt.Errorf("could not convert %s to interface", structField.Name)
			}

			// Convert pointer value to interface
			valueAddrIface := valueAddr.Interface()

			// Decode struct
			err = decodeStruct(data, valueAddrIface)
			if err != nil {
				err = fmt.Errorf("error decoding '%s': %s", expectedName, err)
				return
			}
		default:
			return fmt.Errorf("decode does not know how to decode into %s", kind)
		}
	}

	return
}
