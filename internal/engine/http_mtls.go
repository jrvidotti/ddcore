package engine

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"net/http"
	"strings"
	"sync"

	pkcs12 "software.sslmate.com/src/go-pkcs12"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// httpClientCert is opts.clientCert of ddcore.http: a PKCS#12 file, base64
// encoded, with its password, or a PEM certificate and key.
type httpClientCert struct {
	Pfx      string `json:"pfx"`
	Password string `json:"password"`
	Cert     string `json:"cert"`
	Key      string `json:"key"`
}

// httpClientCertTransports is how many transports clientCertTransport keeps.
// A site talks to a handful of mTLS APIs; the cap only bounds a caller that
// passes a different certificate on every call.
const httpClientCertTransports = 32

// httpRootCAs is the pool a client-certificate transport verifies the server
// against. It is nil outside the tests, which is the system roots.
var httpRootCAs *x509.CertPool

var clientCertCache = struct {
	sync.Mutex
	tick  uint64
	items map[[sha256.Size]byte]*clientCertEntry
}{items: map[[sha256.Size]byte]*clientCertEntry{}}

type clientCertEntry struct {
	transport *http.Transport
	used      uint64
}

// clientCertTransport returns the transport that presents cc, the same one
// for the same material: consecutive calls reuse its connections instead of
// repeating the handshake. Its errors never carry the material.
func clientCertTransport(cc *httpClientCert) (*http.Transport, error) {
	isPfx, isPem := cc.Pfx != "", cc.Cert != "" && cc.Key != ""
	if isPfx == isPem || isPfx && (cc.Cert != "" || cc.Key != "") {
		return nil, cerr.Validation("http: clientCert takes either pfx or cert and key")
	}
	h := sha256.New()
	for _, s := range []string{cc.Pfx, cc.Password, cc.Cert, cc.Key} {
		// length-prefixed, so two fields never read as one
		binary.Write(h, binary.BigEndian, uint64(len(s)))
		h.Write([]byte(s))
	}
	var key [sha256.Size]byte
	h.Sum(key[:0])

	c := &clientCertCache
	c.Lock()
	defer c.Unlock()
	c.tick++
	if e, ok := c.items[key]; ok {
		e.used = c.tick
		return e.transport, nil
	}
	cert, err := parseClientCert(cc, isPfx)
	if err != nil {
		return nil, err
	}
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.TLSClientConfig = &tls.Config{Certificates: []tls.Certificate{cert}, RootCAs: httpRootCAs}
	if len(c.items) >= httpClientCertTransports {
		var oldest [sha256.Size]byte
		first := true
		for k, e := range c.items {
			if first || e.used < c.items[oldest].used {
				oldest, first = k, false
			}
		}
		c.items[oldest].transport.CloseIdleConnections()
		delete(c.items, oldest)
	}
	c.items[key] = &clientCertEntry{transport: t, used: c.tick}
	return t, nil
}

func parseClientCert(cc *httpClientCert, isPfx bool) (tls.Certificate, error) {
	if !isPfx {
		cert, err := tls.X509KeyPair([]byte(cc.Cert), []byte(cc.Key))
		if err != nil {
			return tls.Certificate{}, cerr.Validation("http: clientCert.cert and clientCert.key are not a valid PEM pair")
		}
		return cert, nil
	}
	// a base64 file pasted into a secret often comes wrapped in lines
	der, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(cc.Pfx), ""))
	if err != nil {
		return tls.Certificate{}, cerr.Validation("http: clientCert.pfx is not valid base64")
	}
	priv, leaf, chain, err := pkcs12.DecodeChain(der, cc.Password)
	if err != nil {
		return tls.Certificate{}, cerr.Validation("http: clientCert could not be read: wrong password or malformed file")
	}
	cert := tls.Certificate{Certificate: [][]byte{leaf.Raw}, PrivateKey: priv, Leaf: leaf}
	for _, ca := range chain {
		cert.Certificate = append(cert.Certificate, ca.Raw)
	}
	return cert, nil
}
