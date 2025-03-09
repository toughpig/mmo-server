package security

import (
	"bytes"
	"testing"
	"time"
)

func TestCryptoManager_GenerateSessionKey(t *testing.T) {
	// Create a new crypto manager
	cm, err := NewCryptoManager(&CryptoConfig{
		KeyRotationInterval: int64(time.Hour / time.Second),
	})
	if err != nil {
		t.Fatalf("Failed to create crypto manager: %v", err)
	}

	// Generate a session key
	sessionID := "test-session"
	key, err := cm.GenerateSessionKey(sessionID)
	if err != nil {
		t.Fatalf("Failed to generate session key: %v", err)
	}

	// Verify key length
	if len(key) != KeySize {
		t.Errorf("Expected key size to be %d, got %d", KeySize, len(key))
	}

	// Verify key is stored in the manager
	storedKey, exists := cm.GetSessionKey(sessionID)
	if !exists {
		t.Errorf("Session key not found in manager")
	}
	if !bytes.Equal(key, storedKey) {
		t.Errorf("Stored key does not match generated key")
	}
}

func TestCryptoManager_DeriveSharedKey(t *testing.T) {
	// Create two crypto managers (for client and server)
	clientCM, err := NewCryptoManager(&CryptoConfig{
		KeyRotationInterval: int64(time.Hour / time.Second),
	})
	if err != nil {
		t.Fatalf("Failed to create client crypto manager: %v", err)
	}

	serverCM, err := NewCryptoManager(&CryptoConfig{
		KeyRotationInterval: int64(time.Hour / time.Second),
	})
	if err != nil {
		t.Fatalf("Failed to create server crypto manager: %v", err)
	}

	// Get public keys
	clientPubKey := clientCM.GetPublicKeyBytes()
	serverPubKey := serverCM.GetPublicKeyBytes()

	// Derive shared keys
	sessionID := "test-session"
	clientKey, err := clientCM.DeriveSharedKey(sessionID, serverPubKey)
	if err != nil {
		t.Fatalf("Failed to derive client shared key: %v", err)
	}

	serverKey, err := serverCM.DeriveSharedKey(sessionID, clientPubKey)
	if err != nil {
		t.Fatalf("Failed to derive server shared key: %v", err)
	}

	// Verify shared keys are the same
	if !bytes.Equal(clientKey, serverKey) {
		t.Errorf("Shared keys do not match")
	}
}

func TestCryptoManager_Encrypt_Decrypt(t *testing.T) {
	// Create a new crypto manager
	cm, err := NewCryptoManager(&CryptoConfig{
		KeyRotationInterval: int64(time.Hour / time.Second),
	})
	if err != nil {
		t.Fatalf("Failed to create crypto manager: %v", err)
	}

	// Generate a session key
	sessionID := "test-session"
	_, err = cm.GenerateSessionKey(sessionID)
	if err != nil {
		t.Fatalf("Failed to generate session key: %v", err)
	}

	// Test messages
	testMessages := [][]byte{
		[]byte("Hello, World!"),
		[]byte("This is a test message."),
		[]byte{0x00, 0x01, 0x02, 0x03, 0x04}, // Binary data
		make([]byte, 1024),                   // 1KB of zeros
	}

	for i, plaintext := range testMessages {
		// Encrypt
		em, err := cm.Encrypt(plaintext, sessionID)
		if err != nil {
			t.Fatalf("Failed to encrypt message %d: %v", i, err)
		}

		// Decrypt
		decrypted, err := cm.Decrypt(em)
		if err != nil {
			t.Fatalf("Failed to decrypt message %d: %v", i, err)
		}

		// Verify decrypted message matches original
		if !bytes.Equal(plaintext, decrypted) {
			t.Errorf("Message %d: Decrypted message does not match original", i)
		}
	}
}

func TestCryptoManager_EncryptWithTimestamp_DecryptWithTimestamp(t *testing.T) {
	// Create a new crypto manager
	cm, err := NewCryptoManager(&CryptoConfig{
		KeyRotationInterval: int64(time.Hour / time.Second),
	})
	if err != nil {
		t.Fatalf("Failed to create crypto manager: %v", err)
	}

	// Generate a session key
	sessionID := "test-session"
	_, err = cm.GenerateSessionKey(sessionID)
	if err != nil {
		t.Fatalf("Failed to generate session key: %v", err)
	}

	// Test message
	plaintext := []byte("Test message with timestamp")
	ttl := 60 // 60 seconds

	// Encrypt with timestamp
	em, err := cm.EncryptWithTimestamp(plaintext, sessionID, ttl)
	if err != nil {
		t.Fatalf("Failed to encrypt message with timestamp: %v", err)
	}

	// Decrypt with timestamp validation
	decrypted, err := cm.DecryptWithTimestamp(em)
	if err != nil {
		t.Fatalf("Failed to decrypt message with timestamp: %v", err)
	}

	// Verify decrypted message matches original
	if !bytes.Equal(plaintext, decrypted) {
		t.Errorf("Decrypted message with timestamp does not match original")
	}
}

func TestCryptoManager_RotateKeys(t *testing.T) {
	// Create a new crypto manager with short rotation period
	cm, err := NewCryptoManager(&CryptoConfig{
		KeyRotationInterval: 1, // 1 second minimum
	})
	if err != nil {
		t.Fatalf("Failed to create crypto manager: %v", err)
	}

	// Get initial public key
	initialPubKey := cm.GetPublicKeyBytes()

	// Wait for key rotation to occur
	time.Sleep(10 * time.Millisecond)

	// Force key rotation
	err = cm.RotateKeys()
	if err != nil {
		t.Fatalf("Failed to rotate keys: %v", err)
	}

	// Get new public key
	newPubKey := cm.GetPublicKeyBytes()

	// Verify keys are different
	if bytes.Equal(initialPubKey, newPubKey) {
		t.Errorf("Public key did not change after rotation")
	}
}

func TestCryptoManager_RemoveSessionKey(t *testing.T) {
	// Create a new crypto manager
	cm, err := NewCryptoManager(&CryptoConfig{
		KeyRotationInterval: int64(time.Hour / time.Second),
	})
	if err != nil {
		t.Fatalf("Failed to create crypto manager: %v", err)
	}

	// Generate a session key
	sessionID := "test-session"
	_, err = cm.GenerateSessionKey(sessionID)
	if err != nil {
		t.Fatalf("Failed to generate session key: %v", err)
	}

	// Verify key exists
	_, exists := cm.GetSessionKey(sessionID)
	if !exists {
		t.Fatalf("Session key not found in manager")
	}

	// Remove session key
	cm.RemoveSessionKey(sessionID)

	// Verify key no longer exists
	_, exists = cm.GetSessionKey(sessionID)
	if exists {
		t.Errorf("Session key still exists after removal")
	}
}
