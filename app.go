package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	defaultUpdateManifestURL = "https://persistent.oaistatic.com/codex-app-prod/windows-store-update.json"
	mirrorLatestAPI          = "https://api.github.com/repos/Wangnov/codex-app-mirror/releases/latest"
	packageIdentity          = "OpenAI.Codex"
	packageFamilySuffix      = "2p2nqsd0c76g0"
	architecture             = "x64"
	statePath                = "data/latest.json"
	releaseTagPrefix         = "codex-unpacker"
)

type App struct {
	ctx    context.Context
	client *http.Client
}

type AppStatus struct {
	Repo          string `json:"repo"`
	RepoPrivate   bool   `json:"repoPrivate"`
	GhAvailable   bool   `json:"ghAvailable"`
	GhAuthed      bool   `json:"ghAuthed"`
	StateVersion  string `json:"stateVersion"`
	StateHash     string `json:"stateHash"`
	WorkingFolder string `json:"workingFolder"`
}

type ProbeResult struct {
	SourceKind            string `json:"sourceKind"`
	UpdateManifestVersion string `json:"updateManifestVersion"`
	PackageVersion        string `json:"packageVersion"`
	PackageMoniker        string `json:"packageMoniker"`
	DownloadURL           string `json:"downloadUrl"`
	ExpectedSHA256        string `json:"expectedSha256"`
	MirrorReleaseTag      string `json:"mirrorReleaseTag"`
	MirrorReleaseURL      string `json:"mirrorReleaseUrl"`
	DirectStoreStatus     string `json:"directStoreStatus"`
	WouldUpdate           bool   `json:"wouldUpdate"`
	CurrentStateVersion   string `json:"currentStateVersion"`
	CurrentStateSHA256    string `json:"currentStateSha256"`
}

type PublishResult struct {
	Mode       string `json:"mode"`
	Version    string `json:"version"`
	SHA256     string `json:"sha256"`
	ReleaseTag string `json:"releaseTag"`
	ReleaseURL string `json:"releaseUrl"`
	Message    string `json:"message"`
}

type PackageInfo struct {
	Name              string `json:"name"`
	Version           string `json:"version"`
	PackageMoniker    string `json:"packageMoniker"`
	PackageFamilyName string `json:"packageFamilyName"`
	Publisher         string `json:"publisher"`
	Architecture      string `json:"architecture"`
	SHA256            string `json:"sha256"`
	Size              int64  `json:"size"`
	FileName          string `json:"fileName"`
	Path              string `json:"path"`
}

type updateManifest struct {
	SchemaVersion   int    `json:"schemaVersion"`
	BuildVersion    string `json:"buildVersion"`
	StoreProductID  string `json:"storeProductId"`
	PackageIdentity string `json:"packageIdentity"`
}

type mirrorRelease struct {
	TagName string        `json:"tag_name"`
	HTMLURL string        `json:"html_url"`
	Assets  []mirrorAsset `json:"assets"`
}

type mirrorAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

type mirrorManifest struct {
	Sources struct {
		Windows struct {
			ProductID         string `json:"productId"`
			Architecture      string `json:"architecture"`
			Version           string `json:"version"`
			PackageMoniker    string `json:"packageMoniker"`
			UpdateManifestURL string `json:"updateManifestUrl"`
			UpdateManifest    struct {
				BuildVersion    string `json:"buildVersion"`
				StoreProductID  string `json:"storeProductId"`
				PackageIdentity string `json:"packageIdentity"`
			} `json:"updateManifest"`
		} `json:"windows"`
	} `json:"sources"`
}

type latestState struct {
	SchemaVersion int       `json:"schemaVersion"`
	UpdatedAt     string    `json:"updatedAt"`
	Package       statePack `json:"package"`
	Source        any       `json:"source,omitempty"`
	Release       stateRel  `json:"release"`
}

type statePack struct {
	Name              string `json:"name"`
	Version           string `json:"version"`
	PackageMoniker    string `json:"packageMoniker"`
	PackageFamilyName string `json:"packageFamilyName,omitempty"`
	Publisher         string `json:"publisher"`
	SHA256            string `json:"sha256"`
	Size              int64  `json:"size"`
	FileName          string `json:"fileName,omitempty"`
	SourceKind        string `json:"sourceKind,omitempty"`
}

