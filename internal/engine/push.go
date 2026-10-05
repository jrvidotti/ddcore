package engine

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// ddcore.push sends a Web Push message (RFC 8030) to one browser
// subscription: the payload encrypted for that subscription (RFC 8291,
// aes128gcm) and the request signed with the site's VAPID key (RFC 8292).
// The standard library has every primitive this takes.
//
// The keys are the site's, read from the environment like any integration
// secret: DDCORE_SECRET_VAPID_PUBLIC_KEY, _PRIVATE_KEY and _SUBJECT, in the
// form the `web-push` tooling and `ddcore push keys` print.

const (
	// pushRecordSize is the rs of the single record a message is sent in.
	pushRecordSize = 4096
	// pushHeaderSize is the aes128gcm header: salt, rs, idlen and the
	// application server's uncompressed public key as keyid.
	pushHeaderSize = 16 + 4 + 1 + 65
	// pushMaxPlaintext is what fits in a 4096-byte body once the header, the
	// GCM tag and the 0x02 delimiter are counted: a push service is only
	// required to accept that much.
	pushMaxPlaintext = pushRecordSize - pushHeaderSize - 16 - 1
	// pushDefaultTTL is how long a push service keeps a message for an
	// offline browser when the call names no ttl: a day.
	pushDefaultTTL = 86400
	// vapidValidity is how far ahead a VAPID token expires; RFC 8292 caps it
	// at 24 hours.
	vapidValidity = 12 * time.Hour
	// pushMaxResponse is how much of a push service's answer is kept: an
	// answer is a status and at most a short error text.
	pushMaxResponse = 64 << 10
)

// pushHTTP is the push client: its own timeout, apart from ddcore.http's.
// An endpoint comes from a browser, so a redirect is answered, not followed:
// following it would let whoever subscribed point the server elsewhere.
var pushHTTP = &http.Client{
	Timeout:       15 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// pushSendArgs is what ddcore.push.send hands the host. The prelude turns a
// payload that is not a string into JSON before it gets here.
type pushSendArgs struct {
	Subscription struct {
		Endpoint string `json:"endpoint"`
		Keys     struct {
			P256dh string `json:"p256dh"`
			Auth   string `json:"auth"`
		} `json:"keys"`
	} `json:"subscription"`
	Payload string `json:"payload"`
	Opts    struct {
		TTL     *float64 `json:"ttl"`
		Urgency string   `json:"urgency"`
		Topic   string   `json:"topic"`
	} `json:"opts"`
}

// GenerateVAPIDKeys returns a new VAPID pair as base64url: the 65-byte
// uncompressed public key and the 32-byte private scalar.
func GenerateVAPIDKeys() (public, private string, err error) {
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return "", "", err
	}
	enc := base64.RawURLEncoding
	return enc.EncodeToString(key.PublicKey().Bytes()), enc.EncodeToString(key.Bytes()), nil
}

// decodeB64URL reads base64url with or without padding, which is how both
// the browser and the web-push tooling write keys.
func decodeB64URL(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(strings.TrimRight(strings.TrimSpace(s), "="))
}

// vapidKey reads the site's VAPID pair and checks that the public key is the
// private key's half: a browser subscribed with the public one, and a push
// service checks the signature against it. No error carries a key.
func vapidKey(public, private string) (*ecdsa.PrivateKey, error) {
	priv, err := decodeB64URL(private)
	if err != nil {
		return nil, cerr.Validation("push: {0} is not a VAPID key (base64url, as ddcore push keys prints it)", SecretEnvName("vapid_private_key"))
	}
	key, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), priv)
	if err != nil {
		return nil, cerr.Validation("push: {0} is not a VAPID key (base64url, as ddcore push keys prints it)", SecretEnvName("vapid_private_key"))
	}
	pub, err := decodeB64URL(public)
	if err != nil || len(pub) != 65 {
		return nil, cerr.Validation("push: {0} is not a VAPID key (base64url, as ddcore push keys prints it)", SecretEnvName("vapid_public_key"))
	}
	own, err := key.PublicKey.Bytes()
	if err != nil || !bytes.Equal(own, pub) {
		return nil, cerr.Validation("push: the VAPID public and private keys are not a pair")
	}
	return key, nil
}

