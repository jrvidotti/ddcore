package engine

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// maxRandomBytes and maxRandomChars bound what one call may ask of the CSPRNG.
const (
	maxRandomBytes = 1024
	maxRandomChars = 1 << 16
)

// secureRandomString returns n characters of the lower-case alphanumeric
// alphabet from crypto/rand. Bytes at or above the largest multiple of the
// alphabet size are thrown away, so every character is equally likely
// (`b % 36` alone would favour the first 4 characters).
func secureRandomString(n int) string {
	limit := byte(256 - 256%len(alphabet))
	out := make([]byte, 0, n)
	buf := make([]byte, n+n/4+8)
	for len(out) < n {
		rand.Read(buf)
		for _, b := range buf {
			if b < limit {
				out = append(out, alphabet[int(b)%len(alphabet)])
				if len(out) == n {
					break
				}
			}
		}
	}
	return string(out)
}

// encodeDigest renders bytes as hex (the default), base64 or base64url
// without padding.
func encodeDigest(b []byte, output string) (string, error) {
	switch output {
	case "", "hex":
		return hex.EncodeToString(b), nil
	case "base64":
		return base64.StdEncoding.EncodeToString(b), nil
	case "base64url":
		return base64.RawURLEncoding.EncodeToString(b), nil
	}
	return "", cerr.Validation("crypto: output must be \"hex\", \"base64\" or \"base64url\"")
}

func optString(o map[string]any, k string) string {
	s, _ := o[k].(string)
	return s
}

// hmacSha256Op is `ddcore.crypto.hmacSha256`: the key is read as UTF-8 unless
// keyEncoding says it is base64 or hex.
func hmacSha256Op(key, text string, opts map[string]any) (string, error) {
	var k []byte
	switch enc := optString(opts, "keyEncoding"); enc {
	case "", "utf8":
		k = []byte(key)
	case "base64":
		b, err := base64.StdEncoding.DecodeString(key)
		if err != nil {
			b, err = base64.RawStdEncoding.DecodeString(key)
		}
		if err != nil {
			return "", cerr.Validation("crypto: key is not valid base64")
		}
		k = b
	case "hex":
		b, err := hex.DecodeString(key)
		if err != nil {
			return "", cerr.Validation("crypto: key is not valid hex")
		}
		k = b
	default:
		return "", cerr.Validation("crypto: keyEncoding must be \"utf8\", \"base64\" or \"hex\"")
	}
	mac := hmac.New(sha256.New, k)
	mac.Write([]byte(text))
	return encodeDigest(mac.Sum(nil), optString(opts, "output"))
}

func sha256Op(text string, opts map[string]any) (string, error) {
	sum := sha256.Sum256([]byte(text))
	return encodeDigest(sum[:], optString(opts, "output"))
}

// optInt reads an integer option that JSON delivered as a float64.
func optInt(o map[string]any, k string) (int64, bool) {
	f, ok := o[k].(float64)
	if !ok || f != math.Trunc(f) || math.Abs(f) > 1<<53-1 {
		return 0, false
	}
	return int64(f), true
}

func randomTokenOp(opts map[string]any) (string, error) {
	n, ok := optInt(opts, "bytes")
	if !ok || n < 1 || n > maxRandomBytes {
		return "", cerr.Validation("crypto: bytes must be an integer from 1 to {0}", maxRandomBytes)
	}
	b := make([]byte, n)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func randomIntOp(opts map[string]any) (int64, error) {
	lo, ok1 := optInt(opts, "min")
	hi, ok2 := optInt(opts, "max")
	if !ok1 || !ok2 {
		return 0, cerr.Validation("crypto: min and max must be safe integers")
	}
	if hi <= lo {
		return 0, cerr.Validation("crypto: max must be greater than min")
	}
	n, err := rand.Int(rand.Reader, big.NewInt(hi-lo))
	if err != nil {
		return 0, cerr.Internal("invalid arguments in {0}: {1}", "crypto.randomInt", err)
	}
	return lo + n.Int64(), nil
}

func randomStringOp(opts map[string]any) (string, error) {
	n, ok := optInt(opts, "n")
	if !ok || n < 0 || n > maxRandomChars {
		return "", cerr.Validation("crypto: n must be an integer from 0 to {0}", maxRandomChars)
	}
	return secureRandomString(int(n)), nil
}

// VerifyWebhook checks a Standard Webhooks request the way SignWebhook signed
// it: headers (any case) carry webhook-id, webhook-timestamp and one or more
// space-separated `v1,<base64>` signatures, any of which may match. A missing
// header, an unreadable timestamp or one further than tolerance from now is
// false, never an error.
//
// An empty secret, or a whsec_ one that decodes to no bytes, verifies nothing:
// the key would be known to everyone. tolerance is in seconds, compared as a
// float so that a huge value (to switch the window off) cannot overflow.
func VerifyWebhook(secret string, headers map[string]string, body []byte, tolerance float64, now time.Time) bool {
	if secret == "" {
		return false
	}
	if rest, ok := strings.CutPrefix(secret, "whsec_"); ok {
		if k, err := base64.StdEncoding.DecodeString(rest); err == nil && len(k) == 0 {
			return false
		}
	}
	h := map[string]string{}
	for k, v := range headers {
		h[strings.ToLower(k)] = v
	}
	id, tsRaw, sigs := h["webhook-id"], h["webhook-timestamp"], h["webhook-signature"]
	if id == "" || tsRaw == "" || sigs == "" {
		return false
	}
	ts, err := strconv.ParseInt(strings.TrimSpace(tsRaw), 10, 64)
	if err != nil {
		return false
	}
	if math.Abs(float64(now.Unix())-float64(ts)) > tolerance {
		return false
	}
	want := []byte(SignWebhook(secret, id, ts, body))
	ok := false
	for _, s := range strings.Fields(sigs) {
		// no early exit: every candidate costs the same
		if subtle.ConstantTimeCompare([]byte(s), want) == 1 {
			ok = true
		}
	}
	return ok
}