type stateRel struct {
	Tag string `json:"tag"`
	ID  string `json:"id"`
	URL string `json:"url"`
}

type appxManifest struct {
	Identity struct {
		Name                  string `xml:"Name,attr"`
		ProcessorArchitecture string `xml:"ProcessorArchitecture,attr"`
		Version               string `xml:"Version,attr"`
		Publisher             string `xml:"Publisher,attr"`
	} `xml:"Identity"`
}

func NewApp() *App {
	return &App{client: &http.Client{Timeout: 120 * time.Second}}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) GetStatus() (AppStatus, error) {
	wd, _ := os.Getwd()
	state, _ := readState()
	repo, private := repoInfo()

	return AppStatus{
		Repo:          repo,
		RepoPrivate:   private,
		GhAvailable:   commandExists("gh"),
		GhAuthed:      ghAuthed(),
		StateVersion:  state.Package.Version,
		StateHash:     state.Package.SHA256,
		WorkingFolder: wd,
	}, nil
}

func (a *App) ProbeLatest() (ProbeResult, error) {
	a.log("Checking OpenAI Windows update manifest")
	source, err := a.resolveLatest()
	if err != nil {
		return ProbeResult{}, err
	}
	state, _ := readState()
	wouldUpdate := state.Package.Version != source.PackageVersion || !strings.EqualFold(state.Package.SHA256, source.ExpectedSHA256)
	if source.ExpectedSHA256 == "" {
		wouldUpdate = state.Package.Version != source.PackageVersion
	}
	return ProbeResult{
		SourceKind:            source.SourceKind,
		UpdateManifestVersion: source.UpdateManifestVersion,
		PackageVersion:        source.PackageVersion,
		PackageMoniker:        source.PackageMoniker,
		DownloadURL:           source.DownloadURL,
		ExpectedSHA256:        source.ExpectedSHA256,
		MirrorReleaseTag:      source.MirrorReleaseTag,
		MirrorReleaseURL:      source.MirrorReleaseURL,
		DirectStoreStatus:     source.DirectStoreStatus,
		WouldUpdate:           wouldUpdate,
		CurrentStateVersion:   state.Package.Version,
		CurrentStateSHA256:    state.Package.SHA256,
	}, nil
}

func (a *App) ChooseLocalMSIX() (PackageInfo, error) {
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Choose Codex MSIX",
		Filters: []runtime.FileFilter{
			{DisplayName: "MSIX packages", Pattern: "*.msix;*.appx"},
		},
	})
	if err != nil {
		return PackageInfo{}, err
	}
	if path == "" {
		return PackageInfo{}, nil
	}
	return inspectMSIX(path, "")
}

func (a *App) DryRunLocal(path string) (PublishResult, error) {
	info, err := inspectMSIX(path, "")
	if err != nil {
		return PublishResult{}, err
	}
	state, _ := readState()
	mode := "Would publish"
	msg := "Package is different from data/latest.json."
	if state.Package.Version == info.Version && strings.EqualFold(state.Package.SHA256, info.SHA256) {
		mode = "No update"
		msg = "Package matches data/latest.json."
	}
	return PublishResult{Mode: mode, Version: info.Version, SHA256: info.SHA256, Message: msg}, nil
}

func (a *App) PublishLatest(force bool) (PublishResult, error) {
	source, err := a.resolveLatest()
	if err != nil {
		return PublishResult{}, err
	}
	state, _ := readState()
	if !force && state.Package.Version == source.PackageVersion && strings.EqualFold(state.Package.SHA256, source.ExpectedSHA256) {
		return PublishResult{Mode: "No update", Version: state.Package.Version, SHA256: state.Package.SHA256, Message: "Latest package already matches data/latest.json."}, nil
	}

	tmp, err := os.MkdirTemp("", "codex-unpacker-*")
	if err != nil {
		return PublishResult{}, err
	}
	defer os.RemoveAll(tmp)

	target := filepath.Join(tmp, source.PackageMoniker+".Msix")
	a.log("Downloading " + source.PackageMoniker)
	if err := a.downloadFile(source.DownloadURL, target); err != nil {
		return PublishResult{}, err
	}
	info, err := inspectMSIX(target, source.PackageVersion)
	if err != nil {
		return PublishResult{}, err
	}
	if source.ExpectedSHA256 != "" && !strings.EqualFold(info.SHA256, source.ExpectedSHA256) {
		return PublishResult{}, fmt.Errorf("downloaded package hash mismatch: expected %s, got %s", source.ExpectedSHA256, info.SHA256)
	}
	return a.publishPackage(info, source, force)
}

