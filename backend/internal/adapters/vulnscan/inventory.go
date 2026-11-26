// Package vulnscan hands the package inventory to Grype: it writes the
// inventory as a Syft JSON document, asks systemd to start the dedicated
// monoops-vulnscan.service unit over D-Bus, waits for it and decodes the
// Grype result. The Manager never runs grype itself and never reaches the
// network for this module (SPEC_ADMINISTRATION.md § 8 ter.2).
package vulnscan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"monoops/backend/internal/domain"
)

// syftSchemaVersion is the Syft JSON schema the document follows; Grype
// v0.121 decodes schema 16.
const syftSchemaVersion = "16.1.13"

type syftDocument struct {
	Artifacts             []syftArtifact `json:"artifacts"`
	ArtifactRelationships []struct{}     `json:"artifactRelationships"`
	Source                syftSource     `json:"source"`
	Distro                syftDistro     `json:"distro"`
	Descriptor            syftDescriptor `json:"descriptor"`
	Schema                syftSchema     `json:"schema"`
}

type syftLocation struct {
	Path       string `json:"path"`
	AccessPath string `json:"accessPath"`
}

type syftArtifact struct {
	ID           string         `json:"id"`
	Name         string         `json:"name"`
	Version      string         `json:"version"`
	Type         string         `json:"type"`
	FoundBy      string         `json:"foundBy"`
	Locations    []syftLocation `json:"locations"`
	Licenses     []struct{}     `json:"licenses"`
	Language     string         `json:"language"`
	CPEs         []struct{}     `json:"cpes"`
	PURL         string         `json:"purl"`
	MetadataType string         `json:"metadataType"`
	Metadata     any            `json:"metadata"`
}

// dpkgEntry and apkEntry are the Syft metadata Grype reads: the source
// package (dpkg) or origin (apk) links a package to its distribution
// advisories.
type dpkgEntry struct {
	Package       string     `json:"package"`
	Source        string     `json:"source"`
	Version       string     `json:"version"`
	SourceVersion string     `json:"sourceVersion"`
	Architecture  string     `json:"architecture"`
	Maintainer    string     `json:"maintainer"`
	InstalledSize int        `json:"installedSize"`
	Files         []struct{} `json:"files"`
}

type apkEntry struct {
	Package       string     `json:"package"`
	OriginPackage string     `json:"originPackage"`
	Maintainer    string     `json:"maintainer"`
	Version       string     `json:"version"`
	Architecture  string     `json:"architecture"`
	URL           string     `json:"url"`
	Description   string     `json:"description"`
	Size          int        `json:"size"`
	InstalledSize int        `json:"installedSize"`
	Dependencies  []string   `json:"pullDependencies"`
	Provides      []string   `json:"provides"`
	Checksum      string     `json:"pullChecksum"`
	GitCommit     string     `json:"gitCommitOfApkPort"`
	Files         []struct{} `json:"files"`
}

// rpmEntry is Syft's rpm-db-entry: Grype matches an RPM package by its
// epoch, version and release, and links it to its advisories by the source
// RPM and, on RHEL-like systems, the modularity label.
type rpmEntry struct {
	Name            string     `json:"name"`
	Version         string     `json:"version"`
	Epoch           *int       `json:"epoch"`
	Architecture    string     `json:"architecture"`
	Release         string     `json:"release"`
	SourceRpm       string     `json:"sourceRpm"`
	Size            int        `json:"size"`
	Vendor          string     `json:"vendor"`
	ModularityLabel *string    `json:"modularityLabel,omitempty"`
	Files           []struct{} `json:"files"`
}

type syftSource struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Version  string            `json:"version"`
	Type     string            `json:"type"`
	Metadata map[string]string `json:"metadata"`
}

type syftDistro struct {
	PrettyName      string   `json:"prettyName,omitempty"`
	Name            string   `json:"name,omitempty"`
	ID              string   `json:"id"`
	IDLike          []string `json:"idLike,omitempty"`
	VersionID       string   `json:"versionID,omitempty"`
	VersionCodename string   `json:"versionCodename,omitempty"`
}

