package credstore

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"

	"golang.org/x/crypto/argon2"
)

// The lookup hash and the encryption key are both derived from the domain,
// with distinct prefixes so the two derivations can never collide. The domain
// is a low-entropy secret, so this design provides obfuscation and
// domain-binding against casual file scraping, not brute-force resistance
// against a targeted attacker who guesses common GitLab hosts.
const (
	lookupPrefix = "gl-lookup:v1"
	keyPrefix    = "gl-key:v1:"

	saltSize  = 16
	nonceSize = 12
	keySize   = 32

	kdfNameArgon2id = "argon2id"
)

const (
	defaultKDFTime      uint32 = 2
	defaultKDFMemoryKiB uint32 = 64 * 1024
	defaultKDFThreads   uint8  = 4
)

func defaultKDFParams() kdfParams {
	return kdfParams{
		Name:      kdfNameArgon2id,
		Time:      defaultKDFTime,
		MemoryKiB: defaultKDFMemoryKiB,
		Threads:   defaultKDFThreads,
	}
}

func lookupHash(salt []byte, domain string) []byte {
	hash := sha256.New()
	hash.Write([]byte(lookupPrefix))
	hash.Write(salt)
	hash.Write([]byte(domain))

	return hash.Sum(nil)
}

func deriveKey(domain string, salt []byte, params kdfParams) ([]byte, error) {
	if params.Name != kdfNameArgon2id {
		return nil, fmt.Errorf("%w: unknown kdf %q", ErrCorruptCredentials, params.Name)
	}
	if params.Time == 0 || params.MemoryKiB == 0 || params.Threads == 0 {
		return nil, fmt.Errorf("%w: invalid kdf parameters", ErrCorruptCredentials)
	}

	return argon2.IDKey([]byte(keyPrefix+domain), salt, params.Time, params.MemoryKiB, params.Threads, keySize), nil
}

func encryptToken(domain, token string) (fileEntry, error) {
	salt := make([]byte, saltSize)
	if _, err := rand.Read(salt); err != nil {
		return fileEntry{}, fmt.Errorf("generate salt: %w", err)
	}

	nonce := make([]byte, nonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return fileEntry{}, fmt.Errorf("generate nonce: %w", err)
	}

	params := defaultKDFParams()
	key, err := deriveKey(domain, salt, params)
	if err != nil {
		return fileEntry{}, err
	}

	aead, err := newAEAD(key)
	if err != nil {
		return fileEntry{}, err
	}

	ciphertext := aead.Seal(nil, nonce, []byte(token), nil)
	encode := base64.StdEncoding.EncodeToString

	return fileEntry{
		Salt:   encode(salt),
		Lookup: encode(lookupHash(salt, domain)),
		Nonce:  encode(nonce),
		Token:  encode(ciphertext),
		KDF:    params,
	}, nil
}

func decryptToken(domain string, entry fileEntry) (string, error) {
	decode := func(field, value string) ([]byte, error) {
		decoded, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid %s encoding", ErrCorruptCredentials, field)
		}

		return decoded, nil
	}

	salt, err := decode("salt", entry.Salt)
	if err != nil {
		return "", err
	}
	nonce, err := decode("nonce", entry.Nonce)
	if err != nil {
		return "", err
	}
	ciphertext, err := decode("token", entry.Token)
	if err != nil {
		return "", err
	}

	key, err := deriveKey(domain, salt, entry.KDF)
	if err != nil {
		return "", err
	}

	aead, err := newAEAD(key)
	if err != nil {
		return "", err
	}

	token, err := aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("%w: token decryption failed", ErrCorruptCredentials)
	}

	return string(token), nil
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}

	return aead, nil
}
