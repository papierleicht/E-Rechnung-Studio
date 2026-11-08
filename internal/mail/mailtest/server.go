// Package mailtest provides a local fake SMTP server for tests. It never
// relays messages; everything stays in memory.
package mailtest

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"net"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/zalando/go-keyring"
)

// Config controls the fake server's behavior.
type Config struct {
	// Security is "starttls" (default) or "tls" for implicit TLS.
	Security string
	// AuthMechanisms advertised after TLS, default "PLAIN LOGIN".
	AuthMechanisms string
	Username       string
	Password       string
	// NoSTARTTLS omits STARTTLS from EHLO (downgrade test).
	NoSTARTTLS bool
	// RejectRecipient is answered with 550.
	RejectRecipient string
	// RejectData answers the message body with 554.
	RejectData bool
}

// Received is one accepted message.
type Received struct {
	From string
	To   []string
	Data []byte
	Auth string
}

type Server struct {
	Host    string
	Port    int
	RootCAs *x509.CertPool
	config  Config
	tls     *tls.Config
	ln      net.Listener
	mu      sync.Mutex
	got     []Received
	wg      sync.WaitGroup
}

// Start runs a fake server on 127.0.0.1 until the test ends.
func Start(t testing.TB, config Config) *Server {
	t.Helper()
	if config.Security == "" {
		config.Security = "starttls"
	}
	if config.AuthMechanisms == "" {
		config.AuthMechanisms = "PLAIN LOGIN"
	}
	certificate, pool := selfSigned(t)
	s := &Server{config: config, RootCAs: pool, tls: &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12}}
	var err error
	if config.Security == "tls" {
		s.ln, err = tls.Listen("tcp", "127.0.0.1:0", s.tls)
	} else {
		s.ln, err = net.Listen("tcp", "127.0.0.1:0")
	}
	if err != nil {
		t.Fatal(err)
	}
	address := s.ln.Addr().(*net.TCPAddr)
	s.Host, s.Port = "127.0.0.1", address.Port
	s.wg.Add(1)
	go s.serve()
	t.Cleanup(func() { s.ln.Close(); s.wg.Wait() })
	return s
}

// Messages returns the accepted messages.
func (s *Server) Messages() []Received {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Received(nil), s.got...)
}

func (s *Server) serve() {
	defer s.wg.Done()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			defer conn.Close()
			_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
			s.session(conn)
		}()
	}
}

func (s *Server) session(conn net.Conn) {
	text := textproto.NewConn(conn)
	secure := s.config.Security == "tls"
	authenticated := ""
	var current Received
	reply := func(lines ...string) bool {
		for _, line := range lines {
			if err := text.PrintfLine("%s", line); err != nil {
				return false
			}
		}
		return true
	}
	reply("220 fake.test ESMTP")
	for {
		line, err := text.ReadLine()
		if err != nil {
			return
		}
		verb, arg, _ := strings.Cut(line, " ")
		switch strings.ToUpper(verb) {
		case "EHLO", "HELO":
			lines := []string{"250-fake.test"}
			if !secure && !s.config.NoSTARTTLS {
				lines = append(lines, "250-STARTTLS")
			}
			if secure || s.config.NoSTARTTLS {
				lines = append(lines, "250-AUTH "+s.config.AuthMechanisms)
			}
			reply(append(lines, "250 8BITMIME")...)
		case "STARTTLS":
			if secure {
				reply("503 already secure")
				continue
			}
			reply("220 go ahead")
			tlsConn := tls.Server(conn, s.tls)
			if tlsConn.Handshake() != nil {
				return
			}
			conn, secure = tlsConn, true
			text = textproto.NewConn(conn)
		case "AUTH":
			mechanism, initial, _ := strings.Cut(arg, " ")
			var user, pass string
			switch strings.ToUpper(mechanism) {
			case "PLAIN":
				decoded, _ := base64.StdEncoding.DecodeString(initial)
				parts := strings.Split(string(decoded), "\x00")
				if len(parts) == 3 {
					user, pass = parts[1], parts[2]
				}
			case "LOGIN":
				reply("334 " + base64.StdEncoding.EncodeToString([]byte("Username:")))
				u, _ := text.ReadLine()
				reply("334 " + base64.StdEncoding.EncodeToString([]byte("Password:")))
				p, _ := text.ReadLine()
				du, _ := base64.StdEncoding.DecodeString(u)
				dp, _ := base64.StdEncoding.DecodeString(p)
				user, pass = string(du), string(dp)
			default:
				reply("504 unsupported")
				continue
			}
			if user == s.config.Username && pass == s.config.Password {
				authenticated = strings.ToUpper(mechanism)
				reply("235 2.7.0 Authentication successful")
			} else {
				reply("535 5.7.8 Authentication credentials invalid")
			}
		case "MAIL":
			if s.config.Username != "" && authenticated == "" {
				reply("530 5.7.0 Authentication required")
				continue
			}
			current = Received{From: address(arg), Auth: authenticated}
			reply("250 OK")
		case "RCPT":
			to := address(arg)
			if to == s.config.RejectRecipient {
				reply("550 5.1.1 Mailbox unavailable")
				continue
			}
			current.To = append(current.To, to)
			reply("250 OK")
		case "DATA":
			reply("354 End data with <CR><LF>.<CR><LF>")
			data, err := text.ReadDotBytes()
			if err != nil {
				return
			}
			if s.config.RejectData {
				reply("554 5.6.0 Message rejected")
				continue
			}
			current.Data = data
			s.mu.Lock()
			s.got = append(s.got, current)
			s.mu.Unlock()
			current = Received{}
			reply("250 OK queued")
		case "RSET", "NOOP":
			reply("250 OK")
		case "QUIT":
			reply("221 bye")
			return
		default:
			reply("502 not implemented")
		}
	}
}

func address(arg string) string {
	start, end := strings.Index(arg, "<"), strings.Index(arg, ">")
	if start < 0 || end < start {
		return ""
	}
	return arg[start+1 : end]
}

func selfSigned(t testing.TB) (tls.Certificate, *x509.CertPool) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "fake smtp"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, DNSNames: []string{"localhost"},
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, IsCA: true, BasicConstraintsValid: true,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(parsed)
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}, pool
}

// Vault is an in-memory keyring replacement.
type Vault struct {
	mu      sync.Mutex
	secrets map[string]string
}

func (v *Vault) Get(id string) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	secret, ok := v.secrets[id]
	if !ok {
		return "", keyring.ErrNotFound
	}
	return secret, nil
}

func (v *Vault) Set(id, secret string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.secrets == nil {
		v.secrets = map[string]string{}
	}
	v.secrets[id] = secret
	return nil
}

func (v *Vault) Delete(id string) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.secrets, id)
	return nil
}
