// Package cloudbackup encrypts backups on the device before they are uploaded.
// The server only ever receives ciphertext.
//
// A backup is a set of objects: every document (PDF, XML, photo) and every part
// of the backup document is sealed on its own and stored once. The index of a
// state (Envelope) lists the objects by the hash of their plaintext, so a new
// state costs little and every document can be verified on its own.
//
// The recovery key is 32 random bytes shown once to the customer as Base32
// (with a 3-byte checksum against typos). Because it has full entropy it
// needs no password hashing; HKDF-SHA256 derives a fresh AES-256-GCM key per
// backup from it and a random salt.
//
// File layout of an object or index (format version 1):
//
//	"PLBK" | version (1) | key ID (8) | salt (16) | nonce (12) | AES-GCM(gzip(data))
//
// The whole header is authenticated as additional data.
package cloudbackup

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"time"
)

// FormatVersion is stored in the header and on the server.
const FormatVersion = 1

const (
	magic      = "PLBK"
	keyIDSize  = 8
	saltSize   = 16
	nonceSize  = 12
	headerSize = len(magic) + 1 + keyIDSize + saltSize + nonceSize
	secretSize = 32
	checkSize  = 3
	// MaxPlaintext bounds decompression of a downloaded backup.
	MaxPlaintext = 1 << 30
)

var (
	ErrInvalidKey  = errors.New("Der Wiederherstellungsschlüssel ist ungültig. Prüfe ihn auf Tippfehler.")
	ErrNotBackup   = errors.New("Die Datei ist keine verschlüsselte papierleicht-Sicherung.")
	ErrUnsupported = errors.New("Diese Sicherung stammt aus einer neueren App-Version. Aktualisiere die App.")
	ErrWrongKey    = errors.New("Diese Sicherung wurde mit einem anderen Wiederherstellungsschlüssel verschlüsselt.")
	ErrTampered    = errors.New("Die Sicherung ist beschädigt oder wurde verändert. Sie lässt sich nicht entschlüsseln.")
	ErrTooLarge    = errors.New("Die entschlüsselte Sicherung ist zu groß.")
)

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// Key is a recovery key. Its zero value is invalid.
type Key struct{ secret [secretSize]byte }

func NewKey() (Key, error) {
	var k Key
	_, err := rand.Read(k.secret[:])
	return k, err
}

func checksum(secret []byte) []byte {
	sum := sha256.Sum256(append([]byte("papierleicht recovery key"), secret...))
	return sum[:checkSize]
}

// String formats the key as 14 groups of four Base32 characters.
func (k Key) String() string {
	raw := b32.EncodeToString(append(k.secret[:], checksum(k.secret[:])...))
	var groups []string
	for i := 0; i < len(raw); i += 4 {
		groups = append(groups, raw[i:min(i+4, len(raw))])
	}
	return strings.Join(groups, "-")
}

// ParseKey accepts the formatted key with any spacing, dashes or case and
// maps the easily confused 0/1/8 to O/I/B.
func ParseKey(s string) (Key, error) {
	var k Key
	clean := strings.Map(func(r rune) rune {
		switch r {
		case ' ', '-', '\t', '\n', '\r':
			return -1
		case '0':
			return 'O'
		case '1':
			return 'I'
		case '8':
			return 'B'
		}
		return r
	}, strings.ToUpper(s))
	raw, err := b32.DecodeString(clean)
	if err != nil || len(raw) != secretSize+checkSize || subtle.ConstantTimeCompare(raw[secretSize:], checksum(raw[:secretSize])) != 1 {
		return k, ErrInvalidKey
	}
	copy(k.secret[:], raw)
	return k, nil
}

func (k Key) valid() bool { return k.secret != [secretSize]byte{} }

func (k Key) derive(info string, salt []byte, size int) []byte {
	out, err := hkdf.Key(sha256.New, k.secret[:], salt, info, size)
	if err != nil {
		panic(err) // only for invalid sizes
	}
	return out
}

