package engine

import (
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"strings"
	"time"

	pkcs12 "software.sslmate.com/src/go-pkcs12"

	"github.com/jrvidotti/ddcore/internal/cerr"
)

// What decodePfx fails with. Neither carries the library's own error, which
// could repeat the material: the caller words the message.
var (
	errPfxBase64     = errors.New("pfx is not valid base64")
	errPfxUnreadable = errors.New("pfx could not be read")
)

// decodePfx reads a base64 PKCS#12 file: its key, its leaf certificate and
// the chain that came with it.
func decodePfx(pfx, password string) (any, *x509.Certificate, []*x509.Certificate, error) {
	// a base64 file pasted into a secret often comes wrapped in lines
	der, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(pfx), ""))
	if err != nil {
		return nil, nil, nil, errPfxBase64
	}
	priv, leaf, chain, err := pkcs12.DecodeChain(der, password)
	if err != nil {
		return nil, nil, nil, errPfxUnreadable
	}
	return priv, leaf, chain, nil
}

// pfxInfo is ddcore.crypto.pfxInfo: what the leaf of a PKCS#12 file says,
// and nothing of its key.
func pfxInfo(pfx, password string) (map[string]any, error) {
	_, leaf, chain, err := decodePfx(pfx, password)
	if err == errPfxBase64 {
		return nil, cerr.Validation("crypto: pfx is not valid base64")
	}
	if err != nil {
		return nil, cerr.Validation("crypto: pfx could not be read: wrong password or malformed file")
	}
	return certInfo(leaf, len(chain)), nil
}

// pemCertInfo is ddcore.crypto.certInfo: the first certificate of a PEM text
// is the leaf, the ones after it its chain. Any other block, a key pasted
// along with it included, is skipped.
func pemCertInfo(text string) (map[string]any, error) {
	var certs []*x509.Certificate
	for rest := []byte(text); ; {
		var block *pem.Block
		if block, rest = pem.Decode(rest); block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			certs = nil
			break
		}
		certs = append(certs, cert)
	}
	if len(certs) == 0 {
		return nil, cerr.Validation("crypto: cert is not a valid PEM certificate")
	}
	return certInfo(certs[0], len(certs)-1), nil
}

// certInfo describes a certificate to app code; chain is how many
// certificates came along with it.
func certInfo(leaf *x509.Certificate, chain int) map[string]any {
	return map[string]any{
		"notBefore": leaf.NotBefore.UTC().Format(time.RFC3339),
		"notAfter":  leaf.NotAfter.UTC().Format(time.RFC3339),
		"subject":   leaf.Subject.String(),
		"issuer":    leaf.Issuer.String(),
		"serial":    leaf.SerialNumber.Text(16),
		"chain":     chain,
	}
}
