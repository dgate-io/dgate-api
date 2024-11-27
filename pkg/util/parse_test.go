package util_test

import (
	"strconv"
	"testing"
	"time"

	"github.com/dgate-io/dgate-api/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestParseBase36Timestamp(t *testing.T) {
	originalTime := time.Unix(0, time.Now().UnixNano())
	base36String := strconv.FormatInt(originalTime.UnixNano(), 36)
	if parsedTime, err := util.ParseBase36Timestamp(base36String); err != nil {
		t.Errorf("unexpected error: %v", err)
	} else {
		assert.Equal(t, parsedTime, originalTime)
	}
}
