package main

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	appName             = "codex-unpacker"
	packageName         = "OpenAI.Codex"
	packageArchitecture = "x64"
	packageFamilySuffix = "2p2nqsd0c76g0"
	packageExtension    = ".Msix"
	officialFeedURL     = "https://persistent.oaistatic.com/codex-app-prod/windows-store-update.json"
	mirrorLatestURL     = "https://api.github.com/repos/Wangnov/codex-app-mirror/releases/latest"
	statePath           = "data/latest.json"
)

var httpClient = &http.Client{Timeout: 45 * time.Minute}

type StoredState struct {
	SchemaVersion int            `json:"schemaVersion"`
	UpdatedAt     string         `json:"updatedAt"`
	Package       PackageDetails `json:"package"`
	Source        ResolvedSource `json:"source,omitempty"`
}

type PackageDetails struct {
	Name              string `json:"name"`
	Version           string `json:"version"`
	PackageMoniker    string `json:"packageMoniker"`
	PackageFamilyName string `json:"packageFamilyName"`
	Publisher         string `json:"publisher"`
	Architecture      string `json:"architecture"`
	SHA256            string `json:"sha256"`
	Size              int64  `json:"size"`
	FileName          string `json:"fileName"`
	Path              string `json:"-"`
}

type ResolvedSource struct {
	SourceKind       string `json:"sourceKind"`
	Version          string `json:"version"`
	PackageIdentity  string `json:"packageIdentity,omitempty"`
	StoreProductID   string `json:"storeProductId,omitempty"`
	AssetName        string `json:"assetName,omitempty"`
	Size             int64  `json:"size,omitempty"`
	DownloadURL      string `json:"downloadUrl,omitempty"`
	ChecksumURL      string `json:"checksumUrl,omitempty"`
	ExpectedSHA256   string `json:"expectedSha256,omitempty"`
	AssetDigest      string `json:"assetDigest,omitempty"`
	MirrorReleaseTag string `json:"mirrorReleaseTag,omitempty"`
	MirrorReleaseURL string `json:"mirrorReleaseUrl,omitempty"`
}

type ProbeResult struct {
	State              StoredState    `json:"state"`
	Source             ResolvedSource `json:"source"`
	DefaultDestination string         `json:"defaultDestination"`
	WouldUpdate        bool           `json:"wouldUpdate"`
}

type DownloadResult struct {
	Package     PackageDetails `json:"package"`
	Source      ResolvedSource `json:"source"`
	Destination string         `json:"destination"`
}

type InspectResult struct {
	Package      PackageDetails `json:"package"`
	MatchesState bool           `json:"matchesState"`
}

type officialManifest struct {
	SchemaVersion   int    `json:"schemaVersion"`
	BuildVersion    string `json:"buildVersion"`
	StoreProductID  string `json:"storeProductId"`
	PackageIdentity string `json:"packageIdentity"`
	DownloadURL     string `json:"downloadUrl,omitempty"`
	PackageURL      string `json:"packageUrl,omitempty"`
	MSIXURL         string `json:"msixUrl,omitempty"`
	URL             string `json:"url,omitempty"`
	AppInstallerURL string `json:"appInstallerUrl,omitempty"`
}

type mirrorRelease struct {
	TagName string        `json:"tag_name"`
	HTMLURL string        `json:"html_url"`
	Assets  []mirrorAsset `json:"assets"`
}

type mirrorAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Digest             string `json:"digest,omitempty"`
	Size               int64  `json:"size,omitempty"`
	ContentType        string `json:"content_type,omitempty"`
}

type appxManifest struct {
	Identity struct {
		Name                  string `xml:"Name,attr"`
		ProcessorArchitecture string `xml:"ProcessorArchitecture,attr"`
		Version               string `xml:"Version,attr"`
		Publisher             string `xml:"Publisher,attr"`
	} `xml:"Identity"`
}

// LoadState returns the cached local state if it exists.
func LoadState() (StoredState, error) {
	var state StoredState
	body, err := os.ReadFile(statePath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return StoredState{}, nil
		}
		return StoredState{}, err
	}
	if err := json.Unmarshal(body, &state); err != nil {
		return StoredState{}, err
	}
	return state, nil
}

func SaveState(state StoredState) error {
	if err := os.MkdirAll(filepath.Dir(statePath), 0o755); err != nil {
		return err
	}
	body, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return os.WriteFile(statePath, body, 0o644)
}

