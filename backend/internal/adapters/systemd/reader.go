// Package systemd talks to the systemd manager over the system D-Bus. The
// Reader reads without privilege: ListUnits, GetUnit and property reads
// only. The Controller (controller.go) adds the four verbs of
// SPEC_ADMINISTRATION.md § 5.2, granted by polkit for the allow list only;
// nothing enables, masks or reloads the systemd configuration (§ 2.1).
package systemd

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"homelab-manager/backend/internal/domain"
	"homelab-manager/backend/internal/ports"

	"github.com/godbus/dbus/v5"
)

const (
	busName          = "org.freedesktop.systemd1"
	managerPath      = dbus.ObjectPath("/org/freedesktop/systemd1")
	managerInterface = "org.freedesktop.systemd1.Manager"
	unitInterface    = "org.freedesktop.systemd1.Unit"
	propertiesGetAll = "org.freedesktop.DBus.Properties.GetAll"
)

var typeInterfaces = map[domain.SystemUnitType]string{
	domain.UnitTypeService: "org.freedesktop.systemd1.Service",
	domain.UnitTypeTimer:   "org.freedesktop.systemd1.Timer",
	domain.UnitTypeSocket:  "org.freedesktop.systemd1.Socket",
}

// unavailable are the D-Bus errors of a bus without systemd.
var unavailable = map[string]bool{
	"org.freedesktop.DBus.Error.ServiceUnknown": true,
	"org.freedesktop.DBus.Error.NameHasNoOwner": true,
	"org.freedesktop.DBus.Error.NoServer":       true,
	"org.freedesktop.DBus.Error.Disconnected":   true,
}

// spawnErrors prefixes the D-Bus errors of a failed service activation.
// systemd installs an activation file for its own name that runs /bin/false,
// so a bus where systemd is not running answers Spawn.ChildExited.
const spawnErrors = "org.freedesktop.DBus.Error.Spawn."

// vanished are the D-Bus errors of a unit unloaded between two calls, or
// of a property interface the unit does not have.
var vanished = map[string]bool{
	"org.freedesktop.DBus.Error.UnknownObject":    true,
	"org.freedesktop.DBus.Error.UnknownInterface": true,
	"org.freedesktop.DBus.Error.UnknownMethod":    true,
	"org.freedesktop.systemd1.NoSuchUnit":         true,
}

var _ ports.SystemUnitReader = (*Reader)(nil)

// Connector opens a connection to the bus where systemd runs.
type Connector func(ctx context.Context) (*dbus.Conn, error)

// SystemBus connects to the system bus. On Linux DBUS_SYSTEM_BUS_ADDRESS
// overrides its address (godbus reads DBUS_LAUNCHD_SESSION_BUS_SOCKET, a
// socket path, on macOS).
func SystemBus(ctx context.Context) (*dbus.Conn, error) {
	return dbus.ConnectSystemBus(dbus.WithContext(ctx))
}

// Reader opens one connection per call (one per collection cycle).
type Reader struct {
	connect Connector
}

func New(connect Connector) *Reader {
	return &Reader{connect: connect}
}

// listedUnit is one entry of ListUnits, a(ssssssouso).
type listedUnit struct {
	Name, Description, LoadState, ActiveState, SubState, Following string
	Path                                                           dbus.ObjectPath
	JobID                                                          uint32
	JobType                                                        string
	JobPath                                                        dbus.ObjectPath
}

func (r *Reader) open(ctx context.Context) (*dbus.Conn, error) {
	conn, err := r.connect(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", domain.ErrSystemdUnavailable, err)
	}
	return conn, nil
}

func (r *Reader) Units(ctx context.Context) ([]domain.SystemUnit, error) {
	conn, err := r.open(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = conn.Close() }()

	var listed []listedUnit
	if err := conn.Object(busName, managerPath).CallWithContext(ctx, managerInterface+".ListUnits", 0).Store(&listed); err != nil {
		return nil, translate("list units", err)
	}
	listed = selectUnits(listed)
	units := make([]domain.SystemUnit, 0, len(listed))
	for _, entry := range listed {
		unit, _, err := readUnit(ctx, conn, entry.Path, entry.Name)
		if errors.Is(err, errVanished) {
			unit = fromListing(entry)
		} else if err != nil {
			return nil, err
		}
		units = append(units, unit)
	}
	return units, nil
}

