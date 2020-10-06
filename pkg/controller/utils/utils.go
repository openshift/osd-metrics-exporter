package utils

func RemoveString(finalizers []string, candidate string) []string {
	var result []string
	for _, s := range finalizers {
		if s != candidate {
			result = append(result, s)
		}
	}
	return result
}

func ContainsString(stringArray []string, candidate string) bool {
	for _, s := range stringArray {
		if s == candidate {
			return true
		}
	}
	return false
}
