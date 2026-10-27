package candidate

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/mod/module"
	"golang.org/x/mod/sumdb/dirhash"
	modzip "golang.org/x/mod/zip"
)

const licenseFile = "LICENSE"

// Module identifies a first-party Go module by its repository directory and
// module path. The root module uses the directory ".".
type Module struct {
	Dir  string
	Path string
}

// Checksums holds the go.sum hashes of one module version.
type Checksums struct {
	Zip   string
	GoMod string
}

// Proxy is a file-based GOPROXY directory that serves candidate module
// versions built from a repository revision.
type Proxy struct {
	Dir string
}

// URL returns the proxy as a GOPROXY list entry.
func (p Proxy) URL() string { return "file://" + filepath.ToSlash(p.Dir) }

// Add builds the module archive of m at version from revision rev of the Git
// repository at repo, stores it in the proxy and returns its checksums. The
// archive holds the files the Go command includes for the matching tag: tracked
// regular files of the module directory without nested modules, plus the
// repository root LICENSE for a nested module that has none of its own.
func (p Proxy) Add(repo, rev string, m Module, version string) (Checksums, error) {
	dir, err := os.MkdirTemp("", "candidate-module-")
	if err != nil {
		return Checksums{}, fmt.Errorf("candidate: creating module directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	if err := extract(repo, rev, m.Dir, dir); err != nil {
		return Checksums{}, err
	}
	moduleDir := filepath.Join(dir, filepath.FromSlash(m.Dir))
	if err := addRootLicense(repo, rev, m.Dir, moduleDir); err != nil {
		return Checksums{}, err
	}
	goMod, err := os.ReadFile(filepath.Join(moduleDir, "go.mod"))
	if err != nil {
		return Checksums{}, fmt.Errorf("candidate: reading %s go.mod at %s: %w", m.Path, rev, err)
	}

	versionDir, err := p.versionDir(m.Path)
	if err != nil {
		return Checksums{}, err
	}
	zipPath := filepath.Join(versionDir, version+".zip")
	if err := writeZip(zipPath, module.Version{Path: m.Path, Version: version}, moduleDir); err != nil {
		return Checksums{}, err
	}
	if err := os.WriteFile(filepath.Join(versionDir, version+".mod"), goMod, 0o644); err != nil {
		return Checksums{}, fmt.Errorf("candidate: writing %s@%s go.mod: %w", m.Path, version, err)
	}
	if err := writeInfo(repo, rev, versionDir, version); err != nil {
		return Checksums{}, err
	}
	if err := appendVersion(filepath.Join(versionDir, "list"), version); err != nil {
		return Checksums{}, err
	}

	zipHash, err := dirhash.HashZip(zipPath, dirhash.Hash1)
	if err != nil {
		return Checksums{}, fmt.Errorf("candidate: hashing %s@%s: %w", m.Path, version, err)
	}
	modHash, err := dirhash.Hash1([]string{"go.mod"}, func(string) (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(goMod)), nil
	})
	if err != nil {
		return Checksums{}, fmt.Errorf("candidate: hashing %s@%s go.mod: %w", m.Path, version, err)
	}
	return Checksums{Zip: zipHash, GoMod: modHash}, nil
}

func (p Proxy) versionDir(modulePath string) (string, error) {
	escaped, err := module.EscapePath(modulePath)
	if err != nil {
		return "", fmt.Errorf("candidate: escaping %s: %w", modulePath, err)
	}
	dir := filepath.Join(p.Dir, filepath.FromSlash(escaped), "@v")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("candidate: creating proxy directory for %s: %w", modulePath, err)
	}
	return dir, nil
}

func extract(repo, rev, moduleDir, destination string) error {
	args := []string{"-C", repo, "archive", "--format=tar", rev}
	if moduleDir != "." {
		args = append(args, "--", moduleDir)
	}
	cmd := exec.Command("git", args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("candidate: archiving %s: %w", rev, err)
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("candidate: archiving %s: %w", rev, err)
	}
	extractErr := untarRegular(stdout, destination)
	_, _ = io.Copy(io.Discard, stdout)
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("candidate: archiving %s %s: %w: %s", rev, moduleDir, err, strings.TrimSpace(stderr.String()))
	}
	return extractErr
}

func untarRegular(r io.Reader, destination string) error {
	archive := tar.NewReader(r)
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("candidate: reading archive: %w", err)
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		name := path.Clean(header.Name)
		if !filepath.IsLocal(name) {
			return fmt.Errorf("candidate: archive entry %q escapes the module directory", header.Name)
		}
		target := filepath.Join(destination, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("candidate: creating %s: %w", filepath.Dir(target), err)
		}
		file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("candidate: creating %s: %w", target, err)
		}
		_, copyErr := io.Copy(file, archive)
		closeErr := file.Close()
		if err := errors.Join(copyErr, closeErr); err != nil {
			return fmt.Errorf("candidate: writing %s: %w", target, err)
		}
	}
}

func addRootLicense(repo, rev, moduleDir, destination string) error {
	if moduleDir == "." {
		return nil
	}
	target := filepath.Join(destination, licenseFile)
	if _, err := os.Stat(target); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("candidate: checking %s: %w", target, err)
	}
	license, err := gitOutput(repo, "show", rev+":"+licenseFile)
	if err != nil {
		return fmt.Errorf("candidate: reading root %s at %s: %w", licenseFile, rev, err)
	}
	if err := os.WriteFile(target, license, 0o644); err != nil {
		return fmt.Errorf("candidate: writing %s: %w", target, err)
	}
	return nil
}

func writeZip(zipPath string, version module.Version, dir string) error {
	file, err := os.Create(zipPath)
	if err != nil {
		return fmt.Errorf("candidate: creating %s: %w", zipPath, err)
	}
	createErr := modzip.CreateFromDir(file, version, dir)
	closeErr := file.Close()
	if err := errors.Join(createErr, closeErr); err != nil {
		return fmt.Errorf("candidate: building %s: %w", version, err)
	}
	return nil
}

func writeInfo(repo, rev, versionDir, version string) error {
	committed, err := gitOutput(repo, "log", "-1", "--format=%cI", rev)
	if err != nil {
		return fmt.Errorf("candidate: reading commit time of %s: %w", rev, err)
	}
	info, err := json.Marshal(struct {
		Version string
		Time    string
	}{Version: version, Time: strings.TrimSpace(string(committed))})
	if err != nil {
		return fmt.Errorf("candidate: encoding %s info: %w", version, err)
	}
	if err := os.WriteFile(filepath.Join(versionDir, version+".info"), info, 0o644); err != nil {
		return fmt.Errorf("candidate: writing %s info: %w", version, err)
	}
	return nil
}

func appendVersion(listPath, version string) error {
	content, err := os.ReadFile(listPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("candidate: reading %s: %w", listPath, err)
	}
	if slices.Contains(strings.Fields(string(content)), version) {
		return nil
	}
	file, err := os.OpenFile(listPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("candidate: opening %s: %w", listPath, err)
	}
	_, writeErr := fmt.Fprintln(file, version)
	closeErr := file.Close()
	if err := errors.Join(writeErr, closeErr); err != nil {
		return fmt.Errorf("candidate: writing %s: %w", listPath, err)
	}
	return nil
}

func gitOutput(repo string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}
