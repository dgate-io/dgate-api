package types

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/dgate-io/dgate-api/pkg/spec"
	"github.com/dgate-io/dgate-api/pkg/util"
)

type ResponseWriterWrapper struct {
	rw     spec.ResponseWriterTracker
	req    *http.Request
	status int

	Headers     http.Header
	HeadersSent bool
	Locals      map[string]any
}

func NewResponseWriterWrapper(
	rw http.ResponseWriter,
	req *http.Request,
) *ResponseWriterWrapper {
	rwt := spec.NewResponseWriterTracker(rw)
	return &ResponseWriterWrapper{
		rw:          rwt,
		req:         req,
		Headers:     rw.Header(),
		HeadersSent: rwt.HeadersSent(),
		Locals:      make(map[string]any),
	}
}

type CookieOptions struct {
	Domain   string    `json:"domain"`
	Expires  time.Time `json:"expires"`
	HttpOnly bool      `json:"httpOnly"`
	MaxAge   int       `json:"maxAge"`
	Path     string    `json:"path"`
	Priority string    `json:"priority"`
	Secure   bool      `json:"secure"`
	Signed   bool      `json:"signed"`
	SameSite string    `json:"sameSite"`
}

func (g *ResponseWriterWrapper) Cookie(name string, value string, opts ...*CookieOptions) (*ResponseWriterWrapper, error) {
	if len(opts) > 1 {
		return nil, errors.New("too many auguments")
	}
	cookie := &http.Cookie{
		Name:  name,
		Value: value,
	}
	if len(opts) == 1 {
		opt := opts[0]
		sameSite := http.SameSiteDefaultMode
		switch opt.SameSite {
		case "lax":
			sameSite = http.SameSiteLaxMode
		case "strict":
			sameSite = http.SameSiteStrictMode
		case "none":
			sameSite = http.SameSiteNoneMode
		}
		cookie = &http.Cookie{
			Name:     name,
			Value:    value,
			Domain:   opt.Domain,
			Expires:  opt.Expires,
			HttpOnly: opt.HttpOnly,
			MaxAge:   opt.MaxAge,
			Path:     opt.Path,
			Secure:   opt.Secure,
			SameSite: sameSite,
		}
	}
	http.SetCookie(g.rw, cookie)
	return g, nil
}

func (g *ResponseWriterWrapper) Send(data any) error {
	return g.End(data)
}

// Json sends a JSON response.
func (g *ResponseWriterWrapper) Json(data any) error {
	g.rw.Header().Set("Content-Type", "application/json")
	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return g.End(b)
}

// End sends the response.
func (g *ResponseWriterWrapper) End(data any) error {
	if !g.rw.HeadersSent() {
		if g.status <= 0 {
			g.status = http.StatusOK
		}
		g.rw.WriteHeader(g.status)
		g.HeadersSent = true
	}
	buf, err := util.ToBytes(data)
	if err != nil {
		return err
	}
	_, err = g.rw.Write(buf)
	return err
}

func (g *ResponseWriterWrapper) Redirect(url string) {
	http.Redirect(g.rw, g.req, url, http.StatusTemporaryRedirect)
}

func (g *ResponseWriterWrapper) RedirectPermanent(url string) {
	http.Redirect(g.rw, g.req, url, http.StatusMovedPermanently)
}

func (g *ResponseWriterWrapper) Status(status int) *ResponseWriterWrapper {
	g.status = status
	return g
}

func (g *ResponseWriterWrapper) Location(url string) *ResponseWriterWrapper {
	g.rw.Header().Set("Location", url)
	return g
}

func (g *ResponseWriterWrapper) GetCookies() []*http.Cookie {
	return g.req.Cookies()
}

func (g *ResponseWriterWrapper) GetCookie(name string) *http.Cookie {
	cookies := g.req.Cookies()
	for _, cookie := range cookies {
		if cookie.Name == name {
			return cookie
		}
	}
	return nil
}
