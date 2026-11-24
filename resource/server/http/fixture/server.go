package fixture

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	athanorfixture "github.com/grandper/go-athanor/fixture"
	httpserver "github.com/grandper/go-athanor/resource/server/http"
)

// InitOnFreePort sets a free TCP port on config and calls initialize, typically the Init of the server built
// from config. The HTTP server binds its port itself, so another process can take the port first: initialize
// is then called again with another port, as athanorfixture.OnFreePort does.
func InitOnFreePort(
	ctx context.Context, t *testing.T, config *httpserver.Config, initialize func(context.Context) error,
) error {
	t.Helper()

	return athanorfixture.OnFreePort(t, func(port int) error {
		config.BindPort = port
		return initialize(ctx)
	})
}

// loopbackAddress is the address the certificates of WriteSelfSignedCertificate are valid for.
const loopbackAddress = "127.0.0.1"

// certificateValidity is how long the certificates of WriteSelfSignedCertificate stay valid.
const certificateValidity = time.Hour

// WriteSelfSignedCertificate writes a self-signed TLS certificate for 127.0.0.1 and its private key, PEM-encoded,
// to a temporary directory removed when the test ends, and returns the paths of the certificate and key files.
func WriteSelfSignedCertificate(t *testing.T) (string, string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: loopbackAddress},
		IPAddresses:  []net.IP{net.ParseIP(loopbackAddress)},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(certificateValidity),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)
	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)

	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	writePEM(t, certFile, "CERTIFICATE", certDER)
	writePEM(t, keyFile, "EC PRIVATE KEY", keyDER)
	return certFile, keyFile
}

// writePEM writes der to path as a PEM block of the given type, readable by its owner only.
func writePEM(t *testing.T, path string, blockType string, der []byte) {
	t.Helper()

	require.NoError(t, os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: der}), 0o600))
}