func (a *App) PublishLocal(path string, force bool) (PublishResult, error) {
	info, err := inspectMSIX(path, "")
	if err != nil {
		return PublishResult{}, err
	}
	source := resolvedSource{
		SourceKind:            "LocalMsix",
		PackageVersion:        info.Version,
		PackageMoniker:        info.PackageMoniker,
		DownloadURL:           "local:" + info.FileName,
		UpdateManifestVersion: info.Version,
	}
	state, _ := readState()
	if !force && state.Package.Version == info.Version && strings.EqualFold(state.Package.SHA256, info.SHA256) {
		return PublishResult{Mode: "No update", Version: info.Version, SHA256: info.SHA256, Message: "Local package already matches data/latest.json."}, nil
	}
	return a.publishPackage(info, source, force)
}

type resolvedSource struct {
	SourceKind            string `json:"sourceKind"`
	UpdateManifestVersion string `json:"updateManifestVersion,omitempty"`
	PackageVersion        string `json:"packageVersion"`
	PackageMoniker        string `json:"packageMoniker"`
	DownloadURL           string `json:"downloadUrl"`
	ExpectedSHA256        string `json:"expectedSha256,omitempty"`
	MirrorReleaseTag      string `json:"mirrorReleaseTag,omitempty"`
	MirrorReleaseURL      string `json:"mirrorReleaseUrl,omitempty"`
	DirectStoreStatus     string `json:"directStoreStatus,omitempty"`
}

func (a *App) resolveLatest() (resolvedSource, error) {
	var manifest updateManifest
	if err := a.getJSON(defaultUpdateManifestURL, &manifest); err != nil {
		return resolvedSource{}, fmt.Errorf("update manifest failed: %w", err)
	}
	if manifest.PackageIdentity != "" && manifest.PackageIdentity != packageIdentity {
		return resolvedSource{}, fmt.Errorf("unexpected package identity %q", manifest.PackageIdentity)
	}

	source, err := a.resolveMirror(manifest.BuildVersion)
	if err != nil {
		return resolvedSource{}, err
	}
	source.DirectStoreStatus = "Microsoft Store direct resolver is not embedded in this GUI yet; using validated mirror release source."
	if manifest.BuildVersion != "" && source.UpdateManifestVersion == "" {
		source.UpdateManifestVersion = manifest.BuildVersion
	}
	if manifest.BuildVersion != "" && source.PackageVersion != manifest.BuildVersion {
		a.log(fmt.Sprintf("Official manifest advertises %s; mirror currently serves %s", manifest.BuildVersion, source.PackageVersion))
	}
	return source, nil
}

func (a *App) resolveMirror(officialVersion string) (resolvedSource, error) {
	var release mirrorRelease
	if err := a.getJSON(mirrorLatestAPI, &release); err != nil {
		return resolvedSource{}, fmt.Errorf("mirror release lookup failed: %w", err)
	}
	manifestAsset := findAsset(release.Assets, "release-manifest.json")
	checksumAsset := findAsset(release.Assets, "SHA256SUMS-windows.txt")
	msixAsset := findMSIXAsset(release.Assets)
	if manifestAsset == nil || msixAsset == nil {
		return resolvedSource{}, errors.New("mirror release is missing the Windows manifest or MSIX asset")
	}

	var mm mirrorManifest
	if err := a.getJSON(manifestAsset.BrowserDownloadURL, &mm); err != nil {
		return resolvedSource{}, fmt.Errorf("mirror manifest failed: %w", err)
	}
	expectedHash := ""
	if checksumAsset != nil {
		body, err := a.getText(checksumAsset.BrowserDownloadURL)
		if err == nil {
			expectedHash = parseChecksum(body, msixAsset.Name)
		}
	}
	version := mm.Sources.Windows.Version
	if version == "" {
		version = versionFromMoniker(mm.Sources.Windows.PackageMoniker)
	}
	advertised := mm.Sources.Windows.UpdateManifest.BuildVersion
	if advertised == "" {
		advertised = officialVersion
	}
	return resolvedSource{
		SourceKind:            "MirrorRelease",
		UpdateManifestVersion: advertised,
		PackageVersion:        version,
		PackageMoniker:        strings.TrimSuffix(msixAsset.Name, filepath.Ext(msixAsset.Name)),
		DownloadURL:           msixAsset.BrowserDownloadURL,
		ExpectedSHA256:        expectedHash,
		MirrorReleaseTag:      release.TagName,
		MirrorReleaseURL:      release.HTMLURL,
	}, nil
}

