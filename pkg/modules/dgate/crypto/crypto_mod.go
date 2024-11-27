package crypto

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"math/big"
	"strings"

	"github.com/dgate-io/dgate-api/pkg/modules"
	"github.com/dgate-io/dgate-api/pkg/util"
	"github.com/dop251/goja"
	"github.com/google/uuid"
)

type CryptoModule struct {
	modCtx modules.RuntimeContext
}

var _ modules.GoModule = &CryptoModule{}

func New(modCtx modules.RuntimeContext) modules.GoModule {
	return &CryptoModule{modCtx}
}

func (c *CryptoModule) Exports() *modules.Exports {
	// Hash functions
	hashAlgos := map[string]HashFunc{
		"md5":        c.md5,
		"sha1":       c.sha1,
		"sha256":     c.sha256,
		"sha384":     c.sha384,
		"sha512":     c.sha512,
		"sha512_224": c.sha512_224,
		"sha512_256": c.sha512_256,
	}
	// Named exports
	namedExports := map[string]any{
		"createHash":   c.createHash,
		"createHmac":   c.createHmac,
		"createSign":   nil, // TODO: not implemented
		"createVerify": nil, // TODO: not implemented
		"hmac":         c.hmac,
		"randomBytes":  c.randomBytes,
		"randomInt":    c.randomInt,
		"getHashes": func() []string {
			keys := make([]string, 0, len(hashAlgos))
			for k := range hashAlgos {
				keys = append(keys, strings.Replace(k, "_", "-", -1))
			}
			return keys
		},
		"hexEncode": c.hexEncode,
	}

	for k, v := range hashAlgos {
		namedExports[k] = v
	}
	return &modules.Exports{
		Named: namedExports,
	}
}

func (c *CryptoModule) randomBytes(size int) (*goja.ArrayBuffer, error) {
	if size < 1 {
		return nil, errors.New("invalid size")
	}
	bytes := make([]byte, size)
	_, err := rand.Read(bytes)
	if err != nil {
		return nil, err
	}
	ab := c.modCtx.Runtime().NewArrayBuffer(bytes)
	return &ab, nil
}

func (c *CryptoModule) randomInt(call goja.FunctionCall) (int64, error) {
	argLen := len(call.Arguments)
	if argLen <= 0 || argLen >= 3 {
		return 0, errors.New("invalid number of arguments")
	} else if argLen == 1 {
		nBig, err := rand.Int(rand.Reader, big.NewInt(call.Argument(0).ToInteger()))
		if err != nil {
			return 0, err
		}
		return nBig.Int64(), nil
	} else {
		min := call.Argument(0).ToInteger()
		max := call.Argument(1).ToInteger()
		if min > max {
			return 0, errors.New("min must be less than or equal to max")
		}
		nBig, err := rand.Int(rand.Reader, big.NewInt(max-min))
		if err != nil {
			return 0, err
		}
		return min + nBig.Int64(), nil
	}
}

func (c *CryptoModule) randomUUID() string {
	return uuid.New().String()
}

type HashFunc func(data any, encoding string) (any, error)

func (c *CryptoModule) md5(data any, encoding string) (any, error) {
	return c.update("md5", data, encoding)
}

func (c *CryptoModule) sha1(data any, encoding string) (any, error) {
	return c.update("sha1", data, encoding)
}

func (c *CryptoModule) sha256(data any, encoding string) (any, error) {
	return c.update("sha256", data, encoding)
}

func (c *CryptoModule) sha384(data any, encoding string) (any, error) {
	return c.update("sha384", data, encoding)
}

func (c *CryptoModule) sha512(data any, encoding string) (any, error) {
	return c.update("sha512", data, encoding)
}

func (c *CryptoModule) sha512_224(data any, encoding string) (any, error) {
	return c.update("sha512_224", data, encoding)
}

func (c *CryptoModule) sha512_256(data any, encoding string) (any, error) {
	return c.update("sha512_256", data, encoding)
}

func (c *CryptoModule) createHash(algorithm string) (*GojaHash, error) {
	h, err := c.getHash(algorithm)
	if err != nil {
		return nil, err
	}
	return &GojaHash{h, c.modCtx.Runtime()}, nil
}

func (c *CryptoModule) update(alg string, data any, encoding string) (any, error) {
	hash, err := c.createHash(alg)
	if err != nil {
		return nil, err
	}
	if _, err := hash.Update(data); err != nil {
		return nil, fmt.Errorf("%s failed: %w", alg, err)
	}

	return hash.Digest(encoding)
}

func (c *CryptoModule) hexEncode(data any) (string, error) {
	d, err := util.ToBytes(data)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(d), nil
}

func (c *CryptoModule) createHmac(algorithm string, key any) (*GojaHash, error) {
	h, err := c.getHash(algorithm)
	if err != nil {
		return nil, err
	}

	kb, err := util.ToBytes(key)
	if err != nil {
		return nil, err
	}
	hashFunc := func() hash.Hash { return h }
	return &GojaHash{hmac.New(hashFunc, kb), c.modCtx.Runtime()}, nil
}

func (c *CryptoModule) hmac(algorithm string, key, data any, encoding string) (any, error) {
	hash, err := c.createHmac(algorithm, key)
	if err != nil {
		return nil, err
	}
	_, err = hash.Update(data)
	if err != nil {
		return nil, err
	}
	return hash.Digest(encoding)
}

func (c *CryptoModule) getHash(enc string) (hash.Hash, error) {
	switch enc {
	case "md5":
		return md5.New(), nil
	case "sha1":
		return sha1.New(), nil
	case "sha256":
		return sha256.New(), nil
	case "sha384":
		return sha512.New384(), nil
	case "sha512_224":
		return sha512.New512_224(), nil
	case "sha512_256":
		return sha512.New512_256(), nil
	case "sha512":
		return sha512.New(), nil
	default:
		return nil, errors.New("invalid hash algorithm")
	}
}

type GojaHash struct {
	hash hash.Hash
	rt   *goja.Runtime
}

func (h *GojaHash) Update(data any) (*GojaHash, error) {
	d, err := util.ToBytes(data)
	if err != nil {
		return h, err
	}
	_, err = h.hash.Write(d)
	return h, err
}

func (h *GojaHash) Digest(enc string) (any, error) {
	sum := h.hash.Sum(nil)

	switch enc {
	case "hex":
		return hex.EncodeToString(sum), nil
	case "base64", "b64":
		return base64.StdEncoding.
			EncodeToString(sum), nil
	case "base64raw", "b64raw":
		return base64.RawStdEncoding.
			EncodeToString(sum), nil
	case "base64url", "b64url":
		return base64.URLEncoding.
			EncodeToString(sum), nil
	case "base64rawurl", "b64rawurl":
		return base64.RawURLEncoding.
			EncodeToString(sum), nil
	default: // default to 'binary' (same behavior as Node.js)
		ab := h.rt.NewArrayBuffer(sum)
		return &ab, nil
	}
}
