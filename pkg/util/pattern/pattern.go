package pattern

import (
	"errors"
	"regexp"
	"strings"
)

var (
	ErrEmptyPattern = errors.New("empty pattern")
)

func PatternMatch(value, pattern string) (bool, error) {
	if pattern == "" {
		return false, ErrEmptyPattern
	}
	pattern = singlefyAsterisks(pattern)
	if pattern == "*" {
		return true, nil
	}

	asteriskPrefix := strings.HasPrefix(pattern, "*")
	asteriskSuffix := strings.HasSuffix(pattern, "*")
	if asteriskPrefix && asteriskSuffix {
		return strings.Contains(value, pattern[1:len(pattern)-1]), nil
	} else if asteriskPrefix {
		return strings.HasSuffix(value, pattern[1:]), nil
	} else if asteriskSuffix {
		return strings.HasPrefix(value, pattern[:len(pattern)-1]), nil
	} else if strings.HasPrefix(pattern, "/") && strings.HasSuffix(pattern, "/") {
		re := pattern[1 : len(pattern)-1]
		result, err := regexp.MatchString(re, value)
		return result, err
	}
	return pattern == value, nil
}

func MatchAnyPattern(value string, patterns []string) (string, bool, error) {
	for _, pattern := range patterns {
		match, err := PatternMatch(value, pattern)
		if err != nil {
			return "", false, err
		}
		if match {
			return pattern, true, nil
		}
	}
	return "", false, nil
}

func MatchAllPatterns(value string, patterns []string) (bool, error) {
	for _, pattern := range patterns {
		match, err := PatternMatch(value, pattern)
		if err != nil {
			return false, err
		}
		if !match {
			return false, nil
		}
	}
	return true, nil
}

func singlefyAsterisks(pattern string) string {
	if strings.Contains(pattern, "**") {
		pattern = strings.Replace(pattern, "**", "*", -1)
		return singlefyAsterisks(pattern)
	}
	return pattern
}