func ProbeLatest() (ProbeResult, error) {
	state, err := LoadState()
	if err != nil {
		return ProbeResult{}, err
	}
	source, err := resolveLatestSource()
	if err != nil {
		return ProbeResult{}, err
	}

	wouldUpdate := !strings.EqualFold(state.Package.Version, source.Version)
	if source.ExpectedSHA256 != "" && strings.EqualFold(state.Package.Version, source.Version) {
		wouldUpdate = !sameHash(state.Package.SHA256, source.ExpectedSHA256)
	}

	destination, err := defaultDownloadPath(source.AssetName)
	if err != nil {
		return ProbeResult{}, err
	}

	return ProbeResult{
		State:              state,
		Source:             source,
		DefaultDestination: destination,
		WouldUpdate:        wouldUpdate,
	}, nil
}

func DownloadLatest(target string) (DownloadResult, error) {
	source, err := resolveLatestSource()
	if err != nil {
		return DownloadResult{}, err
	}

	destination, err := resolveTargetPath(target, source.AssetName)
	if err != nil {
		return DownloadResult{}, err
	}

	pkg, err := downloadAndInspect(source, destination)
	if err != nil {
		return DownloadResult{}, err
	}

	state := StoredState{
		SchemaVersion: 1,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		Package:       pkg,
		Source:        source,
	}
	if err := SaveState(state); err != nil {
		return DownloadResult{}, err
	}

	return DownloadResult{
		Package:     pkg,
		Source:      source,
		Destination: destination,
	}, nil
}

func InspectLocal(path string) (InspectResult, error) {
	pkg, err := inspectMSIX(path, "")
	if err != nil {
		return InspectResult{}, err
	}
	state, err := LoadState()
	if err != nil {
		return InspectResult{}, err
	}

	matches := state.Package.Version != "" &&
		strings.EqualFold(state.Package.Version, pkg.Version) &&
		sameHash(state.Package.SHA256, pkg.SHA256)

	return InspectResult{
		Package:      pkg,
		MatchesState: matches,
	}, nil
}

func resolveLatestSource() (ResolvedSource, error) {
	manifest, err := fetchOfficialManifest()
	if err != nil {
		return ResolvedSource{}, err
	}
	if manifest.PackageIdentity != "" && !strings.EqualFold(manifest.PackageIdentity, packageName) {
		return ResolvedSource{}, fmt.Errorf("unexpected package identity %q", manifest.PackageIdentity)
	}

	version := strings.TrimSpace(manifest.BuildVersion)
	packageIdentity := strings.TrimSpace(manifest.PackageIdentity)
	storeProductID := strings.TrimSpace(manifest.StoreProductID)

	if direct := directPackageURLFromManifest(manifest); direct != "" {
		name := filepath.Base(direct)
		if version == "" {
			version = versionFromAssetName(name)
		}
		if version == "" {
			return ResolvedSource{}, fmt.Errorf("official manifest points to %q but the version could not be determined", direct)
		}
		return ResolvedSource{
			SourceKind:      "OfficialManifest",
			Version:         version,
			PackageIdentity: packageIdentity,
			StoreProductID:  storeProductID,
			AssetName:       name,
			DownloadURL:     direct,
		}, nil
	}

	release, err := fetchMirrorRelease()
	if err != nil {
		return ResolvedSource{}, err
	}

	asset, err := findMirrorAsset(release.Assets, version)
	if err != nil {
		return ResolvedSource{}, err
	}
	if version == "" {
		version = versionFromAssetName(asset.Name)
	}
	if version == "" {
		return ResolvedSource{}, fmt.Errorf("unable to infer Codex version from %q", asset.Name)
	}

	checksumURL, checksum := findWindowsChecksum(release.Assets, asset.Name)
	if checksum == "" {
		checksum = parseDigest(asset.Digest)
	}

	return ResolvedSource{
		SourceKind:       "MirrorRelease",
		Version:          version,
		PackageIdentity:  packageIdentity,
		StoreProductID:   storeProductID,
		AssetName:        asset.Name,
		Size:             asset.Size,
		DownloadURL:      asset.BrowserDownloadURL,
		ChecksumURL:      checksumURL,
		ExpectedSHA256:   checksum,
		AssetDigest:      parseDigest(asset.Digest),
		MirrorReleaseTag: release.TagName,
		MirrorReleaseURL: release.HTMLURL,
	}, nil
}

func fetchOfficialManifest() (officialManifest, error) {
	var manifest officialManifest
	if err := fetchJSON(officialFeedURL, &manifest); err != nil {
		return officialManifest{}, fmt.Errorf("official manifest fetch failed: %w", err)
	}
	return manifest, nil
}

func fetchMirrorRelease() (mirrorRelease, error) {
	var release mirrorRelease
	if err := fetchJSON(mirrorLatestURL, &release); err != nil {
		return mirrorRelease{}, fmt.Errorf("mirror release fetch failed: %w", err)
	}
	return release, nil
}

