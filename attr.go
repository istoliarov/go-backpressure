package backpressure

// Attr carries optional low-cardinality context for sampling, snapshots, and
// observer callbacks.
type Attr struct {
	Key   string
	Value string
}

// AttrKey creates an attribute from a string key and value.
func AttrKey(key, value string) Attr {
	return Attr{Key: key, Value: value}
}

// AttrValue returns the first value matching key.
func AttrValue(attrs []Attr, key string) (string, bool) {
	for _, attr := range attrs {
		if attr.Key == key {
			return attr.Value, true
		}
	}
	return "", false
}

func cloneAttrs(attrs []Attr) []Attr {
	if len(attrs) == 0 {
		return nil
	}
	cloned := make([]Attr, len(attrs))
	copy(cloned, attrs)
	return cloned
}
