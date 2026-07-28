package aisdk

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToolApprovalSignature_InjectivePayload(t *testing.T) {
	t.Run("matches upstream JSON payload", func(t *testing.T) {
		signature, err := signToolApproval(
			[]byte("secret"),
			"apr<\u2028",
			"call\t1",
			"name\"\\",
			map[string]any{"b": 2, "a": 1},
		)
		require.NoError(t, err)
		assert.Equal(t, "VOMet8mr-T4bwRmGaKmegABRxv1Q_BnLlFZ-YqoU7cI", signature)
	})

	t.Run("rejects retupled fields", func(t *testing.T) {
		input := map[string]any{"path": "/tmp/target"}
		signature, err := signToolApproval([]byte("secret"), "approval-1", "call-1", "searchDocs\ndeleteFile", input)
		require.NoError(t, err)

		valid, err := verifyToolApprovalSignature([]byte("secret"), signature, "approval-1", "call-1\nsearchDocs", "deleteFile", input)
		require.NoError(t, err)
		assert.False(t, valid)
	})
}

func TestToolApprovalSignature_LegacyVerification(t *testing.T) {
	input := map[string]any{"path": "/tmp/target"}

	t.Run("accepts delimiter-free payload", func(t *testing.T) {
		signature := signLegacyToolApproval(t, []byte("secret"), "approval-1", "call-1", "searchDocs", input)
		valid, err := verifyToolApprovalSignature([]byte("secret"), signature, "approval-1", "call-1", "searchDocs", input)
		require.NoError(t, err)
		assert.True(t, valid)
	})

	t.Run("rejects newline-bearing payload", func(t *testing.T) {
		signature := signLegacyToolApproval(t, []byte("secret"), "approval-1", "call-1", "searchDocs\ndeleteFile", input)
		valid, err := verifyToolApprovalSignature([]byte("secret"), signature, "approval-1", "call-1\nsearchDocs", "deleteFile", input)
		require.NoError(t, err)
		assert.False(t, valid)
	})
}

func signLegacyToolApproval(t *testing.T, secret []byte, approvalID, toolCallID, toolName string, input any) string {
	t.Helper()
	inputDigest, err := hashCanonical(input)
	require.NoError(t, err)
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(legacyApprovalPayload(approvalID, toolCallID, toolName, inputDigest))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
