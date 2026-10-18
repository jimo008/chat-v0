package http

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

func testDeviceKey(t *testing.T) (*ecdsa.PrivateKey, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return key, base64.StdEncoding.EncodeToString(der)
}

func signTestDevice(t *testing.T, r *http.Request, body []byte, key *ecdsa.PrivateKey, deviceID string, at time.Time) {
	t.Helper()
	r.Header.Set("X-Chat-Device-ID", deviceID)
	r.Header.Set("X-Chat-Device-Time", strconv.FormatInt(at.Unix(), 10))
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		t.Fatal(err)
	}
	r.Header.Set("X-Chat-Device-Nonce", hex.EncodeToString(nonce))
	digest := sha256.Sum256(deviceProofMessage(r, body))
	signature, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("X-Chat-Device-Signature", base64.StdEncoding.EncodeToString(signature))
}

func TestDeviceProofProtocolVector(t *testing.T) {
	body := []byte(`{"content":"hello"}`)
	r := httptest.NewRequest(http.MethodPost, "https://support.example/api/v1/agent/conversations/1/messages?after_seq=42", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer test-token")
	r.Header.Set("X-Chat-Device-ID", "android-test-device")
	r.Header.Set("X-Chat-Device-Time", "1792288800")
	r.Header.Set("X-Chat-Device-Nonce", "00112233445566778899aabbccddeeff")
	expected, err := os.ReadFile("../../tests/device_proof_message.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(deviceProofMessage(r, body)) != strings.TrimSuffix(string(expected), "\n") {
		t.Fatal("device proof protocol differs from the Android shared vector")
	}
}

func TestDeviceSignatureRejectsTampering(t *testing.T) {
	key, public := testDeviceKey(t)
	now := time.Now()
	body := []byte(`{"content":"hello"}`)
	for _, change := range []string{"none", "body", "token", "path", "device", "key", "timestamp"} {
		t.Run(change, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "https://support.example/api/v1/agent/conversations/1/messages?after_seq=42", bytes.NewReader(body))
			r.Header.Set("Authorization", "Bearer test-token")
			signTestDevice(t, r, body, key, "android-test-device", now)
			payload, encodedKey := body, public
			switch change {
			case "body":
				payload = []byte(`{"content":"changed"}`)
			case "token":
				r.Header.Set("Authorization", "Bearer different-token")
			case "path":
				r.URL.RawQuery = "after_seq=43"
			case "device":
				r.Header.Set("X-Chat-Device-ID", "different-device")
			case "key":
				_, encodedKey = testDeviceKey(t)
			case "timestamp":
				r.Header.Set("X-Chat-Device-Time", "not-a-time")
			}
			err := checkDeviceSignature(r, payload, encodedKey, "android-test-device", now)
			if change == "none" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			var proof *deviceProofError
			if !errors.As(err, &proof) || !proof.ban {
				t.Fatalf("tampering was not classified as invalid proof: %v", err)
			}
		})
	}
	r := httptest.NewRequest(http.MethodGet, "https://support.example/api/v1/agent/me", nil)
	signTestDevice(t, r, nil, key, "android-test-device", now.Add(-10*time.Minute))
	var proof *deviceProofError
	if err := checkDeviceSignature(r, nil, public, "android-test-device", now); !errors.As(err, &proof) || proof.ban {
		t.Fatalf("a genuine device clock error must not ban its network: %v", err)
	}
}
