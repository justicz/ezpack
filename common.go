package encoding

import (
	"fmt"
	"math"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// maxTagFieldNameLength is the maximum allowed length of a field name
// specified in a struct tag
const maxTagFieldNameLength = 32

// ezPackStructTag contains the parsed out values from a struct tag
type ezPackStructTag struct {
	FieldName string
	MaxLen    uint32
}

// tagRegex matches what we're looking for in a struct tag
// (currently just the encoded field name)
var tagRegex = regexp.MustCompile(`ezpack:"(\w+)(,(\d+))?".*`)

// parseStructTag parses the struct tag on a particular field into
// an ezPackStructTag containing the tag's specified parameters
func parseStructTag(st reflect.StructTag, goFieldName string) (ezst ezPackStructTag, err error) {
	// Parse out field name
	m := tagRegex.FindSubmatch([]byte(st))

	// Ensure we match the ezpack struct tag regex
	if len(m) != 4 {
		err = fmt.Errorf("valid ezpack struct tag required on '%s'", goFieldName)
		return
	}

	// Check that the encoded field name is not too long
	encFieldName := string(m[1])
	if len(encFieldName) > maxTagFieldNameLength {
		err = fmt.Errorf("field name too long on '%s', max %d", goFieldName, maxTagFieldNameLength)
	}

	// Fill in parsed name
	ezst.FieldName = encFieldName

	// Fill in parsed max length, default to 0
	var maxLen uint64
	if len(m[3]) != 0 {
		maxLen, err = strconv.ParseUint(string(m[3]), 10, 32)
		if err != nil {
			err = fmt.Errorf("error parsing max len for '%s': %s", ezst.FieldName, err)
			return
		}
	}

	// Check max len won't cause problems for 32-bit system ints and copy in
	if maxLen > math.MaxInt32 {
		err = fmt.Errorf("max len for '%s' too long: max is %d", ezst.FieldName, math.MaxInt32)
		return
	}
	ezst.MaxLen = uint32(maxLen)

	return
}

// parsedStructField stores a parsed struct tag and its struct offset in a
// convenient struct for sorting
type parsedStructField struct {
	offset          int
	parsedStructTag ezPackStructTag
}

// sortStructFields returns a sorted slice of parsedStructFields, containing
// the field offset and expected name on the wire for each field. t.Kind()
// must be reflect.Struct
func sortStructFields(t reflect.Type) ([]parsedStructField, error) {
	// Ensure we were passed a struct
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("can only sort struct fields, not %s", t.Kind())
	}

	// Ensure we are not dealing with a massive struct
	numFields := t.NumField()
	if numFields < 0 || numFields > math.MaxInt32 {
		return nil, fmt.Errorf("cannot decode massive struct")
	}

	// Build slice of parsed struct fields for sorting
	parsedFields := make([]parsedStructField, 0, numFields)
	for i := 0; i < numFields; i++ {
		// Fetch the field
		field := t.Field(i)

		// Parse the struct tag
		pstag, err := parseStructTag(field.Tag, field.Name)
		if err != nil {
			return nil, err
		}

		// Fill in and append information about this field we'll use for sorting
		psfield := parsedStructField{
			offset:          i,
			parsedStructTag: pstag,
		}
		parsedFields = append(parsedFields, psfield)
	}

	// Sort struct fields by name on the wire
	sort.Slice(parsedFields, func(i, j int) bool {
		iName := parsedFields[i].parsedStructTag.FieldName
		jName := parsedFields[j].parsedStructTag.FieldName
		r := strings.Compare(iName, jName)
		return r == -1
	})

	// Do a scan to check for duplicate names (which are now sorted)
	for i := range parsedFields {
		if i == 0 {
			// Skip first element (still correct if only one)
			continue
		}
		// Compare this element with the last and ensure they are not equal
		fname := parsedFields[i].parsedStructTag.FieldName
		lname := parsedFields[i-1].parsedStructTag.FieldName
		if fname == lname {
			return nil, fmt.Errorf("found duplicate key '%s'", fname)
		}
	}

	return parsedFields, nil
}