// selectUnits keeps the types the Manager reads, failed first then by
// name, within domain.MaxSystemUnits: a large machine never hides a
// failure.
func selectUnits(listed []listedUnit) []listedUnit {
	kept := listed[:0]
	for _, entry := range listed {
		if domain.UnitTypeOf(entry.Name) != "" {
			kept = append(kept, entry)
		}
	}
	sort.SliceStable(kept, func(i, j int) bool {
		failedI, failedJ := kept[i].ActiveState == "failed", kept[j].ActiveState == "failed"
		if failedI != failedJ {
			return failedI
		}
		return kept[i].Name < kept[j].Name
	})
	if len(kept) > domain.MaxSystemUnits {
		kept = kept[:domain.MaxSystemUnits]
	}
	return kept
}

func fromListing(entry listedUnit) domain.SystemUnit {
	return domain.SystemUnit{
		Name: entry.Name, Description: entry.Description, Type: domain.UnitTypeOf(entry.Name),
		LoadState: entry.LoadState, ActiveState: entry.ActiveState, SubState: entry.SubState,
	}
}

func (r *Reader) Unit(ctx context.Context, name string) (domain.SystemUnitDetail, error) {
	if !domain.ValidUnitName(name) {
		return domain.SystemUnitDetail{}, domain.ErrInvalidUnitName
	}
	conn, err := r.open(ctx)
	if err != nil {
		return domain.SystemUnitDetail{}, err
	}
	defer func() { _ = conn.Close() }()

	// GetUnit only looks up a loaded unit; LoadUnit would load one.
	var path dbus.ObjectPath
	if err := conn.Object(busName, managerPath).CallWithContext(ctx, managerInterface+".GetUnit", 0, name).Store(&path); err != nil {
		if isNamed(err, vanished) {
			return domain.SystemUnitDetail{}, domain.ErrUnitNotFound
		}
		return domain.SystemUnitDetail{}, translate("get unit", err)
	}
	unit, props, err := readUnit(ctx, conn, path, name)
	if errors.Is(err, errVanished) {
		return domain.SystemUnitDetail{}, domain.ErrUnitNotFound
	}
	if err != nil {
		return domain.SystemUnitDetail{}, err
	}
	detail := domain.SystemUnitDetail{
		SystemUnit:   unit,
		FragmentPath: props.unit.str("FragmentPath"),
		Requires:     bounded(props.unit.strs("Requires")),
		After:        bounded(props.unit.strs("After")),
		ExecStart:    props.kind.commandLines("ExecStart"),
		Restart:      props.kind.str("Restart"),
		Slice:        props.kind.str("Slice"),
		TasksMax:     props.kind.limit("TasksMax"),
		LimitNOFILE:  props.kind.limit("LimitNOFILE"),
		MemoryMax:    props.kind.limit("MemoryMax"),
	}
	return detail, nil
}

func bounded(names []string) []string {
	if len(names) > domain.MaxUnitDependencies {
		return names[:domain.MaxUnitDependencies]
	}
	return names
}

var errVanished = errors.New("unit vanished")

// unitProperties holds the Unit interface and the interface of the unit
// type (Service, Timer or Socket); either may be empty.
type unitProperties struct {
	unit, kind properties
}