func directPackageURLFromManifest(manifest officialManifest) string {
	for _, candidate := range []string{
		manifest.DownloadURL,
		manifest.PackageURL,
		manifest.MSIXURL,
		manifest.URL,
	} {
		if looksLikePackageURL(candidate) {
			return candidate
		}
	}
	return ""
}

func looksLikePackageURL(candidate string) bool {
	lower := strings.ToLower(strings.TrimSpace(candidate))
	switch {
	case strings.HasSuffix(lower, ".msix"):
		return true
	case strings.HasSuffix(lower, ".appx"):
		return true
	case strings.HasSuffix(lower, ".msixbundle"):
		return true
	case strings.HasSuffix(lower, ".appxbundle"):
		return true
	default:
		return false
	}
}

func findMirrorAsset(assets []mirrorAsset, version string) (mirrorAsset, error) {
	if version != "" {
		want := packageAssetName(version)
		for _, asset := range assets {
			if strings.EqualFold(asset.Name, want) {
				return asset, nil
			}
		}
	}

	for _, asset := range assets {
		if versionFromAssetName(asset.Name) != "" && strings.EqualFold(filepath.Ext(asset.Name), packageExtension) {
			return asset, nil
		}
	}

	want := packageAssetName(version)
	if version == "" {
		want = packageName + "_" + packageArchitecture + "__" + packageFamilySuffix + packageExtension
	}
	return mirrorAsset{}, fmt.Errorf("mirror release is missing the Windows MSIX asset %q", want)
}

func findWindowsChecksum(assets []mirrorAsset, fileName string) (string, string) {
	for _, asset := range assets {
		if strings.EqualFold(asset.Name, "SHA256SUMS-windows.txt") {
			body, err := fetchText(asset.BrowserDownloadURL)
			if err != nil {
				return asset.BrowserDownloadURL, ""
			}
			if hash := parseChecksum(body, fileName); hash != "" {
				return asset.BrowserDownloadURL, hash
			}
			return asset.BrowserDownloadURL, ""
		}
	}
	return "", ""
}

func downloadAndInspect(source ResolvedSource, destination string) (PackageDetails, error) {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return PackageDetails{}, err
	}

	tempDir, err := os.MkdirTemp("", "codex-unpacker-*")
	if err != nil {
		return PackageDetails{}, err
	}
	defer os.RemoveAll(tempDir)

	tempPath := filepath.Join(tempDir, filepath.Base(destination))
	if err := downloadFile(source.DownloadURL, tempPath); err != nil {
		return PackageDetails{}, err
	}

	pkg, err := inspectMSIX(tempPath, source.Version)
	if err != nil {
		return PackageDetails{}, err
	}

	expectedHash := source.ExpectedSHA256
	if expectedHash == "" {
		expectedHash = parseDigest(source.AssetDigest)
	}
	if expectedHash != "" && !sameHash(pkg.SHA256, expectedHash) {
		return PackageDetails{}, fmt.Errorf("downloaded package hash mismatch: expected %s, got %s", expectedHash, pkg.SHA256)
	}

	if err := copyFile(tempPath, destination); err != nil {
		return PackageDetails{}, err
	}
	pkg.Path = destination
	pkg.FileName = filepath.Base(destination)
	return pkg, nil
}

func inspectMSIX(path string, expectedVersion string) (PackageDetails, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return PackageDetails{}, err
	}

	reader, err := zip.OpenReader(abs)
	if err != nil {
		return PackageDetails{}, err
	}
	defer reader.Close()

	required := map[string]bool{
		"AppxManifest.xml":               false,
		"AppxBlockMap.xml":               false,
		"AppxSignature.p7x":              false,
		"AppxMetadata/CodeIntegrity.cat": false,
	}

	var manifestBytes []byte
	for _, file := range reader.File {
		name := strings.ReplaceAll(file.Name, "\\", "/")
		if _, ok := required[name]; ok {
			required[name] = true
		}
		if name == "AppxManifest.xml" {
			rc, err := file.Open()
			if err != nil {
				return PackageDetails{}, err
			}
			manifestBytes, err = io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				return PackageDetails{}, err
			}
		}
	}

	for name, seen := range required {
		if !seen {
			return PackageDetails{}, fmt.Errorf("MSIX is missing %s", name)
		}
	}
	if len(manifestBytes) == 0 {
		return PackageDetails{}, errors.New("MSIX is missing AppxManifest.xml")
	}

	var manifest appxManifest
	if err := xml.Unmarshal(manifestBytes, &manifest); err != nil {
		return PackageDetails{}, err
	}
	if !strings.EqualFold(manifest.Identity.Name, packageName) {
		return PackageDetails{}, fmt.Errorf("unexpected package identity %s", manifest.Identity.Name)
	}
	if manifest.Identity.ProcessorArchitecture != "" && !strings.EqualFold(manifest.Identity.ProcessorArchitecture, packageArchitecture) {
		return PackageDetails{}, fmt.Errorf("unexpected architecture %s", manifest.Identity.ProcessorArchitecture)
	}
	if expectedVersion != "" && manifest.Identity.Version != expectedVersion {
		return PackageDetails{}, fmt.Errorf("expected version %s, got %s", expectedVersion, manifest.Identity.Version)
	}

	sum, size, err := hashFile(abs)
	if err != nil {
		return PackageDetails{}, err
	}

	fileName := filepath.Base(abs)
	moniker := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	if version := versionFromAssetName(fileName); version != "" && version != manifest.Identity.Version {
		return PackageDetails{}, fmt.Errorf("filename version %s does not match manifest version %s", version, manifest.Identity.Version)
	}

	return PackageDetails{
		Name:              manifest.Identity.Name,
		Version:           manifest.Identity.Version,
		PackageMoniker:    moniker,
		PackageFamilyName: packageName + "_" + packageFamilySuffix,
		Publisher:         manifest.Identity.Publisher,
		Architecture:      manifest.Identity.ProcessorArchitecture,
		SHA256:            sum,
		Size:              size,
		FileName:          fileName,
		Path:              abs,
	}, nil
}

