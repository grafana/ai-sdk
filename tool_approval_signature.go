package aisdk

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

func signToolApproval(secret []byte, approvalID, toolCallID, toolName string, input any) (string, error) {
	inputDigest, err := hashCanonical(input)
	if err != nil {
		return "", err
	}
	payload, err := approvalPayload(approvalID, toolCallID, toolName, inputDigest)
	if err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(payload)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func verifyToolApprovalSignature(secret []byte, signature, approvalID, toolCallID, toolName string, input any) (bool, error) {
	inputDigest, err := hashCanonical(input)
	if err != nil {
		return false, err
	}
	want, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return false, nil
	}
	payload, err := approvalPayload(approvalID, toolCallID, toolName, inputDigest)
	if err != nil {
		return false, err
	}
	if validToolApprovalSignature(secret, want, payload) {
		return true, nil
	}
	if strings.Contains(approvalID, "\n") || strings.Contains(toolCallID, "\n") || strings.Contains(toolName, "\n") {
		return false, nil
	}
	return validToolApprovalSignature(secret, want, legacyApprovalPayload(approvalID, toolCallID, toolName, inputDigest)), nil
}

func maybeSignToolApproval(secret []byte, req *ToolApprovalRequest) error {
	if len(secret) == 0 {
		return nil
	}
	signature, err := signToolApproval(secret, req.ApprovalID, req.ToolCallID, req.ToolName, req.Input)
	if err != nil {
		return fmt.Errorf("signing tool approval: %w", err)
	}
	req.Signature = signature
	return nil
}

func approvalPayload(approvalID, toolCallID, toolName, inputDigest string) ([]byte, error) {
	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode([]string{"ai-sdk-tool-approval-v1", approvalID, toolCallID, toolName, inputDigest}); err != nil {
		return nil, fmt.Errorf("encoding tool approval payload: %w", err)
	}
	payload := bytes.TrimSuffix(buf.Bytes(), []byte("\n"))
	payload = bytes.ReplaceAll(payload, []byte(`\u2028`), []byte("\u2028"))
	payload = bytes.ReplaceAll(payload, []byte(`\u2029`), []byte("\u2029"))
	return payload, nil
}

func legacyApprovalPayload(approvalID, toolCallID, toolName, inputDigest string) []byte {
	return []byte(approvalID + "\n" + toolCallID + "\n" + toolName + "\n" + inputDigest)
}

func validToolApprovalSignature(secret, signature, payload []byte) bool {
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(payload)
	return hmac.Equal(signature, mac.Sum(nil))
}

func hashCanonical(value any) (string, error) {
	canonical, err := canonicalJSONString(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(canonical))
	return base64.RawURLEncoding.EncodeToString(sum[:]), nil
}
