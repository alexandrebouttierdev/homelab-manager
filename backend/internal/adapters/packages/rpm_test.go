package packages

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"monoops/backend/internal/domain"
)

// rpmTag is one tag of a header written by encodeRPMHeader.
type rpmTag struct {
	tag   uint32
	kind  uint32
	value any // string or int32
}

// encodeRPMHeader writes a header as rpm stores it in rpmdb.sqlite, with
// the region tag rpm puts first.
func encodeRPMHeader(tags ...rpmTag) []byte {
	var data bytes.Buffer
	type entry struct{ tag, kind, offset, count uint32 }
	entries := []entry{{tag: 63, kind: 7, offset: 0, count: 16}}
	data.Write(make([]byte, 16)) // region trailer, unread
	for _, t := range tags {
		switch value := t.value.(type) {
		case string:
			entries = append(entries, entry{t.tag, t.kind, uint32(data.Len()), 1})
			data.WriteString(value)
			data.WriteByte(0)
		case int32:
			for data.Len()%4 != 0 {
				data.WriteByte(0)
			}
			entries = append(entries, entry{t.tag, t.kind, uint32(data.Len()), 1})
			_ = binary.Write(&data, binary.BigEndian, value)
		}
	}
	var blob bytes.Buffer
	_ = binary.Write(&blob, binary.BigEndian, uint32(len(entries)))
	_ = binary.Write(&blob, binary.BigEndian, uint32(data.Len()))
	for _, e := range entries {
		_ = binary.Write(&blob, binary.BigEndian, e)
	}
	blob.Write(data.Bytes())
	return blob.Bytes()
}

func rpmPackage(name, version, release, arch, source string, epoch *int32) []byte {
	tags := []rpmTag{
		{rpmTagName, rpmTypeString, name}, {rpmTagVersion, rpmTypeString, version},
		{rpmTagRelease, rpmTypeString, release}, {rpmTagArch, rpmTypeString, arch},
	}
	if source != "" {
		tags = append(tags, rpmTag{rpmTagSourceRPM, rpmTypeString, source})
	}
	if epoch != nil {
		tags = append(tags, rpmTag{rpmTagEpoch, rpmTypeInt32, *epoch})
	}
	return encodeRPMHeader(tags...)
}

// writeRPMDatabase creates var/lib/rpm/rpmdb.sqlite below root with the
// Packages table of rpm 4.16 and later.
func writeRPMDatabase(t *testing.T, root string, blobs ...[]byte) {
	t.Helper()
	path := filepath.Join(root, rpmSQLitePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.Remove(path)
	database, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if _, err := database.Exec("CREATE TABLE Packages (hnum INTEGER PRIMARY KEY AUTOINCREMENT, blob BLOB NOT NULL)"); err != nil {
		t.Fatal(err)
	}
	for _, blob := range blobs {
		if _, err := database.Exec("INSERT INTO Packages (blob) VALUES (?)", blob); err != nil {
			t.Fatal(err)
		}
	}
}

const rockyRelease = "NAME=\"Rocky Linux\"\nID=\"rocky\"\nID_LIKE=\"rhel centos fedora\"\nVERSION_ID=\"9.4\"\nPRETTY_NAME=\"Rocky Linux 9.4 (Blue Onyx)\"\n"

func TestRPMInventory(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "etc/os-release", rockyRelease)
	epoch := int32(1)
	writeRPMDatabase(t, root,
		rpmPackage("openssl-libs", "3.0.7", "27.el9", "x86_64", "openssl-3.0.7-27.el9.src.rpm", &epoch),
		rpmPackage("gpg-pubkey", "350d275d", "6279464b", "(none)", "", nil),
		encodeRPMHeader(
			rpmTag{rpmTagName, rpmTypeString, "nodejs"}, rpmTag{rpmTagVersion, rpmTypeString, "18.20.2"},
			rpmTag{rpmTagRelease, rpmTypeString, "1.module+el9.4.0"}, rpmTag{rpmTagArch, rpmTypeString, "x86_64"},
			rpmTag{rpmTagSourceRPM, rpmTypeString, "nodejs-18.20.2-1.module+el9.4.0.src.rpm"},
			rpmTag{rpmTagModularityLabel, rpmTypeString, "nodejs:18:9040020240509:rhel9"},
		),
		rpmPackage("bash", "5.1.8", "9.el9", "x86_64", "bash-5.1.8-9.el9.src.rpm", nil),
	)
	inventory, err := New(root).Inventory(context.Background())
	if err != nil {
		t.Fatalf("Inventory() = %v", err)
	}
	if inventory.Manager != domain.PackageManagerRPM || inventory.Distro.ID != "rocky" || len(inventory.Packages) != 3 {
		t.Fatalf("inventory = %+v", inventory)
	}
	ssl, node, bash := inventory.Packages[0], inventory.Packages[1], inventory.Packages[2]
	if ssl.Name != "openssl-libs" || ssl.Version != "3.0.7" || ssl.Release != "27.el9" || ssl.Epoch == nil || *ssl.Epoch != 1 ||
		ssl.Architecture != "x86_64" || ssl.Source != "openssl-3.0.7-27.el9.src.rpm" {
		t.Fatalf("openssl-libs = %+v", ssl)
	}
	if node.ModularityLabel != "nodejs:18:9040020240509:rhel9" || bash.Epoch != nil || bash.ModularityLabel != "" {
		t.Fatalf("nodejs = %+v, bash = %+v", node, bash)
	}
}

