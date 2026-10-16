package auth

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// SourceStaticKey identifies a boot-configured local credential.
const SourceStaticKey Source = "static-key"

// StaticIdentity binds a credential digest to a local attribution name.
type StaticIdentity struct {
	Name      string
	KeyDigest [sha256.Size]byte
}

type staticKeyAuthenticator struct{ identities []StaticIdentity }

var localIdentityName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// ValidStaticKey checks bounded RFC 6750 bearer-token syntax without normalization.
func ValidStaticKey(value string) bool {
	if len(value) < 1 || len(value) > 4096 {
		return false
	}
	padding := false
	for i := 0; i < len(value); i++ {
		c := value[i]
		if c == '=' {
			if i == 0 {
				return false
			}
			padding = true
			continue
		}
		valid := (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || strings.ContainsRune("._~+/-", rune(c))
		if padding || !valid {
			return false
		}
	}
	return true
}

// NewStaticKeyAuthenticator copies validated local identities and retains only digests.
func NewStaticKeyAuthenticator(identities []StaticIdentity) (RequestAuthenticator, error) {
	if len(identities) < 1 || len(identities) > 64 {
		return nil, fmt.Errorf("gateway auth: static identities must contain 1-64 entries")
	}
	names := make(map[string]bool, len(identities))
	digests := make(map[[sha256.Size]byte]bool, len(identities))
	for _, identity := range identities {
		if !localIdentityName.MatchString(identity.Name) || names[identity.Name] || digests[identity.KeyDigest] {
			return nil, fmt.Errorf("gateway auth: invalid or duplicate static identity")
		}
		names[identity.Name] = true
		digests[identity.KeyDigest] = true
	}
	return staticKeyAuthenticator{identities: append([]StaticIdentity(nil), identities...)}, nil
}

func (authenticator staticKeyAuthenticator) Authenticate(_ context.Context, headers http.Header) (Caller, error) {
	for name := range headers {
		if strings.EqualFold(name, "X-Grafana-Id") || strings.EqualFold(name, "X-Scope-OrgID") || strings.EqualFold(name, "X-Cloud-Org-ID") || strings.EqualFold(name, "X-Access-Policy-ID") {
			return Caller{}, fmt.Errorf("gateway auth: invalid static credential")
		}
	}
	access, err := exactlyOneHeader(headers, "X-Access-Token", false)
	if err != nil {
		return Caller{}, err
	}
	authorization, err := exactlyOneHeader(headers, "Authorization", false)
	if err != nil {
		return Caller{}, err
	}
	if (access == "") == (authorization == "") {
		return Caller{}, fmt.Errorf("gateway auth: invalid static credential")
	}
	if authorization != "" {
		if len(authorization) > 4096+len("Bearer ") {
			return Caller{}, fmt.Errorf("gateway auth: invalid static credential")
		}
		scheme, token, ok := strings.Cut(authorization, " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") {
			return Caller{}, fmt.Errorf("gateway auth: invalid static credential")
		}
		access = token
	}
	digest, valid := StaticKeyDigest(access)
	if !valid {
		return Caller{}, fmt.Errorf("gateway auth: invalid static credential")
	}
	matched := 0
	index := 0
	for i, identity := range authenticator.identities {
		equal := subtle.ConstantTimeCompare(digest[:], identity.KeyDigest[:])
		index = subtle.ConstantTimeSelect(equal, i, index)
		matched |= equal
	}
	if matched != 1 {
		return Caller{}, fmt.Errorf("gateway auth: invalid static credential")
	}
	name := authenticator.identities[index].Name
	return Caller{Source: SourceStaticKey, Service: name, Subject: "local:" + name, access: ConfiguredAccounts}, nil
}

// StaticKeyDigest validates a key and clears its owned temporary hashing buffer.
// It does not erase the source string or copies held by the runtime or hash implementation.
func StaticKeyDigest(value string) ([sha256.Size]byte, bool) {
	if !ValidStaticKey(value) {
		return [sha256.Size]byte{}, false
	}
	var buffer [4096]byte
	size := copy(buffer[:], value)
	return digestAndClear(buffer[:size]), true
}

func digestAndClear(buffer []byte) [sha256.Size]byte {
	defer clear(buffer)
	return sha256.Sum256(buffer)
}