// ID is a public fingerprint that tells "wrong key" apart from tampering.
func (k Key) ID() []byte { return k.derive("papierleicht cloud backup key id", nil, keyIDSize) }

// Equal compares two keys in constant time.
func (k Key) Equal(o Key) bool { return subtle.ConstantTimeCompare(k.secret[:], o.secret[:]) == 1 }

// Seal compresses and encrypts plaintext.
func Seal(k Key, plaintext []byte) ([]byte, error) {
	if !k.valid() {
		return nil, ErrInvalidKey
	}
	var compressed bytes.Buffer
	zw := gzip.NewWriter(&compressed)
	if _, err := zw.Write(plaintext); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	header := make([]byte, headerSize, headerSize+compressed.Len()+16)
	copy(header, magic)
	header[len(magic)] = FormatVersion
	copy(header[len(magic)+1:], k.ID())
	salt := header[len(magic)+1+keyIDSize : len(magic)+1+keyIDSize+saltSize]
	nonce := header[headerSize-nonceSize:]
	if _, err := rand.Read(header[len(magic)+1+keyIDSize:]); err != nil {
		return nil, err
	}
	aead, err := newAEAD(k, salt)
	if err != nil {
		return nil, err
	}
	return aead.Seal(header, nonce, compressed.Bytes(), header), nil
}

func newAEAD(k Key, salt []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(k.derive("papierleicht cloud backup v1 encryption", salt, 32))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// Open decrypts and decompresses a sealed backup.
func Open(k Key, blob []byte) ([]byte, error) {
	if !k.valid() {
		return nil, ErrInvalidKey
	}
	if len(blob) < headerSize+16 || string(blob[:len(magic)]) != magic {
		return nil, ErrNotBackup
	}
	if blob[len(magic)] != FormatVersion {
		return nil, ErrUnsupported
	}
	header := blob[:headerSize]
	if subtle.ConstantTimeCompare(header[len(magic)+1:len(magic)+1+keyIDSize], k.ID()) != 1 {
		return nil, ErrWrongKey
	}
	aead, err := newAEAD(k, header[len(magic)+1+keyIDSize:len(magic)+1+keyIDSize+saltSize])
	if err != nil {
		return nil, err
	}
	compressed, err := aead.Open(nil, header[headerSize-nonceSize:], blob[headerSize:], header)
	if err != nil {
		return nil, ErrTampered
	}
	zr, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, ErrTampered
	}
	out, err := io.ReadAll(io.LimitReader(zr, MaxPlaintext+1))
	if err != nil {
		return nil, ErrTampered
	}
	if len(out) > MaxPlaintext {
		return nil, ErrTooLarge
	}
	return out, nil
}

// ObjectID is the server's name of a document. It is keyed, so the server can
// neither recognize a known file by its hash nor link two customers' files.
func (k Key) ObjectID(sum string) string {
	return hex.EncodeToString(k.derive("papierleicht cloud backup object id", []byte(sum), 32))
}

// Bounds of the parts of a backup document.
const (
	minChunk = 256 << 10
	maxChunk = 4 << 20
	// chunkMask cuts about every megabyte; it reads the high bits of the
	// rolling hash, which depend on the last 64 bytes.
	chunkMask uint64 = (1<<20 - 1) << 44
)

// Split cuts a backup document at positions that depend on its content, so a
// change only affects the parts around it and the others need no new upload.
// The cut points depend on the key and do not reveal anything about the text.
func (k Key) Split(data []byte) [][]byte {
	raw := k.derive("papierleicht cloud backup chunking", nil, 256*8)
	var table [256]uint64
	for i := range table {
		table[i] = binary.LittleEndian.Uint64(raw[i*8:])
	}
	var parts [][]byte
	var hash uint64
	start := 0
	for i, b := range data {
		hash = hash<<1 + table[b]
		size := i + 1 - start
		if (size >= minChunk && hash&chunkMask == 0) || size >= maxChunk {
			parts = append(parts, data[start:i+1])
			start, hash = i+1, 0
		}
	}
	if start < len(data) {
		parts = append(parts, data[start:])
	}
	return parts
}