// vapidJWT is the ES256 token of RFC 8292: the push service's origin as
// audience, signed as the raw r||s JWS asks for rather than ASN.1.
func vapidJWT(key *ecdsa.PrivateKey, aud, sub string, exp time.Time) (string, error) {
	enc := base64.RawURLEncoding
	claims, err := json.Marshal(map[string]any{"aud": aud, "exp": exp.Unix(), "sub": sub})
	if err != nil {
		return "", err
	}
	signed := enc.EncodeToString([]byte(`{"typ":"JWT","alg":"ES256"}`)) + "." + enc.EncodeToString(claims)
	hash := sha256.Sum256([]byte(signed))
	r, s, err := ecdsa.Sign(rand.Reader, key, hash[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signed + "." + enc.EncodeToString(sig), nil
}

// pushEncrypt is the aes128gcm body of RFC 8291 for one subscription, in a
// single record. The application server's ephemeral key and the salt are
// arguments so a test can fix them; a send makes both fresh.
func pushEncrypt(plaintext, uaPublic, authSecret []byte, as *ecdh.PrivateKey, salt []byte) ([]byte, error) {
	if len(plaintext) > pushMaxPlaintext {
		return nil, cerr.Validation("push: the payload is {0} bytes, more than the {1} a push message carries", len(plaintext), pushMaxPlaintext)
	}
	ua, err := ecdh.P256().NewPublicKey(uaPublic)
	if err != nil {
		return nil, cerr.Validation("push: subscription.keys.p256dh is not a P-256 public key")
	}
	if len(authSecret) != 16 {
		return nil, cerr.Validation("push: subscription.keys.auth must be 16 bytes of base64url")
	}
	secret, err := as.ECDH(ua)
	if err != nil {
		return nil, cerr.Validation("push: subscription.keys.p256dh is not a P-256 public key")
	}
	asPublic := as.PublicKey().Bytes()
	// IKM binds the shared secret to the subscription's auth secret and to
	// both public keys; the content key and nonce come from it and the salt.
	keyInfo := "WebPush: info\x00" + string(uaPublic) + string(asPublic)
	ikm, err := hkdf.Key(sha256.New, secret, authSecret, keyInfo, 32)
	if err != nil {
		return nil, err
	}
	cek, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: aes128gcm\x00", 16)
	if err != nil {
		return nil, err
	}
	nonce, err := hkdf.Key(sha256.New, ikm, salt, "Content-Encoding: nonce\x00", 12)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(cek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	body := make([]byte, 0, pushHeaderSize+len(plaintext)+1+gcm.Overhead())
	body = append(body, salt...)
	body = binary.BigEndian.AppendUint32(body, pushRecordSize)
	body = append(body, byte(len(asPublic)))
	body = append(body, asPublic...)
	// the only record is the last one: delimiter 0x02, no padding
	record := append(append(make([]byte, 0, len(plaintext)+1), plaintext...), 0x02)
	return gcm.Seal(body, nonce, record, nil), nil
}

// pushTopic is RFC 8030's Topic: at most 32 characters of the base64url
// alphabet.
func pushTopic(s string) bool {
	if len(s) > 32 {
		return false
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_') {
			return false
		}
	}
	return true
}

// PushSend is ddcore.push.send. Whatever the push service answers is
// returned rather than thrown: a 404 or 410 means the subscription is gone,
// and deleting it is the app's decision. Only a request that never got an
// answer is an error.
func (e *Engine) PushSend(a pushSendArgs) (map[string]any, error) {
	ttl := float64(pushDefaultTTL)
	if a.Opts.TTL != nil {
		ttl = *a.Opts.TTL
	}
	if ttl < 0 || ttl != math.Trunc(ttl) || ttl > math.MaxInt32 {
		return nil, cerr.Validation("push: opts.ttl must be a whole number of seconds, 0 or more")
	}
	switch a.Opts.Urgency {
	case "", "very-low", "low", "normal", "high":
	default:
		return nil, cerr.Validation("push: opts.urgency must be very-low, low, normal or high, not {0}", a.Opts.Urgency)
	}
	if !pushTopic(a.Opts.Topic) {
		return nil, cerr.Validation("push: opts.topic must be at most 32 characters of the base64url alphabet")
	}
	endpoint, err := url.Parse(a.Subscription.Endpoint)
	if err != nil || (endpoint.Scheme != "https" && endpoint.Scheme != "http") || endpoint.Host == "" {
		return nil, cerr.Validation("push: subscription.endpoint must be an http(s) URL")
	}
	// Every push service is https. Plain http is for a receiver on this
	// machine or for development, the rule webhooks follow: the endpoint is
	// whatever a browser sent, and must not reach the site's own network.
	if endpoint.Scheme == "http" && !e.Cfg.Dev && !isLoopbackHost(endpoint.Hostname()) {
		return nil, cerr.Validation("push: subscription.endpoint must use https outside development")
	}
	uaPublic, err := decodeB64URL(a.Subscription.Keys.P256dh)
	if err != nil {
		return nil, cerr.Validation("push: subscription.keys.p256dh is not a P-256 public key")
	}
	auth, err := decodeB64URL(a.Subscription.Keys.Auth)
	if err != nil {
		return nil, cerr.Validation("push: subscription.keys.auth must be 16 bytes of base64url")
	}

	public, err := e.RequireSecret("vapid_public_key")
	if err != nil {
		return nil, err
	}
	private, err := e.RequireSecret("vapid_private_key")
	if err != nil {
		return nil, err
	}
	subject, err := e.RequireSecret("vapid_subject")
	if err != nil {
		return nil, err
	}
	// RFC 8292 asks for a contact; a push service may refuse anything else
	if !strings.HasPrefix(subject, "mailto:") && !strings.HasPrefix(subject, "https:") {
		return nil, cerr.Validation("push: {0} must be a mailto: or https: URL", SecretEnvName("vapid_subject"))
	}
	key, err := vapidKey(public, private)
	if err != nil {
		return nil, err
	}

	ephemeral, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	salt := make([]byte, 16)
	rand.Read(salt)
	body, err := pushEncrypt([]byte(a.Payload), uaPublic, auth, ephemeral, salt)
	if err != nil {
		return nil, err
	}
	jwt, err := vapidJWT(key, endpoint.Scheme+"://"+endpoint.Host, subject, time.Now().Add(vapidValidity))
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, cerr.Validation("push: {0}", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("TTL", strconv.FormatInt(int64(ttl), 10))
	if a.Opts.Urgency != "" {
		req.Header.Set("Urgency", a.Opts.Urgency)
	}
	if a.Opts.Topic != "" {
		req.Header.Set("Topic", a.Opts.Topic)
	}
	k, err := key.PublicKey.Bytes()
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "vapid t="+jwt+", k="+base64.RawURLEncoding.EncodeToString(k))
	req.Header.Set("User-Agent", "ddcore/0.1")
	res, err := pushHTTP.Do(req)
	if err != nil {
		return nil, cerr.Validation("push: {0}", err)
	}
	defer res.Body.Close()
	// an answer longer than this is cut, not refused: it is a diagnostic
	answer, err := io.ReadAll(io.LimitReader(res.Body, pushMaxResponse))
	if err != nil {
		return nil, cerr.Validation("push: {0}", err)
	}
	return map[string]any{"status": res.StatusCode, "body": string(answer), "headers": flatHeaders(res.Header)}, nil
}
