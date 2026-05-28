package ptrx

// Deref safely dereferences a pointer. If pointer is nil, returns default value,
// otherwise returns dereferenced value.
func Deref[T any](v *T, defaultValue T) T {
	if v != nil {
		return *v
	}
	return defaultValue
}
