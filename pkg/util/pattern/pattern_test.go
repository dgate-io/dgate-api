package pattern_test

import (
	"strings"
	"testing"

	"github.com/dgate-io/dgate-api/pkg/util/pattern"
	"github.com/stretchr/testify/assert"
)

func TestCheckDomainMatch(t *testing.T) {
	domains := []string{
		"example.com",
		"a.example.com",
		"b.example.com",
		"example.net",
		"a.example.net",
		"b.example.net",
	}
	patterns := map[string][]int{
		"example.com":                {0},
		"example.*":                  {0, 3},
		"example.**":                 {0, 3},
		"*.example.net*":             {4, 5},
		"**.example.net*":            {4, 5},
		"/.+\\.example\\.(com|net)/": {1, 2, 4, 5},
		"*":                          {0, 1, 2, 3, 4, 5},
	}

	for pt, expected := range patterns {
		for i, name := range domains {
			match, err := pattern.PatternMatch(name, pt)
			if err != nil {
				t.Fatal(err)
			}
			if contains(expected, i) {
				if !match {
					t.Fatalf("expected %v to match %v", name, pt)
				}
			} else {
				if match {
					t.Fatalf("expected %v to not match %v", name, pt)
				}
			}
		}
	}
}

func TestDomainMatchAnyPatternError(t *testing.T) {
	var err error
	_, _, err = pattern.MatchAnyPattern("example.com", []string{""})
	if err == nil {
		t.Fail()
	} else {
		assert.Equal(t, pattern.ErrEmptyPattern, err)
	}
	_, _, err = pattern.MatchAnyPattern("example.com", []string{`/\/`})
	if err == nil {
		t.Fail()
	} else {
		assert.True(t, strings.HasPrefix(err.Error(), "error parsing regexp: trailing backslash"))
	}
}

func contains(s []int, e int) bool {
	for _, a := range s {
		if a == e {
			return true
		}
	}
	return false
}

func TestDomainMatchAnyPattern(t *testing.T) {
	domains := []string{
		"example.com",
		"a.example.com",
		"b.example.com",
		"example.net",
		"a.example.net",
		"b.example.net",
	}
	for _, d := range domains {
		_, matches, err := pattern.MatchAnyPattern(d, domains)
		if err != nil {
			t.Fatal(err)
		}
		if !matches {
			t.Fatalf("expected %v to match itself", d)
		}
	}
	_, matches, err := pattern.MatchAnyPattern("test.com", domains)
	if err != nil {
		t.Fatal(err)
	}
	if matches {
		t.Fatalf("expected %v to not match any domain", "test.com")
	}
}

func TestDomainMatchAnyPatternCache(t *testing.T) {
	for i := 0; i < 10; i++ {
		_, matches, err := pattern.MatchAnyPattern(
			"example.com", []string{"example.com"},
		)
		if err != nil {
			t.Fatal(err)
		}
		if !matches {
			t.Fatalf("expected %v to match itself", "example.com")
		}
	}
}
