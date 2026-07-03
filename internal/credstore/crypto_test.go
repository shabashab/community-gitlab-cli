package credstore

import (
	"bytes"
	"encoding/base64"
	"errors"
	"testing"
)

func TestEncryptDecryptRoundtrip(t *testing.T) {
	entry, err := encryptToken("gitlab.example.com", "glpat-secret")
	if err != nil {
		t.Fatalf("encryptToken error = %v", err)
	}

	token, err := decryptToken("gitlab.example.com", entry)
	if err != nil {
		t.Fatalf("decryptToken error = %v", err)
	}
	if token != "glpat-secret" {
		t.Fatalf("decrypted token = %q, want original", token)
	}
}

func TestEncryptTokenUsesFreshSaltAndNonce(t *testing.T) {
	first, err := encryptToken("gitlab.example.com", "glpat-secret")
	if err != nil {
		t.Fatalf("encryptToken error = %v", err)
	}
	second, err := encryptToken("gitlab.example.com", "glpat-secret")
	if err != nil {
		t.Fatalf("encryptToken error = %v", err)
	}

	if first.Salt == second.Salt {
		t.Fatal("salt reused across encryptions")
	}
	if first.Nonce == second.Nonce {
		t.Fatal("nonce reused across encryptions")
	}
}

func TestLookupHashDeterministicPerSaltAndDomain(t *testing.T) {
	salt := bytes.Repeat([]byte{0x01}, saltSize)
	otherSalt := bytes.Repeat([]byte{0x02}, saltSize)

	if !bytes.Equal(lookupHash(salt, "gitlab.com"), lookupHash(salt, "gitlab.com")) {
		t.Fatal("lookupHash not deterministic for same salt and domain")
	}
	if bytes.Equal(lookupHash(salt, "gitlab.com"), lookupHash(otherSalt, "gitlab.com")) {
		t.Fatal("lookupHash identical across salts")
	}
	if bytes.Equal(lookupHash(salt, "gitlab.com"), lookupHash(salt, "gitlab.example.com")) {
		t.Fatal("lookupHash identical across domains")
	}
}

func TestDecryptTokenWrongDomainFails(t *testing.T) {
	entry, err := encryptToken("gitlab.example.com", "glpat-secret")
	if err != nil {
		t.Fatalf("encryptToken error = %v", err)
	}

	if _, err := decryptToken("gitlab.other.com", entry); err == nil {
		t.Fatal("decryptToken with wrong domain succeeded, want error")
	}
}

func TestDecryptTokenDetectsTampering(t *testing.T) {
	entry, err := encryptToken("gitlab.example.com", "glpat-secret")
	if err != nil {
		t.Fatalf("encryptToken error = %v", err)
	}

	tamper := func(value string) string {
		raw, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			t.Fatalf("decode field: %v", err)
		}
		raw[0] ^= 0xff

		return base64.StdEncoding.EncodeToString(raw)
	}

	cases := []struct {
		name   string
		mutate func(fileEntry) fileEntry
	}{
		{name: "ciphertext", mutate: func(e fileEntry) fileEntry { e.Token = tamper(e.Token); return e }},
		{name: "nonce", mutate: func(e fileEntry) fileEntry { e.Nonce = tamper(e.Nonce); return e }},
		{name: "salt", mutate: func(e fileEntry) fileEntry { e.Salt = tamper(e.Salt); return e }},
		{name: "invalid base64", mutate: func(e fileEntry) fileEntry { e.Token = "!!!"; return e }},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if _, err := decryptToken("gitlab.example.com", testCase.mutate(entry)); err == nil {
				t.Fatal("decryptToken with tampered entry succeeded, want error")
			}
		})
	}
}

func TestDeriveKeyRejectsUnknownKDF(t *testing.T) {
	params := defaultKDFParams()
	params.Name = "scrypt"

	if _, err := deriveKey("gitlab.com", bytes.Repeat([]byte{0x01}, saltSize), params); !errors.Is(err, ErrCorruptCredentials) {
		t.Fatalf("deriveKey error = %v, want ErrCorruptCredentials", err)
	}
}
