package util_test

import (
	"math"
	"net/http"
	"testing"

	"github.com/dgate-io/dgate-api/pkg/util"
	"github.com/stretchr/testify/assert"
)

func TestGetTrustedIP_Depth(t *testing.T) {
	req := requestWithXForwardedFor(t, "1.2.3.4", "1.2.3.5", "1.2.3.6")

	t.Run("Depth 0", func(t *testing.T) {
		assert.Equal(t, util.GetTrustedIP(req, 0), "127.0.0.1")
	})

	t.Run("Depth 1", func(t *testing.T) {
		assert.Equal(t, util.GetTrustedIP(req, 1), "1.2.3.6")
	})

	t.Run("Depth 2", func(t *testing.T) {
		assert.Equal(t, util.GetTrustedIP(req, 2), "1.2.3.5")
	})

	t.Run("Depth 3", func(t *testing.T) {
		assert.Equal(t, util.GetTrustedIP(req, 3), "1.2.3.4")
	})

	t.Run("Depth too High", func(t *testing.T) {
		assert.Equal(t, util.GetTrustedIP(req, 4), "1.2.3.4")
		assert.Equal(t, util.GetTrustedIP(req, 8), "1.2.3.4")
		assert.Equal(t, util.GetTrustedIP(req, 16), "1.2.3.4")
	})

	t.Run("Depth too Low", func(t *testing.T) {
		assert.Equal(t, util.GetTrustedIP(req, -1), "127.0.0.1")
		assert.Equal(t, util.GetTrustedIP(req, -10), "127.0.0.1")
		assert.Equal(t, util.GetTrustedIP(req, math.MinInt), "127.0.0.1")
	})
}

func requestWithXForwardedFor(t *testing.T, ips ...string) *http.Request {
	req, err := http.NewRequest("GET", "http://localhost:8080", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.RemoteAddr = "127.0.0.1"
	for _, ip := range ips {
		req.Header.Add("X-Forwarded-For", ip)
	}
	return req
}
