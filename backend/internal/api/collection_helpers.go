package api

func groupByStringKey[T any](items []T, keyFn func(T) string) map[string][]T {
	result := make(map[string][]T, len(items))
	for _, item := range items {
		key := keyFn(item)
		result[key] = append(result[key], item)
	}
	return result
}

func indexByStringKey[T any](items []T, keyFn func(T) string) map[string]T {
	result := make(map[string]T, len(items))
	for _, item := range items {
		result[keyFn(item)] = item
	}
	return result
}

func firstByStringKey[T any](items []T, keyFn func(T) string) map[string]T {
	result := make(map[string]T, len(items))
	for _, item := range items {
		key := keyFn(item)
		if _, exists := result[key]; exists {
			continue
		}
		result[key] = item
	}
	return result
}

func nestedIndexByStringKey[T any](items []T, outerKeyFn func(T) string, innerKeyFn func(T) string) map[string]map[string]T {
	result := make(map[string]map[string]T, len(items))
	for _, item := range items {
		outerKey := outerKeyFn(item)
		if _, ok := result[outerKey]; !ok {
			result[outerKey] = map[string]T{}
		}
		result[outerKey][innerKeyFn(item)] = item
	}
	return result
}