func readUnit(ctx context.Context, conn *dbus.Conn, path dbus.ObjectPath, name string) (domain.SystemUnit, unitProperties, error) {
	unitType := domain.UnitTypeOf(name)
	object := conn.Object(busName, path)
	var props unitProperties
	if err := object.CallWithContext(ctx, propertiesGetAll, 0, unitInterface).Store(&props.unit); err != nil {
		if isNamed(err, vanished) {
			return domain.SystemUnit{}, props, errVanished
		}
		return domain.SystemUnit{}, props, translate("read unit properties", err)
	}
	if err := object.CallWithContext(ctx, propertiesGetAll, 0, typeInterfaces[unitType]).Store(&props.kind); err != nil {
		if !isNamed(err, vanished) {
			return domain.SystemUnit{}, props, translate("read unit type properties", err)
		}
		props.kind = nil
	}
	unit := domain.SystemUnit{
		Name: name, Type: unitType,
		Description:    props.unit.str("Description"),
		LoadState:      props.unit.str("LoadState"),
		ActiveState:    props.unit.str("ActiveState"),
		SubState:       props.unit.str("SubState"),
		UnitFileState:  props.unit.str("UnitFileState"),
		StateChangedAt: props.unit.timestamp("StateChangeTimestamp"),
		Result:         props.kind.str("Result"),
	}
	switch unitType {
	case domain.UnitTypeService:
		if pid, ok := props.kind.u32("MainPID"); ok {
			unit.MainPID = domain.OptionalPID(pid)
		}
		unit.MemoryBytes = props.kind.counter("MemoryCurrent")
		unit.CPUNanoseconds = props.kind.counter("CPUUsageNSec")
	case domain.UnitTypeTimer:
		unit.NextElapse = props.kind.timestamp("NextElapseUSecRealtime")
		unit.LastTrigger = props.kind.timestamp("LastTriggerUSec")
		unit.Triggers = props.unit.strs("Triggers")
	case domain.UnitTypeSocket:
		unit.Triggers = props.unit.strs("Triggers")
	}
	return unit, props, nil
}

// errorName is the name of a D-Bus error, "" for any other error.
func errorName(err error) string {
	var dbusErr dbus.Error
	if errors.As(err, &dbusErr) {
		return dbusErr.Name
	}
	var dbusErrPtr *dbus.Error
	if errors.As(err, &dbusErrPtr) {
		return dbusErrPtr.Name
	}
	return ""
}

func isNamed(err error, names map[string]bool) bool {
	return names[errorName(err)]
}

func translate(action string, err error) error {
	if isNamed(err, unavailable) || strings.HasPrefix(errorName(err), spawnErrors) {
		return fmt.Errorf("%w: %s: %v", domain.ErrSystemdUnavailable, action, err)
	}
	return fmt.Errorf("%s: %w", action, err)
}

// properties is the result of Properties.GetAll. A property of an
// unexpected type is treated as absent.
type properties map[string]dbus.Variant

func (p properties) str(name string) string {
	value, _ := p[name].Value().(string)
	return value
}

func (p properties) strs(name string) []string {
	value, _ := p[name].Value().([]string)
	return value
}

func (p properties) u32(name string) (uint32, bool) {
	value, ok := p[name].Value().(uint32)
	return value, ok
}

func (p properties) u64(name string) (uint64, bool) {
	value, ok := p[name].Value().(uint64)
	return value, ok
}

func (p properties) counter(name string) *uint64 {
	if value, ok := p.u64(name); ok {
		return domain.OptionalCounter(value)
	}
	return nil
}

func (p properties) timestamp(name string) *time.Time {
	if value, ok := p.u64(name); ok {
		return domain.OptionalTimestamp(value)
	}
	return nil
}

func (p properties) limit(name string) domain.UnitLimit {
	value, ok := p.u64(name)
	return domain.UnitLimitFrom(value, ok)
}

// commandLines reads an a(sasbttttuii) property such as ExecStart: the
// argument vector of each command, joined by spaces with the arguments
// that are empty or hold a space or a quote in double quotes, or its path
// when the vector is empty.
func (p properties) commandLines(name string) []string {
	commands, ok := p[name].Value().([][]any)
	if !ok {
		return nil
	}
	lines := make([]string, 0, len(commands))
	for _, command := range commands {
		if len(command) < 2 {
			continue
		}
		path, _ := command[0].(string)
		argv, _ := command[1].([]string)
		if len(argv) == 0 {
			lines = append(lines, path)
			continue
		}
		quoted := make([]string, len(argv))
		for i, argument := range argv {
			quoted[i] = quoteArgument(argument)
		}
		lines = append(lines, strings.Join(quoted, " "))
	}
	return lines
}

// quoteArgument keeps an argument readable once joined: without quotes,
// "sh -c 'a; b'" would show as three arguments.
func quoteArgument(argument string) string {
	if argument != "" && !strings.ContainsAny(argument, " \t\n\"'") {
		return argument
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(argument) + `"`
}