type syftDescriptor struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type syftSchema struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// purl builds a package URL as Syft does: name, version and qualifier
// values query-escaped, qualifiers sorted by key.
func purl(kind, namespace, name, version string, qualifiers map[string]string) string {
	keys := make([]string, 0, len(qualifiers))
	for key, value := range qualifiers {
		if value != "" {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, key := range keys {
		parts[i] = key + "=" + url.QueryEscape(qualifiers[key])
	}
	text := "pkg:" + kind + "/" + url.QueryEscape(namespace) + "/" + url.QueryEscape(name) + "@" + url.QueryEscape(version)
	if len(parts) > 0 {
		text += "?" + strings.Join(parts, "&")
	}
	return text
}

func artifactID(kind string, pkg domain.InstalledPackage) string {
	sum := sha256.Sum256([]byte(kind + "\x00" + pkg.Name + "\x00" + pkg.Version + "\x00" + pkg.Release + "\x00" + pkg.Architecture))
	return hex.EncodeToString(sum[:8])
}

// syftInventory converts an inventory into the Syft JSON document read by
// "grype sbom:".
func syftInventory(inventory domain.PackageInventory, version string) (syftDocument, error) {
	distro := inventory.Distro
	qualifierDistro := distro.ID
	if distro.VersionID != "" {
		qualifierDistro += "-" + distro.VersionID
	}
	document := syftDocument{
		Artifacts:             make([]syftArtifact, 0, len(inventory.Packages)),
		ArtifactRelationships: []struct{}{},
		Source:                syftSource{ID: "monoops-host", Name: "/", Type: "directory", Metadata: map[string]string{"path": "/"}},
		Distro: syftDistro{
			PrettyName: distro.PrettyName, Name: distro.Name, ID: distro.ID, IDLike: distro.IDLike,
			VersionID: distro.VersionID, VersionCodename: distro.VersionCodename,
		},
		Descriptor: syftDescriptor{Name: "monoops", Version: version},
		Schema: syftSchema{
			Version: syftSchemaVersion,
			URL:     "https://raw.githubusercontent.com/anchore/syft/main/schema/json/schema-" + syftSchemaVersion + ".json",
		},
	}
	for _, pkg := range inventory.Packages {
		upstream := pkg.Source
		if upstream != "" && pkg.SourceVersion != "" {
			upstream += "@" + pkg.SourceVersion
		}
		qualifiers := map[string]string{"arch": pkg.Architecture, "distro": qualifierDistro, "upstream": upstream}
		artifact := syftArtifact{
			Name: pkg.Name, Version: pkg.Version, FoundBy: "monoops", Licenses: []struct{}{}, CPEs: []struct{}{},
		}
		switch inventory.Manager {
		case domain.PackageManagerDpkg:
			artifact.Type, artifact.MetadataType = "deb", "dpkg-db-entry"
			artifact.Locations = []syftLocation{{Path: "/var/lib/dpkg/status", AccessPath: "/var/lib/dpkg/status"}}
			artifact.Metadata = dpkgEntry{
				Package: pkg.Name, Source: pkg.Source, Version: pkg.Version, SourceVersion: pkg.SourceVersion,
				Architecture: pkg.Architecture, Files: []struct{}{},
			}
			artifact.PURL = purl("deb", distro.ID, pkg.Name, pkg.Version, qualifiers)
		case domain.PackageManagerApk:
			artifact.Type, artifact.MetadataType = "apk", "apk-db-entry"
			artifact.Locations = []syftLocation{{Path: "/lib/apk/db/installed", AccessPath: "/lib/apk/db/installed"}}
			artifact.Metadata = apkEntry{
				Package: pkg.Name, OriginPackage: pkg.Source, Version: pkg.Version, Architecture: pkg.Architecture,
				Dependencies: []string{}, Provides: []string{}, Files: []struct{}{},
			}
			origin := pkg.Source
			if origin == pkg.Name {
				origin = ""
			}
			artifact.PURL = purl("apk", distro.ID, pkg.Name, pkg.Version, map[string]string{
				"arch": pkg.Architecture, "distro": qualifierDistro, "upstream": origin,
			})
		case domain.PackageManagerRPM:
			artifact.Type, artifact.MetadataType = "rpm", "rpm-db-entry"
			artifact.Locations = []syftLocation{{Path: "/var/lib/rpm/rpmdb.sqlite", AccessPath: "/var/lib/rpm/rpmdb.sqlite"}}
			entry := rpmEntry{
				Name: pkg.Name, Version: pkg.Version, Epoch: pkg.Epoch, Architecture: pkg.Architecture,
				Release: pkg.Release, SourceRpm: pkg.Source, Files: []struct{}{},
			}
			if pkg.ModularityLabel != "" {
				label := pkg.ModularityLabel
				entry.ModularityLabel = &label
			}
			artifact.Metadata = entry
			// Syft names the version [epoch:]version-release; the PURL
			// carries the epoch as a qualifier.
			versionRelease := pkg.Version
			if pkg.Release != "" {
				versionRelease += "-" + pkg.Release
			}
			artifact.Version = versionRelease
			epoch := ""
			if pkg.Epoch != nil {
				epoch = strconv.Itoa(*pkg.Epoch)
				artifact.Version = epoch + ":" + versionRelease
			}
			namespace := distro.ID
			if namespace == "rhel" {
				namespace = "redhat"
			}
			artifact.PURL = purl("rpm", namespace, pkg.Name, versionRelease, map[string]string{
				"arch": pkg.Architecture, "distro": qualifierDistro, "epoch": epoch, "upstream": pkg.Source,
			})
		default:
			return syftDocument{}, fmt.Errorf("%w: %s", domain.ErrUnsupportedDistro, distro.ID)
		}
		artifact.ID = artifactID(artifact.Type, pkg)
		document.Artifacts = append(document.Artifacts, artifact)
	}
	return document, nil
}

// writeInventory writes the document atomically (temporary file then
// rename) and readable by the group of the directory, the unit account.
func writeInventory(path string, inventory domain.PackageInventory, version string) error {
	document, err := syftInventory(inventory, version)
	if err != nil {
		return err
	}
	content, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("encode inventory: %w", err)
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".inventory-*.json")
	if err != nil {
		return fmt.Errorf("create inventory: %w", err)
	}
	defer os.Remove(temporary.Name())
	if _, err := temporary.Write(content); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write inventory: %w", err)
	}
	// The Manager's umask (0077) would leave the file unreadable by the
	// unit, which reads it through the group of the directory.
	if err := temporary.Chmod(0o640); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("chmod inventory: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close inventory: %w", err)
	}
	if err := os.Rename(temporary.Name(), path); err != nil {
		return fmt.Errorf("rename inventory: %w", err)
	}
	return nil
}