func TestRPMDatabaseMissingOrBerkeley(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "etc/os-release", rockyRelease)
	if _, err := New(root).Inventory(context.Background()); err == nil || errors.Is(err, domain.ErrUnsupportedDistro) {
		t.Fatalf("missing database = %v, want a read error", err)
	}
	writeFile(t, root, rpmBerkeleyPath, "berkeley")
	inventory, err := New(root).Inventory(context.Background())
	if !errors.Is(err, domain.ErrUnsupportedDistro) || !strings.Contains(err.Error(), "Berkeley") || inventory.Distro.ID != "rocky" {
		t.Fatalf("Berkeley DB = (%+v, %v)", inventory, err)
	}
	writeFile(t, root, rpmSQLitePath, "not a database")
	if _, err := New(root).Inventory(context.Background()); err == nil {
		t.Fatal("a corrupt database must fail")
	}
}

func TestRPMHeaderRefusesMalformedData(t *testing.T) {
	valid := rpmPackage("bash", "5.1.8", "9.el9", "x86_64", "", nil)
	if _, err := parseRPMHeader(valid); err != nil {
		t.Fatal(err)
	}
	corrupt := func(change func([]byte) []byte) []byte {
		return change(append([]byte{}, valid...))
	}
	nameEntry := 8 + 16 // the entry after the region tag
	cases := map[string][]byte{
		"empty":     nil,
		"too short": valid[:6],
		"truncated": valid[:len(valid)-3],
		"no entry":  corrupt(func(b []byte) []byte { binary.BigEndian.PutUint32(b[0:4], 0); return b }),
		"too many":  corrupt(func(b []byte) []byte { binary.BigEndian.PutUint32(b[0:4], 0x10000); return b }),
		"offset out": corrupt(func(b []byte) []byte {
			binary.BigEndian.PutUint32(b[nameEntry+8:nameEntry+12], 1<<20)
			return b
		}),
		"not a string": corrupt(func(b []byte) []byte {
			binary.BigEndian.PutUint32(b[nameEntry+4:nameEntry+8], rpmTypeInt32)
			return b
		}),
		"unterminated": encodeRPMHeader(rpmTag{rpmTagName, rpmTypeString, "bash"})[:0],
		"no version":   encodeRPMHeader(rpmTag{rpmTagName, rpmTypeString, "bash"}),
		"negative epoch": func() []byte {
			epoch := int32(-1)
			return rpmPackage("bash", "1", "1", "x86_64", "", &epoch)
		}(),
	}
	unterminated := encodeRPMHeader(rpmTag{rpmTagName, rpmTypeString, "bash"}, rpmTag{rpmTagVersion, rpmTypeString, "1"})
	cases["unterminated"] = unterminated[:len(unterminated)-1]
	binary.BigEndian.PutUint32(cases["unterminated"][4:8], binary.BigEndian.Uint32(unterminated[4:8])-1)
	for name, blob := range cases {
		if _, err := parseRPMHeader(blob); !errors.Is(err, errMalformed) {
			t.Errorf("%s: %v, want errMalformed", name, err)
		}
	}
}

func TestUnknownDistributionIsUnsupported(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "etc/os-release", "ID=arch\nNAME=\"Arch Linux\"\n")
	inventory, err := New(root).Inventory(context.Background())
	if !errors.Is(err, domain.ErrUnsupportedDistro) || inventory.Distro.ID != "arch" || inventory.Manager != "" {
		t.Fatalf("Inventory() = (%+v, %v)", inventory, err)
	}
}
