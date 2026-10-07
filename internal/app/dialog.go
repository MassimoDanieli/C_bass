package app

import "strings"

// lines are the paths a helper printed, one to a line; nil if it failed or printed none.
func lines(out []byte, err error) []string {
	if err != nil {
		return nil
	}
	var found []string
	for _, line := range strings.Split(string(out), "\n") {
		if line = strings.TrimRight(line, "\r"); line != "" {
			found = append(found, line)
		}
	}
	return found
}
