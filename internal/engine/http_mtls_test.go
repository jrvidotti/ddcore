package engine

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	pkcs12 "software.sslmate.com/src/go-pkcs12"

	"github.com/jrvidotti/ddcore/internal/js"
)

const mtlsPassword = "s3cret-pfx-password"

// mtlsFixture is a server that demands a client certificate signed by its
// CA, and that client certificate as a PKCS#12 file and as a PEM pair.
type mtlsFixture struct {
	server  *httptest.Server
	conns   atomic.Int32
	pfx     string
	certPEM string
	keyPEM  string
}

func newMTLSFixture(t *testing.T) *mtlsFixture {
	t.Helper()
	issue := func(tmpl, parent *x509.Certificate, parentKey *ecdsa.PrivateKey) (*x509.Certificate, *ecdsa.PrivateKey) {
		key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatal(err)
		}
		if parent == nil {
			parent, parentKey = tmpl, key
		}
		der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, parentKey)
		if err != nil {
			t.Fatal(err)
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			t.Fatal(err)
		}
		return cert, key
	}
	validity := func(serial int64, cn string) *x509.Certificate {
		return &x509.Certificate{
			SerialNumber: big.NewInt(serial),
			Subject:      pkix.Name{CommonName: cn},
			NotBefore:    time.Now().Add(-time.Hour),
			NotAfter:     time.Now().Add(time.Hour),
		}
	}
	caTmpl := validity(1, "test ca")
	caTmpl.IsCA = true
	caTmpl.BasicConstraintsValid = true
	caTmpl.KeyUsage = x509.KeyUsageCertSign
	ca, caKey := issue(caTmpl, nil, nil)

	serverTmpl := validity(2, "server")
	serverTmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	serverTmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
	serverCert, serverKey := issue(serverTmpl, ca, caKey)

	clientTmpl := validity(3, "pix client")
	clientTmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	clientCert, clientKey := issue(clientTmpl, ca, caKey)

	roots := x509.NewCertPool()
	roots.AddCert(ca)
	f := &mtlsFixture{}
	f.server = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"cn":    r.TLS.PeerCertificates[0].Subject.CommonName,
			"chain": len(r.TLS.PeerCertificates),
		})
	}))
	f.server.TLS = &tls.Config{
		Certificates: []tls.Certificate{{Certificate: [][]byte{serverCert.Raw}, PrivateKey: serverKey}},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    roots,
	}
	f.server.Config.ConnState = func(_ net.Conn, s http.ConnState) {
		if s == http.StateNew {
			f.conns.Add(1)
		}
	}
	f.server.StartTLS()
	t.Cleanup(f.server.Close)

	httpRootCAs = roots
	t.Cleanup(func() { httpRootCAs = nil })

	pfx, err := pkcs12.Modern.Encode(clientKey, clientCert, []*x509.Certificate{ca}, mtlsPassword)
	if err != nil {
		t.Fatal(err)
	}
	f.pfx = base64.StdEncoding.EncodeToString(pfx)
	keyDER, err := x509.MarshalPKCS8PrivateKey(clientKey)
	if err != nil {
		t.Fatal(err)
	}
	f.certPEM = string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: clientCert.Raw}))
	f.keyPEM = string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}))
	return f
}

func mtlsRuntime(t *testing.T) *js.Runtime {
	t.Helper()
	pool, err := js.NewPool(&Engine{}, nil, 1, false)
	if err != nil {
		t.Fatal(err)
	}
	rt, err := pool.Acquire()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rt.Release)
	rt.Ctx = &Ctx{}
	return rt
}

// A PKCS#12 file authenticates the call, and the chain in it goes with the leaf.
func TestHTTPClientCertPfx(t *testing.T) {
	f := newMTLSFixture(t)
	rt := mtlsRuntime(t)
	v, err := rt.Eval(fmt.Sprintf(`ddcore.http.get(%q, {clientCert: {pfx: %q, password: %q}}).body`, f.server.URL, f.pfx, mtlsPassword))
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); !strings.Contains(got, `pix client`) || !strings.Contains(got, `\"chain\":2`) {
		t.Fatalf("server saw %s", got)
	}
}

func TestHTTPClientCertPem(t *testing.T) {
	f := newMTLSFixture(t)
	rt := mtlsRuntime(t)
	v, err := rt.Eval(fmt.Sprintf(`ddcore.http.post(%q, {a: 1}, {clientCert: {cert: %q, key: %q}}).body`, f.server.URL, f.certPEM, f.keyPEM))
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); !strings.Contains(got, `pix client`) || !strings.Contains(got, `\"chain\":1`) {
		t.Fatalf("server saw %s", got)
	}
}

// Consecutive calls with the same certificate share a connection: the token
// request and the operation after it pay for one handshake.
func TestHTTPClientCertReuse(t *testing.T) {
	f := newMTLSFixture(t)
	rt := mtlsRuntime(t)
	code := fmt.Sprintf(`const cc = {pfx: %q, password: %q};
ddcore.http.get(%q, {clientCert: cc}); ddcore.http.get(%q, {clientCert: cc}); 1`, f.pfx, mtlsPassword, f.server.URL, f.server.URL)
	if _, err := rt.Eval(code); err != nil {
		t.Fatal(err)
	}
	if n := f.conns.Load(); n != 1 {
		t.Fatalf("%d connections for two calls, want 1", n)
	}
}

// A certificate that cannot be read is a validation error that says nothing
// of the material.
func TestHTTPClientCertErrors(t *testing.T) {
	f := newMTLSFixture(t)
	rt := mtlsRuntime(t)
	garbage := base64.StdEncoding.EncodeToString([]byte("not a pkcs12 file at all"))
	for _, tc := range []struct{ name, clientCert, want string }{
		{"wrong password", fmt.Sprintf(`{pfx: %q, password: "wr0ng-guess"}`, f.pfx), "wrong password or malformed file"},
		{"no password", fmt.Sprintf(`{pfx: %q}`, f.pfx), "wrong password or malformed file"},
		{"malformed file", fmt.Sprintf(`{pfx: %q, password: %q}`, garbage, mtlsPassword), "wrong password or malformed file"},
		{"not base64", fmt.Sprintf(`{pfx: "@@not-base64@@", password: %q}`, mtlsPassword), "not valid base64"},
		{"both shapes", fmt.Sprintf(`{pfx: %q, cert: %q, key: %q}`, f.pfx, f.certPEM, f.keyPEM), "either pfx or cert and key"},
		{"neither shape", `{}`, "either pfx or cert and key"},
		{"cert without key", fmt.Sprintf(`{cert: %q}`, f.certPEM), "either pfx or cert and key"},
		{"bad pem", fmt.Sprintf(`{cert: "-----BEGIN NOPE-----", key: %q}`, f.keyPEM), "not a valid PEM pair"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := rt.Eval(fmt.Sprintf(`ddcore.http.get(%q, {clientCert: %s})`, f.server.URL, tc.clientCert))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("want %q, got %v", tc.want, err)
			}
			for _, secret := range []string{mtlsPassword, "wr0ng-guess", f.pfx[:24], garbage, "not-base64", "BEGIN"} {
				if strings.Contains(err.Error(), secret) {
					t.Fatalf("the error echoes the material: %v", err)
				}
			}
		})
	}
}
