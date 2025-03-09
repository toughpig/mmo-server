package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

const (
	// NonceSize is the size of the nonce used in the AES-GCM
	NonceSize = 12

	// KeySize is the size of the AES key in bytes (256 bits)
	KeySize = 32

	// MaxPayloadSize is the maximum size of the payload that can be encrypted
	MaxPayloadSize = 16 * 1024 * 1024 // 16MB

	// DefaultKeyRotationInterval is the default interval for key rotation
	DefaultKeyRotationInterval = 24 * time.Hour
)

var (
	ErrInvalidKeySize    = errors.New("invalid key size")
	ErrInvalidNonceSize  = errors.New("invalid nonce size")
	ErrInvalidCiphertext = errors.New("invalid ciphertext")
	ErrPayloadTooLarge   = errors.New("payload too large")
	ErrInvalidPublicKey  = errors.New("invalid public key")
	ErrNoSharedSecret    = errors.New("could not generate shared secret")
)

// CryptoConfig 用于配置CryptoManager的参数
type CryptoConfig struct {
	KeyRotationInterval int64 // 密钥轮换间隔（秒）
}

// CryptoManager handles encryption and decryption operations
type CryptoManager struct {
	mu             sync.RWMutex
	sessionKeys    map[string][]byte // sessionID -> key
	privateKey     *ecdsa.PrivateKey
	publicKey      *ecdsa.PublicKey
	keyExpiration  time.Time
	rotationPeriod time.Duration
}

// EncryptedMessage represents an encrypted message
type EncryptedMessage struct {
	Nonce      []byte
	Ciphertext []byte
	SessionID  string
}

// NewCryptoManager creates a new CryptoManager
func NewCryptoManager(config *CryptoConfig) (*CryptoManager, error) {
	// Generate ECDH key pair for key exchange
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ECDH key pair: %w", err)
	}

	var rotationPeriod time.Duration
	if config != nil && config.KeyRotationInterval > 0 {
		rotationPeriod = time.Duration(config.KeyRotationInterval) * time.Second
	} else {
		rotationPeriod = DefaultKeyRotationInterval
	}

	return &CryptoManager{
		sessionKeys:    make(map[string][]byte),
		privateKey:     privateKey,
		publicKey:      &privateKey.PublicKey,
		keyExpiration:  time.Now().Add(rotationPeriod),
		rotationPeriod: rotationPeriod,
	}, nil
}

// GenerateSessionKey generates a new session key
func (cm *CryptoManager) GenerateSessionKey(sessionID string) ([]byte, error) {
	// Generate a random key
	key := make([]byte, KeySize)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate session key: %w", err)
	}

	cm.mu.Lock()
	cm.sessionKeys[sessionID] = key
	cm.mu.Unlock()

	return key, nil
}

// DeriveSharedKey derives a shared key using ECDHE
func (cm *CryptoManager) DeriveSharedKey(sessionID string, peerPublicKeyBytes []byte) ([]byte, error) {
	// Rotate keys if needed
	if time.Now().After(cm.keyExpiration) {
		if err := cm.RotateKeys(); err != nil {
			return nil, err
		}
	}

	// Unmarshal peer's public key
	x, y := elliptic.Unmarshal(elliptic.P256(), peerPublicKeyBytes)
	if x == nil {
		return nil, ErrInvalidPublicKey
	}

	peerPublicKey := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     x,
		Y:     y,
	}

	// Compute shared secret (ECDH)
	sharedSecret, _ := peerPublicKey.Curve.ScalarMult(peerPublicKey.X, peerPublicKey.Y, cm.privateKey.D.Bytes())
	if sharedSecret == nil {
		return nil, ErrNoSharedSecret
	}

	// Derive key from shared secret using SHA-256
	key := sha256.Sum256(sharedSecret.Bytes())

	cm.mu.Lock()
	cm.sessionKeys[sessionID] = key[:]
	cm.mu.Unlock()

	return key[:], nil
}

// GetPublicKeyBytes returns the public key in bytes
func (cm *CryptoManager) GetPublicKeyBytes() []byte {
	return elliptic.Marshal(cm.publicKey.Curve, cm.publicKey.X, cm.publicKey.Y)
}