func (a *App) publishPackage(info PackageInfo, source resolvedSource, force bool) (PublishResult, error) {
	if !commandExists("gh") {
		return PublishResult{}, errors.New("GitHub CLI is not installed or not in PATH")
	}
	if !ghAuthed() {
		return PublishResult{}, errors.New("GitHub CLI is not authenticated; run gh auth login first")
	}
	repo, _ := repoInfo()
	if repo == "" {
		return PublishResult{}, errors.New("could not determine GitHub repository")
	}
	commit, err := gitOutput("rev-parse", "HEAD")
	if err != nil {
		return PublishResult{}, err
	}

	tmp, err := os.MkdirTemp("", "codex-release-*")
	if err != nil {
		return PublishResult{}, err
	}
	defer os.RemoveAll(tmp)

	tag := releaseTag(info.Version, info.SHA256, force)
	packageAsset := filepath.Join(tmp, info.FileName)
	if err := copyFile(info.Path, packageAsset); err != nil {
		return PublishResult{}, err
	}
	shaPath := filepath.Join(tmp, "SHA256SUMS.txt")
	if err := os.WriteFile(shaPath, []byte(fmt.Sprintf("%s  %s\r\n", info.SHA256, info.FileName)), 0644); err != nil {
		return PublishResult{}, err
	}
	manifestPath := filepath.Join(tmp, "release.json")
	if err := writeJSON(manifestPath, releaseManifest(info, source, tag, "", "")); err != nil {
		return PublishResult{}, err
	}
	notesPath := filepath.Join(tmp, "release-notes.md")
	if err := os.WriteFile(notesPath, []byte(releaseNotes(info, source)), 0644); err != nil {
		return PublishResult{}, err
	}

	a.log("Creating GitHub release " + tag)
	args := []string{"release", "create", tag, "--repo", repo, "--target", strings.TrimSpace(commit), "--title", "Codex MSIX " + info.Version, "--notes-file", notesPath, packageAsset, shaPath, manifestPath}
	if out, err := runCommand("gh", args...); err != nil {
		return PublishResult{}, fmt.Errorf("gh release create failed: %w\n%s", err, out)
	}
	view, err := runCommand("gh", "release", "view", tag, "--repo", repo, "--json", "id,url")
	if err != nil {
		return PublishResult{}, fmt.Errorf("release was created, but gh release view failed: %w", err)
	}
	var release struct {
		ID  json.Number `json:"id"`
		URL string      `json:"url"`
	}
	_ = json.Unmarshal([]byte(view), &release)

	state := latestState{
		SchemaVersion: 1,
		UpdatedAt:     time.Now().UTC().Format(time.RFC3339),
		Package: statePack{
			Name:              info.Name,
			Version:           info.Version,
			PackageMoniker:    info.PackageMoniker,
			PackageFamilyName: info.PackageFamilyName,
			Publisher:         info.Publisher,
			SHA256:            info.SHA256,
			Size:              info.Size,
			FileName:          info.FileName,
			SourceKind:        source.SourceKind,
		},
		Source: source,
		Release: stateRel{
			Tag: tag,
			ID:  release.ID.String(),
			URL: release.URL,
		},
	}
	if err := writeJSON(statePath, state); err != nil {
		return PublishResult{}, err
	}
	_, _ = runCommand("git", "add", statePath)
	_, _ = runCommand("git", "commit", "-m", "Update mirrored Codex package state")
	_, _ = runCommand("git", "push")

	return PublishResult{Mode: "Published", Version: info.Version, SHA256: info.SHA256, ReleaseTag: tag, ReleaseURL: release.URL, Message: "Release created and local state updated."}, nil
}

