package packages

import (
	"context"
	"database/sql"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"os"

	"monoops/backend/internal/domain"

	_ "modernc.org/sqlite" // Pure-Go SQLite driver registered as "sqlite".
)

// The RPM database: SQLite since rpm 4.16 (Fedora 33, RHEL, Rocky Linux
// and AlmaLinux 9). The Berkeley DB of older releases is not read.
const (
	rpmSQLitePath   = "var/lib/rpm/rpmdb.sqlite"
	rpmBerkeleyPath = "var/lib/rpm/Packages"
	// maxRPMDatabaseBytes bounds the database file; a full Fedora
	// workstation stays near 150 MiB.
	maxRPMDatabaseBytes = 1 << 30
	// maxRPMHeaderBytes bounds one header; file lists make the largest.
	maxRPMHeaderBytes = 64 << 20
	// maxRPMHeaderEntries is rpm's own bound on the tags of a header.
	maxRPMHeaderEntries = 0xffff
)

// Tags and types of an RPM header (rpmtag.h).
const (
	rpmTagName            = 1000
	rpmTagVersion         = 1001
	rpmTagRelease         = 1002
	rpmTagEpoch           = 1003
	rpmTagArch            = 1022
	rpmTagSourceRPM       = 1044
	rpmTagModularityLabel = 5096

	rpmTypeInt32       = 4
	rpmTypeString      = 6
	rpmTypeStringArray = 8
	rpmTypeI18NString  = 9
)

var errBerkeleyRPM = errors.New("the RPM database uses Berkeley DB (rpm before 4.16), which is not read")

// readRPMDatabase lists the packages of /var/lib/rpm/rpmdb.sqlite, opened
// read only: every account may read it, and the Manager never writes it.
func (r *Reader) readRPMDatabase(ctx context.Context) ([]domain.InstalledPackage, error) {
	path := r.path(rpmSQLitePath)
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		if _, berkeley := os.Stat(r.path(rpmBerkeleyPath)); berkeley == nil {
			return nil, fmt.Errorf("%w: %w", domain.ErrUnsupportedDistro, errBerkeleyRPM)
		}
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > maxRPMDatabaseBytes {
		return nil, errTooLarge
	}
	packages, err := queryRPMDatabase(ctx, "file:"+(&url.URL{Path: path}).EscapedPath()+"?mode=ro&_pragma=query_only(1)")
	if err != nil {
		// rpm may leave the database in WAL mode, whose shared memory file
		// a reader without write access cannot create: read the main file
		// as is, as rpm checkpoints it when it closes.
		packages, err = queryRPMDatabase(ctx, "file:"+(&url.URL{Path: path}).EscapedPath()+"?mode=ro&immutable=1")
	}
	return packages, err
}

func queryRPMDatabase(ctx context.Context, dsn string) ([]domain.InstalledPackage, error) {
	database, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	defer database.Close()
	database.SetMaxOpenConns(1)
	rows, err := database.QueryContext(ctx, "SELECT blob FROM Packages")
	if err != nil {
		return nil, fmt.Errorf("query the RPM database: %w", err)
	}
	defer rows.Close()
	var packages []domain.InstalledPackage
	for rows.Next() {
		var blob []byte
		if err := rows.Scan(&blob); err != nil {
			return nil, err
		}
		if len(blob) > maxRPMHeaderBytes {
			return nil, errTooLarge
		}
		pkg, err := parseRPMHeader(blob)
		if err != nil {
			return nil, err
		}
		// gpg-pubkey entries are the signing keys rpm imported, not software.
		if pkg.Name == "gpg-pubkey" {
			continue
		}
		if packages, err = appendPackage(packages, pkg); err != nil {
			return nil, err
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the RPM database: %w", err)
	}
	return packages, nil
}

// rpmEntry is one entry of the index of a header.
type rpmEntry struct {
	kind   uint32
	offset uint32
	count  uint32
}

// parseRPMHeader decodes a header as rpm stores it in its database: the
// number of index entries and the size of the data, both big-endian
// 32-bit, the entries (tag, type, offset, count) then the data. Every
// offset and length is checked against the data before use.
func parseRPMHeader(blob []byte) (domain.InstalledPackage, error) {
	if len(blob) < 8 {
		return domain.InstalledPackage{}, fmt.Errorf("%w: RPM header too short", errMalformed)
	}
	entries, dataLength := binary.BigEndian.Uint32(blob[0:4]), binary.BigEndian.Uint32(blob[4:8])
	if entries == 0 || entries > maxRPMHeaderEntries || dataLength > maxRPMHeaderBytes {
		return domain.InstalledPackage{}, fmt.Errorf("%w: RPM header bounds", errMalformed)
	}
	indexEnd := 8 + 16*uint64(entries)
	if uint64(len(blob)) < indexEnd+uint64(dataLength) {
		return domain.InstalledPackage{}, fmt.Errorf("%w: RPM header truncated", errMalformed)
	}
	data := blob[indexEnd : indexEnd+uint64(dataLength)]
	index := map[uint32]rpmEntry{}
	for i := uint64(0); i < uint64(entries); i++ {
		raw := blob[8+16*i : 8+16*(i+1)]
		tag := binary.BigEndian.Uint32(raw[0:4])
		if _, seen := index[tag]; !seen {
			index[tag] = rpmEntry{kind: binary.BigEndian.Uint32(raw[4:8]), offset: binary.BigEndian.Uint32(raw[8:12]), count: binary.BigEndian.Uint32(raw[12:16])}
		}
	}
	text := func(tag uint32) (string, error) {
		entry, ok := index[tag]
		if !ok {
			return "", nil
		}
		switch entry.kind {
		case rpmTypeString, rpmTypeStringArray, rpmTypeI18NString:
		default:
			return "", fmt.Errorf("%w: RPM tag %d is not a string", errMalformed, tag)
		}
		if entry.count == 0 || uint64(entry.offset) >= uint64(len(data)) {
			return "", fmt.Errorf("%w: RPM tag %d out of the data", errMalformed, tag)
		}
		value := data[entry.offset:]
		end := 0
		for end < len(value) && value[end] != 0 {
			end++
		}
		if end == len(value) {
			return "", fmt.Errorf("%w: RPM tag %d unterminated", errMalformed, tag)
		}
		return clean(string(value[:end])), nil
	}
	var pkg domain.InstalledPackage
	var err error
	for _, field := range []struct {
		tag    uint32
		target *string
	}{
		{rpmTagName, &pkg.Name}, {rpmTagVersion, &pkg.Version}, {rpmTagRelease, &pkg.Release},
		{rpmTagArch, &pkg.Architecture}, {rpmTagSourceRPM, &pkg.Source}, {rpmTagModularityLabel, &pkg.ModularityLabel},
	} {
		if *field.target, err = text(field.tag); err != nil {
			return domain.InstalledPackage{}, err
		}
	}
	if entry, ok := index[rpmTagEpoch]; ok {
		if entry.kind != rpmTypeInt32 || entry.count == 0 || uint64(entry.offset)+4 > uint64(len(data)) {
			return domain.InstalledPackage{}, fmt.Errorf("%w: RPM epoch", errMalformed)
		}
		epoch := int(int32(binary.BigEndian.Uint32(data[entry.offset : entry.offset+4])))
		if epoch < 0 {
			return domain.InstalledPackage{}, fmt.Errorf("%w: RPM epoch", errMalformed)
		}
		pkg.Epoch = &epoch
	}
	if pkg.Name == "" || pkg.Version == "" {
		return domain.InstalledPackage{}, fmt.Errorf("%w: RPM header without name or version", errMalformed)
	}
	return pkg, nil
}
