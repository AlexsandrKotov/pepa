package sdk

// Helpers shared by PEPA plugins. They operate on plain map[string]interface{}
// values so they work with both decoded JSON payloads and unstructured
// Kubernetes objects.

// GetNestedString safely extracts a string from nested maps.
// It reports false as soon as any intermediate field is missing or is not a
// nested map, so callers never need to type-assert the intermediate levels.
func GetNestedString(obj map[string]interface{}, fields ...string) (string, bool) {
	var val interface{} = obj
	for _, field := range fields {
		if m, ok := val.(map[string]interface{}); ok {
			val = m[field]
		} else {
			return "", false
		}
	}
	if s, ok := val.(string); ok {
		return s, true
	}
	return "", false
}