func inspectMSIX(path string, expectedVersion string) (PackageInfo, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return PackageInfo{}, err
	}
	zr, err := zip.OpenReader(abs)
	if err != nil {
		return PackageInfo{}, err
	}
	defer zr.Close()

	required := map[string]bool{
		"AppxManifest.xml":                false,
		"AppxBlockMap.xml":                false,
		"AppxSignature.p7x":               false,
		"AppxMetadata/CodeIntegrity.cat":  false,
		"AppxMetadata\\CodeIntegrity.cat": false,
	}
	var manifestBytes []byte
	for _, f := range zr.File {
		name := strings.ReplaceAll(f.Name, "\\", "/")
		if _, ok := required[name]; ok {
			required[name] = true
		}
		if name == "AppxManifest.xml" {
			rc, err := f.Open()
			if err != nil {
				return PackageInfo{}, err
			}
			manifestBytes, err = io.ReadAll(rc)
			_ = rc.Close()
			if err != nil {
				return PackageInfo{}, err
			}
		}
	}
	for name, seen := range required {
		if strings.Contains(name, "\\") {
			continue
		}
		if !seen {
			return PackageInfo{}, fmt.Errorf("MSIX is missing %s", name)
		}
	}
	var manifest appxManifest
	if err := xml.Unmarshal(manifestBytes, &manifest); err != nil {
		return PackageInfo{}, err
	}
	if manifest.Identity.Name != packageIdentity {
		return PackageInfo{}, fmt.Errorf("unexpected package identity %s", manifest.Identity.Name)
	}
	if manifest.Identity.ProcessorArchitecture != "" && !strings.EqualFold(manifest.Identity.ProcessorArchitecture, architecture) {
		return PackageInfo{}, fmt.Errorf("unexpected architecture %s", manifest.Identity.ProcessorArchitecture)
	}
	if expectedVersion != "" && manifest.Identity.Version != expectedVersion {
		return PackageInfo{}, fmt.Errorf("expected version %s, got %s", expectedVersion, manifest.Identity.Version)
	}
	hash, size, err := hashFile(abs)
	if err != nil {
		return PackageInfo{}, err
	}
	fileName := filepath.Base(abs)
	moniker := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	if versionFromMoniker(moniker) != "" && versionFromMoniker(moniker) != manifest.Identity.Version {
		return PackageInfo{}, fmt.Errorf("filename version %s does not match manifest version %s", versionFromMoniker(moniker), manifest.Identity.Version)
	}
	return PackageInfo{
		Name:              manifest.Identity.Name,
		Version:           manifest.Identity.Version,
		PackageMoniker:    moniker,
		PackageFamilyName: manifest.Identity.Name + "_" + packageFamilySuffix,
		Publisher:         manifest.Identity.Publisher,
		Architecture:      manifest.Identity.ProcessorArchitecture,
		SHA256:            hash,
		Size:              size,
		FileName:          fileName,
		Path:              abs,
	}, nil
}

func (a *App) getJSON(url string, out any) error {
	body, err := a.getBytes(url)
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func (a *App) getText(url string) (string, error) {
	body, err := a.getBytes(url)
	return string(body), err
}

func (a *App) getBytes(url string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "codex-unpacker/1.0")
	req.Header.Set("Accept", "application/vnd.github+json, application/json, */*")
	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, url, strings.TrimSpace(string(b)))
	}
	return io.ReadAll(resp.Body)
}

func (a *App) downloadFile(url, path string) error {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "codex-unpacker/1.0")
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("HTTP %d from %s: %s", resp.StatusCode, url, strings.TrimSpace(string(b)))
	}
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, resp.Body)
	return err
}

func (a *App) log(message string) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "log", message)
	}
}

func findAsset(assets []mirrorAsset, name string) *mirrorAsset {
	for i := range assets {
		if assets[i].Name == name {
			return &assets[i]
		}
	}
	return nil
}

func findMSIXAsset(assets []mirrorAsset) *mirrorAsset {
	re := regexp.MustCompile(`^OpenAI\.Codex_.+_x64__2p2nqsd0c76g0\.Msix$`)
	var matches []mirrorAsset
	for _, asset := range assets {
		if re.MatchString(asset.Name) {
			matches = append(matches, asset)
		}
	}
	sort.Slice(matches, func(i, j int) bool {
		left := versionFromMoniker(strings.TrimSuffix(matches[i].Name, filepath.Ext(matches[i].Name)))
		right := versionFromMoniker(strings.TrimSuffix(matches[j].Name, filepath.Ext(matches[j].Name)))
		return versionLess(right, left)
	})
	if len(matches) == 0 {
		return nil
	}
	return &matches[0]
}

