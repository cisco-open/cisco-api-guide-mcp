// SPDX-License-Identifier: Apache-2.0

// Copyright 2026 Cisco Systems, Inc. and their affiliates

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

// http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package modules

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cisco-open/cisco-api-guide-mcp/internal/db"
)

const (
	// DefaultRegistryURL points to the public release manifest.
	DefaultRegistryURL = "https://github.com/cisco-open/cisco-api-guide-mcp/releases/download/data-modules-latest/modules.json"
)

// FetcherOptions configures the ModuleFetcher.
type FetcherOptions struct {
	DataDir     string
	RegistryURL string
	HTTPClient  *http.Client
}

// ModuleFetcher manages downloading, caching, and verifying module databases.
type ModuleFetcher struct {
	dataDir     string
	registryURL string
	client      *http.Client
}

// NewModuleFetcher returns a new fetcher configured with local storage path.
func NewModuleFetcher(opts FetcherOptions) (*ModuleFetcher, error) {
	dataDir := opts.DataDir
	if dataDir == "" {
		cacheDir, err := os.UserCacheDir()
		if err != nil {
			dataDir = filepath.Join(os.TempDir(), "cisco-api-guide")
		} else {
			dataDir = filepath.Join(cacheDir, "cisco-api-guide")
		}
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("create data dir %q: %w", dataDir, err)
	}

	regURL := opts.RegistryURL
	if regURL == "" {
		regURL = DefaultRegistryURL
	}

	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 60 * time.Second}
	}

	return &ModuleFetcher{
		dataDir:     dataDir,
		registryURL: regURL,
		client:      client,
	}, nil
}

// DataDir returns the path where module SQLite databases are stored.
func (f *ModuleFetcher) DataDir() string {
	return f.dataDir
}

// ModuleDBPath returns the local filepath for a module database.
func (f *ModuleFetcher) ModuleDBPath(moduleKey string) string {
	return filepath.Join(f.dataDir, moduleKey+".db")
}

// FetchManifest retrieves the remote manifest.
func (f *ModuleFetcher) FetchManifest() (*Manifest, error) {
	resp, err := f.client.Get(f.registryURL)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest from %s: %w", f.registryURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch manifest returned HTTP %d: %s", resp.StatusCode, resp.Status)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read manifest body: %w", err)
	}

	return LoadManifest(data)
}

// EnsureModules ensures the requested module keys are present locally and verified.
// If requested is empty or contains "all", all modules in the manifest are downloaded.
func (f *ModuleFetcher) EnsureModules(requested []string, autoUpdate bool) ([]string, error) {
	manifest, err := f.FetchManifest()
	if err != nil {
		// If remote manifest fails, check what's already locally available
		localFiles, findErr := f.ListLocalModules()
		if findErr == nil && len(localFiles) > 0 {
			return localFiles, nil
		}
		return nil, fmt.Errorf("could not fetch manifest and no local modules found: %w", err)
	}

	var targets []string
	loadAll := len(requested) == 0
	reqSet := make(map[string]bool)
	for _, r := range requested {
		r = strings.TrimSpace(strings.ToLower(r))
		if r == "all" || r == "" {
			loadAll = true
			break
		}
		reqSet[r] = true
	}

	for key, info := range manifest.Modules {
		if loadAll || reqSet[key] || reqSet[strings.ToLower(info.ProductID)] {
			targets = append(targets, key)
		} else {
			// Check aliases
			for _, alias := range info.Aliases {
				if reqSet[strings.ToLower(alias)] {
					targets = append(targets, key)
					break
				}
			}
		}
	}

	if len(targets) == 0 {
		return nil, fmt.Errorf("no matching modules found in manifest for %v", requested)
	}

	var loadedPaths []string
	for _, key := range targets {
		info, ok := manifest.Modules[key]
		if !ok {
			continue
		}
		dbPath := f.ModuleDBPath(key)
		needsDownload := false

		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			needsDownload = true
		} else if autoUpdate && info.SHA256 != "" {
			// Verify hash
			hash, err := fileSHA256(dbPath)
			if err != nil || hash != info.SHA256 {
				needsDownload = true
			}
		}

		if needsDownload {
			if err := f.downloadModule(key, info); err != nil {
				return nil, fmt.Errorf("download module %q: %w", key, err)
			}
		}

		loadedPaths = append(loadedPaths, dbPath)
	}

	return loadedPaths, nil
}

// ListLocalModules finds all *.db files in dataDir.
func (f *ModuleFetcher) ListLocalModules() ([]string, error) {
	entries, err := os.ReadDir(f.dataDir)
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".db") {
			paths = append(paths, filepath.Join(f.dataDir, e.Name()))
		}
	}
	return paths, nil
}

// LoadIntoManager loads all specified paths or local modules into the DB Manager.
func (f *ModuleFetcher) LoadIntoManager(mgr *db.Manager, paths []string) error {
	for _, p := range paths {
		if err := mgr.LoadDBFile(p); err != nil {
			return fmt.Errorf("load db file %s: %w", p, err)
		}
	}
	return nil
}

func (f *ModuleFetcher) downloadModule(key string, info ModuleInfo) error {
	if info.URL == "" {
		return fmt.Errorf("module %q has no download URL", key)
	}

	resp, err := f.client.Get(info.URL)
	if err != nil {
		return fmt.Errorf("get %s: %w", info.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s failed: HTTP %d %s", info.URL, resp.StatusCode, resp.Status)
	}

	tmpFile, err := os.CreateTemp(f.dataDir, key+"-download-*.tmp")
	if err != nil {
		return fmt.Errorf("create temp download file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	var reader io.Reader = resp.Body
	if strings.HasSuffix(info.URL, ".gz") {
		gzReader, err := gzip.NewReader(resp.Body)
		if err != nil {
			tmpFile.Close()
			return fmt.Errorf("init gzip reader: %w", err)
		}
		defer gzReader.Close()
		reader = gzReader
	}

	if _, err := io.Copy(tmpFile, reader); err != nil {
		tmpFile.Close()
		return fmt.Errorf("write unpacked db: %w", err)
	}
	tmpFile.Close()

	// Verify SHA256 of extracted db if provided
	if info.SHA256 != "" {
		hash, err := fileSHA256(tmpFile.Name())
		if err != nil {
			return fmt.Errorf("calculate sha256: %w", err)
		}
		if hash != info.SHA256 {
			return fmt.Errorf("sha256 mismatch for module %q: expected %s, got %s", key, info.SHA256, hash)
		}
	}

	destPath := f.ModuleDBPath(key)
	if err := os.Rename(tmpFile.Name(), destPath); err != nil {
		return fmt.Errorf("move to %s: %w", destPath, err)
	}

	return nil
}

func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
