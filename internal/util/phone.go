package util

import (
	"regexp"
	"strings"
)

// NormalizePhone tries to normalize user input into E.164-like format.
func NormalizePhone(raw string) string {
	s := strings.TrimSpace(raw)
	re := regexp.MustCompile(`[^\d\+]+`)
	s = re.ReplaceAllString(s, "")

	if strings.HasPrefix(s, "00") {
		s = "+" + s[2:]
	} else if strings.HasPrefix(s, "0") && len(s) == 11 {
		s = "+98" + s[1:]
	} else if strings.HasPrefix(s, "9") && len(s) == 10 {
		s = "+98" + s
	} else if strings.HasPrefix(s, "98") {
		s = "+" + s
	}
	
	return s
}