func parseChecksum(body, fileName string) string {
	for _, line := range strings.Split(body, "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.EqualFold(fields[1], fileName) && len(fields[0]) == 64 {
			return strings.ToLower(fields[0])
		}
	}
	return ""
}

func versionFromMoniker(moniker string) string {
	parts := strings.Split(moniker, "_")
	if len(parts) > 1 {
		return parts[1]
	}
	return ""
}

func versionLess(a, b string) bool {
	ap := parseVersion(a)
	bp := parseVersion(b)
	for i := 0; i < 4; i++ {
		if ap[i] != bp[i] {
			return ap[i] < bp[i]
		}
	}
	return false
}

func parseVersion(v string) [4]int {
	var out [4]int
	parts := strings.Split(v, ".")
	for i := 0; i < len(parts) && i < 4; i++ {
		fmt.Sscanf(parts[i], "%d", &out[i])
	}
	return out
}

func hashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}

func readState() (latestState, error) {
	var state latestState
	b, err := os.ReadFile(statePath)
	if err != nil {
		return state, err
	}
	err = json.Unmarshal(b, &state)
	return state, err
}

func writeJSON(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	return os.WriteFile(path, b, 0644)
}

func releaseTag(version, hash string, force bool) string {
	tag := fmt.Sprintf("%s-%s-%s", releaseTagPrefix, version, strings.ToLower(hash[:12]))
	if force {
		tag += "-force-" + time.Now().UTC().Format("20060102150405")
	}
	return tag
}

func releaseManifest(info PackageInfo, source resolvedSource, tag, id, url string) map[string]any {
	return map[string]any{
		"schemaVersion": 1,
		"generatedAt":   time.Now().UTC().Format(time.RFC3339),
		"release": map[string]string{
			"tag": tag,
			"id":  id,
			"url": url,
		},
		"source": source,
		"package": map[string]any{
			"name":              info.Name,
			"version":           info.Version,
			"packageMoniker":    info.PackageMoniker,
			"packageFamilyName": info.PackageFamilyName,
			"publisher":         info.Publisher,
			"architecture":      info.Architecture,
			"sha256":            info.SHA256,
			"size":              info.Size,
			"fileName":          info.FileName,
		},
	}
}

func releaseNotes(info PackageInfo, source resolvedSource) string {
	var b bytes.Buffer
	fmt.Fprintf(&b, "Codex MSIX mirror update\n\n")
	fmt.Fprintf(&b, "Version: %s\n", info.Version)
	fmt.Fprintf(&b, "Package: %s\n", info.PackageMoniker)
	fmt.Fprintf(&b, "SHA256: %s\n", info.SHA256)
	fmt.Fprintf(&b, "Source: %s\n", source.DownloadURL)
	fmt.Fprintf(&b, "Resolved via: %s\n", source.SourceKind)
	fmt.Fprintf(&b, "Advertised build: %s\n", source.UpdateManifestVersion)
	fmt.Fprintf(&b, "Timestamp: %s\n", time.Now().UTC().Format(time.RFC3339))
	return b.String()
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

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

func ghAuthed() bool {
	if !commandExists("gh") {
		return false
	}
	cmd := exec.Command("gh", "auth", "status")
	return cmd.Run() == nil
}

func repoInfo() (string, bool) {
	if commandExists("gh") {
		out, err := runCommand("gh", "repo", "view", "--json", "nameWithOwner,isPrivate")
		if err == nil {
			var repo struct {
				NameWithOwner string `json:"nameWithOwner"`
				IsPrivate     bool   `json:"isPrivate"`
			}
			if json.Unmarshal([]byte(out), &repo) == nil && repo.NameWithOwner != "" {
				return repo.NameWithOwner, repo.IsPrivate
			}
		}
	}
	remote, err := gitOutput("remote", "get-url", "origin")
	if err != nil {
		return "", false
	}
	re := regexp.MustCompile(`github\.com[:/](.+?)(?:\.git)?\s*$`)
	match := re.FindStringSubmatch(remote)
	if len(match) == 2 {
		return strings.TrimSuffix(match[1], ".git"), false
	}
	return "", false
}

func gitOutput(args ...string) (string, error) {
	return runCommand("git", args...)
}

func runCommand(name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
