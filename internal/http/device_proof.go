package http

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type deviceProofError struct {
	code string
	ban  bool
}

func (e *deviceProofError) Error() string { return e.code }

func parseDevicePublicKey(encoded string) (*ecdsa.PublicKey, error) {
	if len(encoded) > 512 {
		return nil, errors.New("invalid device public key")
	}
	der, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, err
	}
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return nil, err
	}
	key, ok := parsed.(*ecdsa.PublicKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, errors.New("device key must be P-256")
	}
	return key, nil
}

func deviceProofMessage(r *http.Request, body []byte) []byte {
	sum := sha256.Sum256(body)
	return []byte(strings.Join([]string{
		"chat-v0-device-v1", r.Header.Get("X-Chat-Device-ID"),
		r.Header.Get("X-Chat-Device-Time"), r.Header.Get("X-Chat-Device-Nonce"),
		r.Method, r.URL.RequestURI(), r.Header.Get("Authorization"), hex.EncodeToString(sum[:]),
	}, "\n"))
}

func checkDeviceSignature(r *http.Request, body []byte, publicKey, deviceID string, now time.Time) error {
	if r.Header.Get("X-Chat-Device-ID") != deviceID || len(deviceID) == 0 {
		return &deviceProofError{"invalid_device_proof", true}
	}
	if r.Header.Get("X-Chat-Device-Time") == "" || r.Header.Get("X-Chat-Device-Signature") == "" {
		return &deviceProofError{"invalid_device_proof", true}
	}
	nonce := r.Header.Get("X-Chat-Device-Nonce")
	if len(nonce) != 32 {
		return &deviceProofError{"invalid_device_proof", true}
	}
	if _, err := hex.DecodeString(nonce); err != nil {
		return &deviceProofError{"invalid_device_proof", true}
	}
	key, err := parseDevicePublicKey(publicKey)
	if err != nil {
		return &deviceProofError{"invalid_device_proof", true}
	}
	signature, err := base64.StdEncoding.DecodeString(r.Header.Get("X-Chat-Device-Signature"))
	if err != nil || len(signature) > 80 {
		return &deviceProofError{"invalid_device_proof", true}
	}
	digest := sha256.Sum256(deviceProofMessage(r, body))
	if !ecdsa.VerifyASN1(key, digest[:], signature) {
		return &deviceProofError{"invalid_device_proof", true}
	}
	timestamp, err := strconv.ParseInt(r.Header.Get("X-Chat-Device-Time"), 10, 64)
	if err != nil || timestamp < now.Unix()-120 || timestamp > now.Unix()+120 {
		return &deviceProofError{"device_clock_out_of_sync", false}
	}
	return nil
}

func (s *Server) preflightDeviceLogin(w http.ResponseWriter, r *http.Request, req agentLoginRequest, body []byte) bool {
	if strings.TrimSpace(req.PairingCode) == "" {
		s.rejectSecurity(w, r, "pairing_code_required")
		return false
	}
	if req.PublicKey == "" {
		s.rejectSecurity(w, r, "device_pairing_required")
		return false
	}
	if err := checkDeviceSignature(r, body, req.PublicKey, req.DeviceID, time.Now()); err != nil {
		s.writeAgentAuthError(w, r, err)
		return false
	}
	return true
}

func readProofBody(r *http.Request) ([]byte, error) {
	if r.Body == nil {
		return nil, nil
	}
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, (32<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(body) > 32<<20 {
		return nil, errors.New("request body too large")
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, nil
}

func (s *Server) verifyDeviceProof(r *http.Request, body []byte, publicKey, deviceID string) error {
	if err := checkDeviceSignature(r, body, publicKey, deviceID, time.Now()); err != nil {
		return err
	}
	nonceKey := "chat-v0:device-nonce:" + deviceID + ":" + r.Header.Get("X-Chat-Device-Nonce")
	accepted, err := s.redis.SetNX(r.Context(), nonceKey, "1", 5*time.Minute).Result()
	if err != nil {
		return fmt.Errorf("device nonce store: %w", err)
	}
	if !accepted {
		return &deviceProofError{"device_request_replayed", false}
	}
	return nil
}

func (s *Server) verifyAuthenticatedDevice(r *http.Request, agent authenticatedAgent) error {
	policy, err := s.loadSecurityPolicy(r.Context())
	if err != nil {
		return err
	}
	if agent.PublicKey == "" {
		if policy.RequireDeviceProof {
			return &deviceProofError{"device_pairing_required", true}
		}
		return nil
	}
	body, err := readProofBody(r)
	if err != nil {
		return err
	}
	return s.verifyDeviceProof(r, body, agent.PublicKey, agent.DeviceKey)
}

func (s *Server) writeAgentAuthError(w http.ResponseWriter, r *http.Request, err error) {
	var proof *deviceProofError
	if errors.As(err, &proof) {
		if proof.ban {
			s.rejectSecurity(w, r, proof.code)
		} else {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": proof.code})
		}
		return
	}
	writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "device_verification_unavailable"})
}

func (s *Server) rejectUnknownAgent(w http.ResponseWriter, r *http.Request) bool {
	policy, err := s.loadSecurityPolicy(r.Context())
	if err != nil {
		s.writeAgentAuthError(w, r, err)
		return true
	}
	if !policy.RequireDeviceProof {
		return false
	}
	var publicKey string
	err = s.db.QueryRowContext(r.Context(), `SELECT public_key FROM agent_devices WHERE device_id = ? AND public_key IS NOT NULL AND public_key <> '' LIMIT 1`, r.Header.Get("X-Chat-Device-ID")).Scan(&publicKey)
	if err == sql.ErrNoRows {
		s.rejectSecurity(w, r, "unpaired_device")
		return true
	}
	if err != nil {
		s.writeAgentAuthError(w, r, err)
		return true
	}
	body, err := readProofBody(r)
	if err == nil {
		err = s.verifyDeviceProof(r, body, publicKey, r.Header.Get("X-Chat-Device-ID"))
	}
	if err != nil {
		s.writeAgentAuthError(w, r, err)
		return true
	}
	return false
}
