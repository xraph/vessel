package vessel

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

// In is a marker type that should be embedded in structs to indicate
// they are parameter objects. Fields of the struct will be treated as
// dependencies to inject. This follows the dig pattern for constructor injection.
//
// Example:
//
//	type ServiceParams struct {
//	    vessel.In
//
//	    DB     *Database
//	    Logger *Logger           `optional:"true"`
//	    Cache  *Cache            `name:"redis"`
//	    Handlers []http.Handler  `group:"http"`
//	}
type In struct{}

// Out is a marker type that should be embedded in structs to indicate
// they are result objects. Each field of the struct will be registered
// as a separate service. This follows the dig pattern for multi-value returns.
//
// Example:
//
//	type ServiceResult struct {
//	    vessel.Out
//
//	    UserService    *UserService
//	    ProductService *ProductService `name:"products"`
//	    Handler        http.Handler    `group:"http"`
//	}
type Out struct{}

var (
	inType    = reflect.TypeOf(In{})
	outType   = reflect.TypeOf(Out{})
	errorType = reflect.TypeOf((*error)(nil)).Elem()
)

// constructorInfo holds analyzed constructor metadata
type constructorInfo struct {
	fn       reflect.Value
	fnType   reflect.Type
	params   []paramInfo
	results  []resultInfo
	hasError bool
}

// paramInfo describes a constructor parameter
type paramInfo struct {
	typ      reflect.Type
	name     string      // From `name:"..."` tag, empty for type-based lookup
	optional bool        // From `optional:"true"` tag
	group    bool        // From `group:"..."` tag - expects slice type
	groupKey string      // The group name for collection
	index    int         // Position in function parameters or struct field index
	isIn     bool        // Whether this is an In struct (expanded into multiple deps)
	inFields []paramInfo // Expanded fields if isIn is true
}

// resultInfo describes a constructor result
type resultInfo struct {
	typ       reflect.Type
	name      string       // From `name:"..."` tag
	group     string       // From `group:"..."` tag
	index     int          // Position in function results or struct field index
	fieldName string       // The actual struct field name (for Out structs)
	isOut     bool         // Whether this is an Out struct (expanded into multiple results)
	outFields []resultInfo // Expanded fields if isOut is true
}

// analyzeConstructor inspects a constructor function and extracts its dependency
// and result information for automatic resolution.
func analyzeConstructor(constructor any) (*constructorInfo, error) {
	fnValue := reflect.ValueOf(constructor)
	fnType := fnValue.Type()

	if fnType.Kind() != reflect.Func {
		return nil, errors.New("constructor must be a function")
	}

	info := &constructorInfo{
		fn:     fnValue,
		fnType: fnType,
	}

	// Analyze parameters
	for i := 0; i < fnType.NumIn(); i++ {
		paramType := fnType.In(i)
		param, err := analyzeParam(paramType, i)
		if err != nil {
			return nil, fmt.Errorf("parameter %d: %w", i, err)
		}
		info.params = append(info.params, param)
	}

	// Analyze results
	for i := 0; i < fnType.NumOut(); i++ {
		resultType := fnType.Out(i)

		// Check for error return (must be last)
		if resultType.Implements(errorType) {
			if i != fnType.NumOut()-1 {
				return nil, errors.New("error must be the last return value")
			}
			info.hasError = true
			continue
		}

		result, err := analyzeResult(resultType, i)
		if err != nil {
			return nil, fmt.Errorf("result %d: %w", i, err)
		}
		info.results = append(info.results, result)
	}

	if len(info.results) == 0 {
		return nil, errors.New("constructor must return at least one non-error value")
	}

	return info, nil
}

// analyzeParam analyzes a single parameter type
func analyzeParam(t reflect.Type, index int) (paramInfo, error) {
	param := paramInfo{
		typ:   t,
		index: index,
	}

	// Check if it's an In struct
	if isInStruct(t) {
		param.isIn = true
		fields, err := expandInStruct(t)
		if err != nil {
			return param, err
		}
		param.inFields = fields
	}

	return param, nil
}

// analyzeResult analyzes a single result type
func analyzeResult(t reflect.Type, index int) (resultInfo, error) {
	result := resultInfo{
		typ:   t,
		index: index,
	}

	// Check if it's an Out struct
	if isOutStruct(t) {
		result.isOut = true
		fields, err := expandOutStruct(t)
		if err != nil {
			return result, err
		}
		result.outFields = fields
	}

	return result, nil
}

// isInStruct checks if a type embeds vessel.In
func isInStruct(t reflect.Type) bool {
	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return false
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Anonymous && field.Type == inType {
			return true
		}
		// Check embedded structs recursively
		if field.Anonymous && isInStruct(field.Type) {
			return true
		}
	}
	return false
}

// isOutStruct checks if a type embeds vessel.Out
func isOutStruct(t reflect.Type) bool {
	// Handle pointer types
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t.Kind() != reflect.Struct {
		return false
	}

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Anonymous && field.Type == outType {
			return true
		}
		// Check embedded structs recursively
		if field.Anonymous && isOutStruct(field.Type) {
			return true
		}
	}
	return false
}

// expandInStruct expands an In struct into its field dependencies
func expandInStruct(t reflect.Type) ([]paramInfo, error) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	var params []paramInfo

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip the embedded In marker
		if field.Anonymous && (field.Type == inType || isInStruct(field.Type)) {
			continue
		}

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		param := paramInfo{
			typ:   field.Type,
			index: i,
		}

		// Parse struct tags
		if tag := field.Tag.Get("name"); tag != "" {
			param.name = tag
		}

		if tag := field.Tag.Get("optional"); strings.ToLower(tag) == "true" {
			param.optional = true
		}

		if tag := field.Tag.Get("group"); tag != "" {
			param.group = true
			param.groupKey = tag
			// Verify it's a slice type for group injection
			if field.Type.Kind() != reflect.Slice {
				return nil, fmt.Errorf("field %s with group tag must be a slice type", field.Name)
			}
		}

		params = append(params, param)
	}

	return params, nil
}

// expandOutStruct expands an Out struct into its result fields
func expandOutStruct(t reflect.Type) ([]resultInfo, error) {
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	var results []resultInfo

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)

		// Skip the embedded Out marker
		if field.Anonymous && (field.Type == outType || isOutStruct(field.Type)) {
			continue
		}

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		result := resultInfo{
			typ:       field.Type,
			index:     i,
			fieldName: field.Name, // Capture the field name for extraction
		}

		// Parse struct tags
		if tag := field.Tag.Get("name"); tag != "" {
			result.name = tag
		}

		if tag := field.Tag.Get("group"); tag != "" {
			result.group = tag
		}

		results = append(results, result)
	}

	return results, nil
}

// flattenResults returns all results including expanded Out struct fields
func (c *constructorInfo) flattenResults() []resultInfo {
	var flat []resultInfo
	for _, r := range c.results {
		if r.isOut {
			flat = append(flat, r.outFields...)
		} else {
			flat = append(flat, r)
		}
	}
	return flat
}
