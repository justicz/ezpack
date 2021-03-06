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

func checkMaxLength(length, maxLength uint32) error {
	// If length is greater than the user-defined maximum, fail
	if length > maxLength {
		errs := "cannot decode value of length %d, max is %d"
		if maxLength == 0 {
			errs += " -- did you remember to set a max length in the struct tag?"
		}
		return fmt.Errorf(errs, length, maxLength)
	}

	// If length would cause problems for 32-bit signed integers, fail
	if length > math.MaxInt32 {
		return fmt.Errorf("cannot decode value of length %d: overflows 32-bit signed integers", length)
	}

	return nil
}

func decodeByteSlice(data io.Reader, maxLength uint32) ([]byte, error) {
	// Decode the header
	length, err := decodeCommonHeader(data, PackBytesID)
	if err != nil {
		return nil, err
	}

	// Enforce maximum length
	err = checkMaxLength(length, maxLength)
	if err != nil {
		return nil, err
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

func decodeString(data io.Reader, maxLength uint32) (string, error) {
	// Decode the header
	length, err := decodeCommonHeader(data, PackStringID)
	if err != nil {
		return "", err
	}

	// Enforce maximum length
	err = checkMaxLength(length, maxLength)
	if err != nil {
		return "", err
	}

	// Allocate space for the bytes
	out := make([]byte, length)

	// Read the requested number of bytes
	_, err = io.ReadFull(data, out)
	if err != nil {
		return "", ErrBufTooShort
	}

	return string(out), nil
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
		case reflect.Array:
			// We only support arrays of byte or uint8
			elType := structField.Type.Elem()
			elKind := elType.Kind()
			if elKind != reflect.Uint8 {
				return fmt.Errorf("only arrays of byte or uint8 are supported")
			}

			// Decode slice of byte or uint8
			var dec []byte
			dec, err = decodeByteSlice(data, parsedField.parsedStructTag.MaxLen)
			if err != nil {
				return fmt.Errorf("error decoding '%s': %s", expectedName, err)
			}

			// Length should be exact for arrays
			expectedLen := fieldValue.Len()
			if len(dec) != expectedLen {
				return fmt.Errorf("expected array of length %d, got %d", expectedLen, len(dec))
			}

			// Create output array
			arrayType := reflect.ArrayOf(expectedLen, elType)
			decArrayPtr := reflect.New(arrayType)
			decArray := reflect.Indirect(decArrayPtr)
			for i := 0; i < expectedLen; i++ {
				// Ensure we can set the ith entry of the output array
				arrayEntry := decArray.Index(i)
				if !arrayEntry.CanSet() {
					return fmt.Errorf("Decode cannot call Set() on %v[%d]", structField.Name, i)
				}

				// Set the entry at i to the decoded byte
				arrayEntry.Set(reflect.ValueOf(dec[i]))
			}

			// Set the value to be the decoded byte array
			fieldValue.Set(decArray)
		case reflect.Slice:
			// Got a slice, check if it's a slice of bytes or structs
			elType := structField.Type.Elem()
			elKind := elType.Kind()

			switch elKind {
			case reflect.Uint8:
				// Decode slice of byte or uint8
				var dec []byte
				dec, err = decodeByteSlice(data, parsedField.parsedStructTag.MaxLen)
				if err != nil {
					return fmt.Errorf("error decoding '%s': %s", expectedName, err)
				}

				// Set the value to be the decoded byte slice
				fieldValue.SetBytes(dec)
			case reflect.Struct:
				// Decode header
				var length uint32
				length, err = decodeCommonHeader(data, PackArrayID)
				if err != nil {
					return fmt.Errorf("error decoding '%s': %s", expectedName, err)
				}

				// Enforce maximum length
				err = checkMaxLength(length, parsedField.parsedStructTag.MaxLen)
				if err != nil {
					return fmt.Errorf("error decoding '%s': %s", expectedName, err)
				}

				// Make sure it's safe to cast to int on 32-bit platforms
				if length > math.MaxInt32 {
					return fmt.Errorf("error decoding '%s': length %d overflows 32-bit signed integers", expectedName, length)
				}

				// This cast is OK because we just checked that length <= math.MaxInt32
				ilen := int(length)

				// Create slice of structs
				dec := reflect.MakeSlice(structField.Type, ilen, ilen)
				for i := 0; i < ilen; i++ {
					// Ensure we can make a pointer to this slice entry
					sliceEntry := dec.Index(i)
					if !sliceEntry.CanAddr() {
						return fmt.Errorf("Decode cannot call Addr() on %v[%d]", structField.Name, i)
					}

					// Make a pointer to this slice entry
					entryAddr := sliceEntry.Addr()

					// Ensure we can convert the pointer to this slice entry to an
					// interface, which we need for decodeStruct
					if !entryAddr.CanInterface() {
						return fmt.Errorf("could not convert &%v[%d] to interface", structField.Name, i)
					}

					// Decode struct in place
					err = decodeStruct(data, entryAddr.Interface())
					if err != nil {
						return fmt.Errorf("error decoding '%s': %s", expectedName, err)
					}
				}

				// Set the value to be the decoded struct slice
				fieldValue.Set(dec)
			default:
				return fmt.Errorf("can only decode slices of byte, uint8, or struct, not %s", elKind)
			}

		case reflect.String:
			// Decode string
			var dec string
			dec, err = decodeString(data, parsedField.parsedStructTag.MaxLen)
			if err != nil {
				return fmt.Errorf("error decoding '%s': %s", expectedName, err)
			}

			// Set the value to be the decoded string
			fieldValue.SetString(dec)
		case reflect.Uint64:
			// Decode uint64
			var dec uint64
			dec, err = decodeUint64(data)
			if err != nil {
				return fmt.Errorf("error decoding '%s': %s", expectedName, err)
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

			// Ensure we can convert the pointer to this field to an interface, which
			// we need for decodeStruct
			if !valueAddr.CanInterface() {
				return fmt.Errorf("could not convert %s to interface", structField.Name)
			}

			// Decode struct
			err = decodeStruct(data, valueAddr.Interface())
			if err != nil {
				return fmt.Errorf("error decoding '%s': %s", expectedName, err)
			}
		default:
			return fmt.Errorf("decode does not know how to decode into %s", kind)
		}
	}

	return
}
