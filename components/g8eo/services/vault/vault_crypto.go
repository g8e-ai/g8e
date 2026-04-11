// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

const (
	KeySize            = 32
	NonceSize          = 12
	KeyFingerprintSize = 16
	HKDFInfo           = "g8e-lfaa-kek-v1"

	aesKWDefaultIVHigh = 0xA6A6A6A6
	aesKWDefaultIVLow  = 0xA6A6A6A6
)

var (
	ErrInvalidKeySize      = errors.New("invalid key size: must be 32 bytes")
	ErrInvalidNonceSize    = errors.New("invalid nonce size: must be 12 bytes")
	ErrDecryptionFailed    = errors.New("decryption failed: authentication error")
	ErrKeyWrapFailed       = errors.New("key wrap failed")
	ErrKeyUnwrapFailed     = errors.New("key unwrap failed: integrity check failed")
	ErrInvalidWrappedKey   = errors.New("invalid wrapped key size")
	ErrInvalidPlaintextKey = errors.New("plaintext key must be multiple of 8 bytes")
)

// DeriveKEK derives a Key Encryption Key from an API key using HKDF-SHA256.
func DeriveKEK(apiKey string) ([]byte, error) {
	if apiKey == "" {
		return nil, errors.New("API key cannot be empty")
	}

	reader := hkdf.New(sha256.New, []byte(apiKey), nil, []byte(HKDFInfo))

	kek := make([]byte, KeySize)
	if _, err := io.ReadFull(reader, kek); err != nil {
		return nil, fmt.Errorf("HKDF expansion failed: %w", err)
	}

	return kek, nil
}

// GenerateDEK generates a cryptographically secure random Data Encryption Key.
func GenerateDEK() ([]byte, error) {
	dek := make([]byte, KeySize)
	if _, err := rand.Read(dek); err != nil {
		return nil, fmt.Errorf("failed to generate random DEK: %w", err)
	}
	return dek, nil
}

// GenerateNonce generates a cryptographically secure random nonce for AES-GCM.
func GenerateNonce() ([]byte, error) {
	nonce := make([]byte, NonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("failed to generate random nonce: %w", err)
	}
	return nonce, nil
}

// KeyFingerprint returns the first KeyFingerprintSize bytes of SHA256(key).
func KeyFingerprint(key []byte) []byte {
	hash := sha256.Sum256(key)
	return hash[:KeyFingerprintSize]
}

// APIKeyFingerprint returns the fingerprint of an API key.
func APIKeyFingerprint(apiKey string) []byte {
	return KeyFingerprint([]byte(apiKey))
}

// AESKeyWrap wraps a plaintext key using AES Key Wrap (RFC 3394).
// The KEK must be 16, 24, or 32 bytes (AES-128, AES-192, or AES-256).
// The plaintext must be a multiple of 8 bytes and at least 16 bytes.
func AESKeyWrap(kek, plaintext []byte) ([]byte, error) {
	if len(kek) != 16 && len(kek) != 24 && len(kek) != 32 {
		return nil, ErrInvalidKeySize
	}

	n := len(plaintext)
	if n < 16 || n%8 != 0 {
		return nil, ErrInvalidPlaintextKey
	}

	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	numBlocks := n / 8

	a := make([]byte, 8)
	binary.BigEndian.PutUint32(a[0:4], aesKWDefaultIVHigh)
	binary.BigEndian.PutUint32(a[4:8], aesKWDefaultIVLow)

	r := make([][]byte, numBlocks)
	for i := 0; i < numBlocks; i++ {
		r[i] = make([]byte, 8)
		copy(r[i], plaintext[i*8:(i+1)*8])
	}

	buf := make([]byte, 16)

	for j := 0; j <= 5; j++ {
		for i := 1; i <= numBlocks; i++ {
			// B = AES(K, A | R[i])
			copy(buf[0:8], a)
			copy(buf[8:16], r[i-1])
			block.Encrypt(buf, buf)

			// A = MSB(64, B) ^ t where t = (n*j)+i
			t := uint64(numBlocks*j + i)
			copy(a, buf[0:8])
			for k := 7; k >= 0 && t > 0; k-- {
				a[k] ^= byte(t & 0xFF)
				t >>= 8
			}

			// R[i] = LSB(64, B)
			copy(r[i-1], buf[8:16])
		}
	}

	ciphertext := make([]byte, 8+n)
	copy(ciphertext[0:8], a)
	for i := 0; i < numBlocks; i++ {
		copy(ciphertext[8+i*8:8+(i+1)*8], r[i])
	}

	return ciphertext, nil
}

// AESKeyUnwrap unwraps a ciphertext using AES Key Unwrap (RFC 3394).
// Returns the original plaintext key if the integrity check passes.
func AESKeyUnwrap(kek, ciphertext []byte) ([]byte, error) {
	if len(kek) != 16 && len(kek) != 24 && len(kek) != 32 {
		return nil, ErrInvalidKeySize
	}

	n := len(ciphertext)
	if n < 24 || n%8 != 0 {
		return nil, ErrInvalidWrappedKey
	}

	block, err := aes.NewCipher(kek)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	numBlocks := (n / 8) - 1

	a := make([]byte, 8)
	copy(a, ciphertext[0:8])

	r := make([][]byte, numBlocks)
	for i := 0; i < numBlocks; i++ {
		r[i] = make([]byte, 8)
		copy(r[i], ciphertext[8+i*8:8+(i+1)*8])
	}

	buf := make([]byte, 16)

	for j := 5; j >= 0; j-- {
		for i := numBlocks; i >= 1; i-- {
			t := uint64(numBlocks*j + i)
			aCopy := make([]byte, 8)
			copy(aCopy, a)
			for k := 7; k >= 0 && t > 0; k-- {
				aCopy[k] ^= byte(t & 0xFF)
				t >>= 8
			}

			// B = AES^-1(K, (A ^ t) | R[i])
			copy(buf[0:8], aCopy)
			copy(buf[8:16], r[i-1])
			block.Decrypt(buf, buf)

			// A = MSB(64, B)
			copy(a, buf[0:8])

			// R[i] = LSB(64, B)
			copy(r[i-1], buf[8:16])
		}
	}

	expectedA := make([]byte, 8)
	binary.BigEndian.PutUint32(expectedA[0:4], aesKWDefaultIVHigh)
	binary.BigEndian.PutUint32(expectedA[4:8], aesKWDefaultIVLow)

	if subtle.ConstantTimeCompare(a, expectedA) != 1 {
		return nil, ErrKeyUnwrapFailed
	}

	plaintext := make([]byte, numBlocks*8)
	for i := 0; i < numBlocks; i++ {
		copy(plaintext[i*8:(i+1)*8], r[i])
	}

	return plaintext, nil
}

// EncryptAESGCM encrypts plaintext using AES-256-GCM with the provided key and nonce.
func EncryptAESGCM(key, nonce, plaintext, additionalData []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKeySize
	}
	if len(nonce) != NonceSize {
		return nil, ErrInvalidNonceSize
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	ciphertext := gcm.Seal(nil, nonce, plaintext, additionalData)
	return ciphertext, nil
}

// DecryptAESGCM decrypts ciphertext using AES-256-GCM with the provided key and nonce.
func DecryptAESGCM(key, nonce, ciphertext, additionalData []byte) ([]byte, error) {
	if len(key) != KeySize {
		return nil, ErrInvalidKeySize
	}
	if len(nonce) != NonceSize {
		return nil, ErrInvalidNonceSize
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, additionalData)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}

// SecureZero zeros out a byte slice to prevent key material from lingering in memory.
func SecureZero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
