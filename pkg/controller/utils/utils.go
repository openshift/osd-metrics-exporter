package utils


func ContainsString(stringArray []string, candidate string) bool {
	for _, s := range stringArray {
		if s == candidate {
			return true
		}
	}
	return false
}
