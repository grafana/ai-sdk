package aisdk

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"

	"github.com/grafana/ai-sdk/provider"
)

const maxGeneratedFileBytes int64 = 2 * 1024 * 1024 * 1024

var generatedFileEmbeddedIPv4Networks = []netip.Prefix{
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("::ffff:0:0:0/96"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
}

var blockedGeneratedFileNetworks = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("127.0.0.0/8"),
	netip.MustParsePrefix("169.254.0.0/16"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("192.168.0.0/16"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("224.0.0.0/4"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("fc00::/7"),
	netip.MustParsePrefix("fe80::/10"),
	netip.MustParsePrefix("fec0::/10"),
	netip.MustParsePrefix("ff00::/8"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("3fff::/20"),
}

func resolveGeneratedFile(ctx context.Context, data *provider.StreamFileData, mediaType string) (GeneratedFile, error) {
	file := generatedFileFromStreamData(data, mediaType)
	if data == nil {
		return file, nil
	}
	if err := data.Validate(); err != nil {
		return GeneratedFile{}, fmt.Errorf("aisdk: invalid generated file data: %w", err)
	}
	if data.Type != provider.StreamFileDataTypeURL && data.URL == "" {
		return file, nil
	}
	bytes, err := downloadGeneratedFile(ctx, data.URL)
	if err != nil {
		return GeneratedFile{}, fmt.Errorf("aisdk: downloading generated file: %w", err)
	}
	file.Data = bytes
	file.Base64 = ""
	return file, nil
}

func downloadGeneratedFile(ctx context.Context, rawURL string) ([]byte, error) {
	client := newGeneratedFileClient(net.DefaultResolver.LookupIPAddr, (&net.Dialer{}).DialContext)
	defer client.CloseIdleConnections()
	return downloadGeneratedFileWithClient(ctx, rawURL, client)
}

func downloadGeneratedFileWithClient(ctx context.Context, rawURL string, client *http.Client) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid generated file URL: %w", err)
	}
	if parsed.Scheme == "data" {
		bytes, err := decodeGeneratedFileDataURL(rawURL)
		if err != nil {
			return nil, err
		}
		return bytes, ctx.Err()
	}
	if err := validateGeneratedFileURL(parsed); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating generated file request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching generated file: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	return readGeneratedFileResponse(resp, maxGeneratedFileBytes)
}

func readGeneratedFileResponse(resp *http.Response, limit int64) ([]byte, error) {
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("generated file request returned HTTP %d", resp.StatusCode)
	}
	if resp.ContentLength > limit {
		return nil, fmt.Errorf("generated file exceeds maximum size of %d bytes", limit)
	}
	bytes, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, fmt.Errorf("reading generated file: %w", err)
	}
	if int64(len(bytes)) > limit {
		return nil, fmt.Errorf("generated file exceeds maximum size of %d bytes", limit)
	}
	return bytes, nil
}

func decodeGeneratedFileDataURL(rawURL string) ([]byte, error) {
	meta, data, ok := strings.Cut(strings.TrimPrefix(rawURL, "data:"), ",")
	if !ok {
		return nil, fmt.Errorf("invalid generated file data URL")
	}
	maxEncodedSize := maxGeneratedFileBytes * 3
	if strings.HasSuffix(strings.ToLower(meta), ";base64") {
		maxEncodedSize = maxGeneratedFileBytes*4 + 4
	}
	if int64(len(data)) > maxEncodedSize {
		return nil, fmt.Errorf("generated file exceeds maximum size of %d bytes", maxGeneratedFileBytes)
	}
	decoded, err := url.PathUnescape(data)
	if err != nil {
		return nil, fmt.Errorf("decoding generated file data URL: %w", err)
	}
	if strings.HasSuffix(strings.ToLower(meta), ";base64") {
		bytes, err := base64.StdEncoding.DecodeString(decoded)
		if err != nil {
			bytes, err = base64.RawStdEncoding.DecodeString(decoded)
			if err != nil {
				return nil, fmt.Errorf("decoding generated file base64: %w", err)
			}
		}
		if int64(len(bytes)) > maxGeneratedFileBytes {
			return nil, fmt.Errorf("generated file exceeds maximum size of %d bytes", maxGeneratedFileBytes)
		}
		return bytes, nil
	}
	if int64(len(decoded)) > maxGeneratedFileBytes {
		return nil, fmt.Errorf("generated file exceeds maximum size of %d bytes", maxGeneratedFileBytes)
	}
	return []byte(decoded), nil
}

func validateGeneratedFileURL(parsed *url.URL) error {
	if parsed.User != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" {
		return fmt.Errorf("generated file URL is not a credential-free HTTP(S) URL")
	}
	host := strings.TrimRight(strings.ToLower(parsed.Hostname()), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return fmt.Errorf("generated file URL hostname is not public")
	}
	if address, err := netip.ParseAddr(host); err == nil && !isPublicGeneratedFileAddress(address) {
		return fmt.Errorf("generated file URL address is not public")
	}
	return nil
}

func isPublicGeneratedFileAddress(address netip.Addr) bool {
	address = address.Unmap()
	if !address.IsGlobalUnicast() || address.IsPrivate() {
		return false
	}
	for _, network := range blockedGeneratedFileNetworks {
		if network.Contains(address) {
			return false
		}
	}
	if address.Is6() {
		for _, network := range generatedFileEmbeddedIPv4Networks {
			if network.Contains(address) {
				bytes := address.As16()
				return isPublicGeneratedFileAddress(netip.AddrFrom4([4]byte{bytes[12], bytes[13], bytes[14], bytes[15]}))
			}
		}
	}
	return true
}

func newGeneratedFileClient(lookup func(context.Context, string) ([]net.IPAddr, error), dial func(context.Context, string, string) (net.Conn, error)) *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				return nil, err
			}
			addresses, err := lookup(ctx, host)
			if err != nil {
				return nil, fmt.Errorf("resolving generated file host: %w", err)
			}
			if len(addresses) == 0 {
				return nil, fmt.Errorf("generated file host has no addresses")
			}
			for _, candidate := range addresses {
				parsed, ok := netip.AddrFromSlice(candidate.IP)
				if !ok || !isPublicGeneratedFileAddress(parsed) {
					return nil, fmt.Errorf("generated file host resolved to a disallowed address")
				}
			}
			return dial(ctx, network, net.JoinHostPort(addresses[0].IP.String(), port))
		},
	}
	return &http.Client{
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 10 {
				return fmt.Errorf("generated file exceeded 10 redirects")
			}
			return validateGeneratedFileURL(req.URL)
		},
	}
}