// Ref names a stored object by the SHA-256 of its plaintext.
type Ref struct {
	Sum  string `json:"sum"`
	Size int64  `json:"size"`
}

// NewRef describes plaintext that will be stored as an object.
func NewRef(data []byte) Ref {
	sum := sha256.Sum256(data)
	return Ref{Sum: hex.EncodeToString(sum[:]), Size: int64(len(data))}
}

// Check rejects anything but the bytes the reference was made from.
func (r Ref) Check(data []byte) error {
	if NewRef(data).Sum != r.Sum {
		return ErrTampered
	}
	return nil
}

// Counts summarize a state for the restore preview.
type Counts struct {
	Customers int `json:"customers"`
	Products  int `json:"products"`
	Invoices  int `json:"invoices"`
	Received  int `json:"received"`
	Expenses  int `json:"expenses"`
}

// Envelope is the encrypted index of one backup state: the parts of its
// document and the documents it refers to. Secrets such as FinTS PINs or SMTP
// passwords are not part of the document and therefore not included.
type Envelope struct {
	Format     string    `json:"format"`
	Version    int       `json:"version"`
	CreatedAt  time.Time `json:"createdAt"`
	DeviceName string    `json:"deviceName"`
	AppVersion string    `json:"appVersion"`
	Document   []Ref     `json:"document"`
	Blobs      []Ref     `json:"blobs"`
	Counts     Counts    `json:"counts"`
}

const envelopeFormat = "papierleicht-cloud-backup"

// EnvelopeVersion is reported to the server as the format of a version.
const EnvelopeVersion = 2

// Pack seals the index of a backup state.
func Pack(k Key, env Envelope) ([]byte, error) {
	env.Format, env.Version, env.CreatedAt = envelopeFormat, EnvelopeVersion, env.CreatedAt.UTC()
	raw, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	return Seal(k, raw)
}

// Unpack opens a sealed index.
func Unpack(k Key, blob []byte) (Envelope, error) {
	var env Envelope
	raw, err := Open(k, blob)
	if err != nil {
		return env, err
	}
	if json.Unmarshal(raw, &env) != nil || env.Format != envelopeFormat || len(env.Document) == 0 {
		return Envelope{}, ErrNotBackup
	}
	if env.Version != EnvelopeVersion {
		return Envelope{}, ErrUnsupported
	}
	return env, nil
}

// Refs lists every object of a state once, the parts of the document first.
func (env Envelope) Refs() []Ref {
	seen := make(map[string]bool, len(env.Document)+len(env.Blobs))
	refs := make([]Ref, 0, len(env.Document)+len(env.Blobs))
	for _, list := range [][]Ref{env.Document, env.Blobs} {
		for _, r := range list {
			if !seen[r.Sum] {
				seen[r.Sum] = true
				refs = append(refs, r)
			}
		}
	}
	return refs
}

// Preview summarizes a state before it replaces local data.
type Preview struct {
	CreatedAt  string `json:"createdAt"`
	DeviceName string `json:"deviceName"`
	AppVersion string `json:"appVersion"`
	Customers  int    `json:"customers"`
	Products   int    `json:"products"`
	Invoices   int    `json:"invoices"`
	Received   int    `json:"received"`
	Expenses   int    `json:"expenses"`
	// Documents and Bytes count the stored files.
	Documents int   `json:"documents"`
	Bytes     int64 `json:"bytes"`
}

func (env Envelope) Preview() Preview {
	p := Preview{CreatedAt: env.CreatedAt.UTC().Format(time.RFC3339), DeviceName: env.DeviceName, AppVersion: env.AppVersion,
		Customers: env.Counts.Customers, Products: env.Counts.Products, Invoices: env.Counts.Invoices, Received: env.Counts.Received,
		Expenses: env.Counts.Expenses, Documents: len(env.Blobs)}
	for _, r := range env.Blobs {
		p.Bytes += r.Size
	}
	return p
}