func defaultDownloadPath(fileName string) (string, error) {
	downloads, err := downloadsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(downloads, resolvedFileName(fileName)), nil
}

func downloadsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Downloads"), nil
}

func resolveTargetPath(target string, defaultName string) (string, error) {
	if strings.TrimSpace(target) == "" {
		return defaultDownloadPath(defaultName)
	}

	cleaned := filepath.Clean(target)
	if strings.HasSuffix(target, string(os.PathSeparator)) || strings.HasSuffix(target, "/") || strings.HasSuffix(target, `\`) {
		return filepath.Join(cleaned, resolvedFileName(defaultName)), nil
	}
	if info, err := os.Stat(cleaned); err == nil && info.IsDir() {
		return filepath.Join(cleaned, resolvedFileName(defaultName)), nil
	}
	if filepath.Ext(filepath.Base(cleaned)) == "" {
		return filepath.Join(cleaned, resolvedFileName(defaultName)), nil
	}
	return cleaned, nil
}

func resolvedFileName(candidate string) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return packageAssetName("")
	}
	if looksLikePackageURL(candidate) {
		return filepath.Base(candidate)
	}
	if strings.HasSuffix(strings.ToLower(candidate), strings.ToLower(packageExtension)) {
		return filepath.Base(candidate)
	}
	if version := versionFromAssetName(candidate); version != "" {
		return candidate
	}
	if strings.Contains(candidate, "_") && strings.Contains(candidate, packageFamilySuffix) {
		return candidate
	}
	return packageAssetName(candidate)
}

func packageAssetName(version string) string {
	if version == "" {
		return packageName + "_latest_" + packageArchitecture + "__" + packageFamilySuffix + packageExtension
	}
	return fmt.Sprintf("%s_%s_%s__%s%s", packageName, version, packageArchitecture, packageFamilySuffix, packageExtension)
}

func versionFromAssetName(name string) string {
	base := filepath.Base(name)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	prefix := packageName + "_"
	suffix := "_" + packageArchitecture + "__" + packageFamilySuffix
	if !strings.HasPrefix(base, prefix) || !strings.HasSuffix(base, suffix) {
		return ""
	}
	return strings.TrimSuffix(strings.TrimPrefix(base, prefix), suffix)
}

func parseChecksum(body, fileName string) string {
	for _, line := range strings.Split(body, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		hash := strings.ToLower(strings.TrimSpace(fields[0]))
		name := strings.TrimSpace(fields[len(fields)-1])
		name = strings.TrimPrefix(name, "*")
		if len(hash) == 64 && strings.EqualFold(name, fileName) {
			return hash
		}
	}
	return ""
}

func parseDigest(digest string) string {
	return strings.TrimPrefix(strings.ToLower(strings.TrimSpace(digest)), "sha256:")
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()

	hash := sha256.New()
	size, err := io.Copy(hash, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func fetchJSON(url string, out any) error {
	body, err := fetchBytes(url)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func fetchText(url string) (string, error) {
	body, err := fetchBytes(url)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func fetchBytes(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", appName+"/1.0")
	req.Header.Set("Accept", "application/json, text/plain, */*")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, url, strings.TrimSpace(string(snippet)))
	}
	return io.ReadAll(resp.Body)
}

func downloadFile(url, path string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", appName+"/1.0")

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, url, strings.TrimSpace(string(snippet)))
	}

	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

func sameHash(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == "" || b == "" {
		return false
	}
	return strings.EqualFold(a, b)
}