// GetSessionKey gets a session key
func (cm *CryptoManager) GetSessionKey(sessionID string) ([]byte, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	key, exists := cm.sessionKeys[sessionID]
	return key, exists
}

// RemoveSessionKey removes a session key
func (cm *CryptoManager) RemoveSessionKey(sessionID string) {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	delete(cm.sessionKeys, sessionID)
}

// RotateKeys rotates the ECDH key pair
func (cm *CryptoManager) RotateKeys() error {
	// Generate new ECDH key pair
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("failed to rotate ECDH keys: %w", err)
	}

	cm.mu.Lock()
	cm.privateKey = privateKey
	cm.publicKey = &privateKey.PublicKey
	cm.keyExpiration = time.Now().Add(cm.rotationPeriod)
	cm.mu.Unlock()

	return nil
}

// Encrypt encrypts a plaintext using AES-GCM
func (cm *CryptoManager) Encrypt(plaintext []byte, sessionID string) (*EncryptedMessage, error) {
	if len(plaintext) > MaxPayloadSize {
		return nil, ErrPayloadTooLarge
	}

	// Get the session key
	cm.mu.RLock()
	key, exists := cm.sessionKeys[sessionID]
	cm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("session key not found for session ID: %s", sessionID)
	}

	// Create cipher block
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create GCM cipher
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM cipher: %w", err)
	}

	// Generate nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt
	ciphertext := gcm.Seal(nil, nonce, plaintext, nil)

	return &EncryptedMessage{
		Nonce:      nonce,
		Ciphertext: ciphertext,
		SessionID:  sessionID,
	}, nil
}

// Decrypt decrypts a ciphertext using AES-GCM
func (cm *CryptoManager) Decrypt(em *EncryptedMessage) ([]byte, error) {
	// Get the session key
	cm.mu.RLock()
	key, exists := cm.sessionKeys[em.SessionID]
	cm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("session key not found for session ID: %s", em.SessionID)
	}

	// Create cipher block
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create GCM cipher
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM cipher: %w", err)
	}

	// Check nonce size
	if len(em.Nonce) != gcm.NonceSize() {
		return nil, ErrInvalidNonceSize
	}

	// Decrypt
	plaintext, err := gcm.Open(nil, em.Nonce, em.Ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt: %w", err)
	}

	return plaintext, nil
}

// EncryptWithTimestamp encrypts a plaintext with a timestamp to prevent replay attacks
func (cm *CryptoManager) EncryptWithTimestamp(plaintext []byte, sessionID string, ttlSeconds int) (*EncryptedMessage, error) {
	// Add timestamp to plaintext
	timestamp := time.Now().Unix()
	timestampBytes := make([]byte, 8)
	binary.BigEndian.PutUint64(timestampBytes, uint64(timestamp))

	// Add TTL to plaintext
	ttlBytes := make([]byte, 4)
	binary.BigEndian.PutUint32(ttlBytes, uint32(ttlSeconds))

	// Combine timestamp, TTL and payload
	timestampedPlaintext := append(timestampBytes, ttlBytes...)
	timestampedPlaintext = append(timestampedPlaintext, plaintext...)

	// Encrypt with timestamp
	return cm.Encrypt(timestampedPlaintext, sessionID)
}

// DecryptWithTimestamp decrypts a ciphertext and validates its timestamp
func (cm *CryptoManager) DecryptWithTimestamp(em *EncryptedMessage) ([]byte, error) {
	// Decrypt
	plaintext, err := cm.Decrypt(em)
	if err != nil {
		return nil, err
	}

	// Message must have at least timestamp (8 bytes) and TTL (4 bytes)
	if len(plaintext) < 12 {
		return nil, errors.New("invalid timestamped message format")
	}

	// Extract timestamp and TTL
	timestamp := int64(binary.BigEndian.Uint64(plaintext[:8]))
	ttl := int(binary.BigEndian.Uint32(plaintext[8:12]))

	// Check if message is expired
	now := time.Now().Unix()
	if now > timestamp+int64(ttl) {
		return nil, errors.New("message expired")
	}

	// Check for replay (message from the future or too old)
	if now < timestamp || now > timestamp+int64(ttl) {
		return nil, errors.New("invalid message timestamp")
	}

	// Return actual payload (without timestamp and TTL)
	return plaintext[12:], nil
}
