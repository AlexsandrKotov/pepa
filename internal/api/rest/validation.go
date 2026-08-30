package rest

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

// ValidationError represents a single field validation error.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationErrors is a collection of validation errors.
type ValidationErrors []ValidationError

// Error implements the error interface.
func (ve ValidationErrors) Error() string {
	if len(ve) == 0 {
		return "validation failed"
	}
	messages := make([]string, len(ve))
	for i, e := range ve {
		messages[i] = fmt.Sprintf("%s: %s", e.Field, e.Message)
	}
	return strings.Join(messages, "; ")
}

// bindAndValidate binds JSON request body to a struct and validates it.
// Returns the populated struct and true if successful, or nil and false if validation failed.
// On failure, it automatically writes the error response to the gin context.
func bindAndValidate[T any](c *gin.Context) (*T, bool) {
	var req T
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid request body",
			"details": err.Error(),
		})
		return nil, false
	}

	// Validate struct fields
	if errors := validateStruct(&req); len(errors) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "validation failed",
			"details": errors,
		})
		return nil, false
	}

	return &req, true
}

// validateStruct validates a struct based on its tags.
// Supported tags:
//   - `binding:"required"` - field must not be empty
//   - `binding:"min=N"` - minimum length for strings, minimum value for numbers
//   - `binding:"max=N"` - maximum length for strings, maximum value for numbers
func validateStruct(s interface{}) ValidationErrors {
	var errors ValidationErrors
	v := reflect.ValueOf(s)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}

	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)
		tag := fieldType.Tag.Get("binding")
		if tag == "" {
			continue
		}

		fieldName := fieldType.Name
		jsonTag := fieldType.Tag.Get("json")
		if jsonTag != "" {
			parts := strings.Split(jsonTag, ",")
			if parts[0] != "" {
				fieldName = parts[0]
			}
		}

		rules := strings.Split(tag, ",")
		for _, rule := range rules {
			rule = strings.TrimSpace(rule)
			if rule == "" {
				continue
			}

			if rule == "required" {
				if isZero(field) {
					errors = append(errors, ValidationError{
						Field:   fieldName,
						Message: "is required",
					})
					break
				}
			} else if strings.HasPrefix(rule, "min=") {
				minStr := strings.TrimPrefix(rule, "min=")
				var min int
				_, _ = fmt.Sscanf(minStr, "%d", &min)

				if field.Kind() == reflect.String {
					if utf8.RuneCountInString(field.String()) < min {
						errors = append(errors, ValidationError{
							Field:   fieldName,
							Message: fmt.Sprintf("must be at least %d characters", min),
						})
					}
				} else if field.Kind() >= reflect.Int && field.Kind() <= reflect.Int64 {
					if field.Int() < int64(min) {
						errors = append(errors, ValidationError{
							Field:   fieldName,
							Message: fmt.Sprintf("must be at least %d", min),
						})
					}
				}
			} else if strings.HasPrefix(rule, "max=") {
				maxStr := strings.TrimPrefix(rule, "max=")
				var max int
				_, _ = fmt.Sscanf(maxStr, "%d", &max)

				if field.Kind() == reflect.String {
					if utf8.RuneCountInString(field.String()) > max {
						errors = append(errors, ValidationError{
							Field:   fieldName,
							Message: fmt.Sprintf("must be at most %d characters", max),
						})
					}
				} else if field.Kind() >= reflect.Int && field.Kind() <= reflect.Int64 {
					if field.Int() > int64(max) {
						errors = append(errors, ValidationError{
							Field:   fieldName,
							Message: fmt.Sprintf("must be at most %d", max),
						})
					}
				}
			}
		}
	}

	return errors
}

// isZero checks if a reflect.Value is the zero value for its type.
func isZero(v reflect.Value) bool {
	switch v.Kind() {
	case reflect.String:
		return v.String() == ""
	case reflect.Bool:
		return !v.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return v.Int() == 0
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return v.Uint() == 0
	case reflect.Float32, reflect.Float64:
		return v.Float() == 0
	case reflect.Interface, reflect.Ptr:
		return v.IsNil()
	case reflect.Slice, reflect.Map:
		return v.IsNil() || v.Len() == 0
	}
	return false
}
