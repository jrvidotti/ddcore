package engine

import "strings"

func normalizeEmail(value string) string {
	return strings.TrimSpace(value)
}

func validEmail(value string) bool {
	if value == "" {
		return true
	}
	if len(value) > 254 || strings.Count(value, "@") != 1 {
		return false
	}
	local, domain, _ := strings.Cut(value, "@")
	if len(local) == 0 || len(local) > 64 || len(domain) == 0 || len(domain) > 253 || !strings.Contains(domain, ".") {
		return false
	}
	if local[0] == '.' || local[len(local)-1] == '.' || strings.Contains(local, "..") {
		return false
	}
	for _, ch := range local {
		if !isEmailLocalChar(ch) {
			return false
		}
	}
	for _, label := range strings.Split(domain, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, ch := range label {
			if !isASCIIAlphaNumeric(ch) && ch != '-' {
				return false
			}
		}
	}
	return true
}

func isEmailLocalChar(ch rune) bool {
	return isASCIIAlphaNumeric(ch) || strings.ContainsRune(".!#$%&'*+/=?^_`{|}~-", ch)
}

func isASCIIAlphaNumeric(ch rune) bool {
	return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch >= '0' && ch <= '9'
}
