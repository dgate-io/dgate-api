package util

import (
	"bytes"
	"io"
	"net/http"
	"strconv"

	"github.com/dgate-io/dgate-api/pkg/modules"
)

type UtilModule struct {
	modCtx modules.RuntimeContext
}

var _ modules.GoModule = &UtilModule{}

func New(modCtx modules.RuntimeContext) modules.GoModule {
	return &UtilModule{modCtx}
}

func (um *UtilModule) Exports() *modules.Exports {
	return &modules.Exports{
		Named: map[string]any{
			"readWriteBody": um.ReadWriteBody,
		},
	}
}

func (um *UtilModule) ReadWriteBody(res *http.Response, callback func(string) string) string {
	if callback == nil {
		return ""
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		panic(err)
	}
	err = res.Body.Close()
	if err != nil {
		panic(err)
	}
	newBody := callback(string(body))
	res.Body = io.NopCloser(bytes.NewReader([]byte(newBody)))
	res.ContentLength = int64(len(newBody))
	res.Header.Set("Content-Length", strconv.Itoa(len(newBody)))
	return string(newBody)
}
