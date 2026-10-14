package remotecontrol

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
)

// Screenshot captures the screen. It's served while the screenshot app is enabled, which
// it isn't until an owner or admin enables it (see RemoteControl.apps).
func (s *Screen) Screenshot() ([]byte, error) {
	return CaptureScreenshot(s.sessionBus)
}

// CaptureScreenshot performs the actual portal call. It is intended to be
// invoked from the foreground helper process.
func CaptureScreenshot(conn *dbus.Conn) ([]byte, error) {
	matchOptions := []dbus.MatchOption{
		dbus.WithMatchInterface("org.freedesktop.portal.Request"),
		dbus.WithMatchMember("Response"),
	}

	if err := conn.AddMatchSignal(matchOptions...); err != nil {
		return nil, err
	}
	defer conn.RemoveMatchSignal(matchOptions...) //nolint:errcheck

	ch := make(chan *dbus.Signal, 10)
	conn.Signal(ch)
	defer conn.RemoveSignal(ch)

	portal := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")

	token := fmt.Sprintf("h%d", time.Now().UnixNano())

	opts := map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(token),
		"interactive":  dbus.MakeVariant(false),
	}

	var handle dbus.ObjectPath

	if err := portal.Call("org.freedesktop.portal.Screenshot.Screenshot", 0, "", opts).Store(&handle); err != nil {
		return nil, fmt.Errorf("screenshot portal: %w", err)
	}

	// Long enough for someone at the machine to answer the portal's permission prompt.
	results, err := waitForPortalResponse(ch, handle, time.Minute)
	if err != nil {
		return nil, err
	}

	uriVar, ok := results["uri"]
	if !ok {
		return nil, fmt.Errorf("no uri in screenshot response")
	}

	uri, ok := uriVar.Value().(string)
	if !ok {
		return nil, fmt.Errorf("unexpected uri type %T", uriVar.Value())
	}

	path := fileURIToPath(uri)
	if path == "" {
		return nil, fmt.Errorf("could not resolve screenshot path from URI: %s", uri)
	}

	defer os.Remove(path)

	return os.ReadFile(path)
}

func waitForPortalResponse(ch chan *dbus.Signal, handle dbus.ObjectPath,
	timeout time.Duration) (map[string]dbus.Variant, error) {
	timer := time.After(timeout)

	for {
		select {
		case sig := <-ch:
			if sig == nil {
				continue
			}

			if sig.Path != handle || len(sig.Body) < 2 {
				continue
			}

			respCode, ok := sig.Body[0].(uint32)
			if !ok {
				return nil, fmt.Errorf("unexpected response code type %T", sig.Body[0])
			}

			if err := portalResponseError(respCode); err != nil {
				return nil, err
			}

			results, ok := sig.Body[1].(map[string]dbus.Variant)
			if !ok {
				return nil, fmt.Errorf("unexpected response type %T", sig.Body[1])
			}

			return results, nil

		case <-timer:
			return nil, fmt.Errorf("timed out waiting for portal response after %s", timeout)
		}
	}
}

func portalResponseError(respCode uint32) error {
	switch respCode {
	case 0:
		return nil
	case 1:
		return fmt.Errorf("screenshot cancelled")
	case 2:
		return fmt.Errorf("screen capture isn't allowed on this machine: request permission and accept the prompt there")
	default:
		return fmt.Errorf("screenshot failed")
	}
}

// LeaveAppScope moves this process into its own systemd scope when it was started inside
// an app's scope (e.g. a terminal's or a snap's). The portal names the app asking for a
// screenshot after that scope, and GNOME only shows the permission prompt to a named app
// while it's focused; in its own scope the app is unnamed and the prompt always shows.
func LeaveAppScope(conn *dbus.Conn) error {
	cgroup, err := os.ReadFile("/proc/self/cgroup")
	if err != nil {
		return err
	}

	unit := strings.TrimSpace(string(cgroup))
	unit = unit[strings.LastIndex(unit, "/")+1:]
	if !strings.HasPrefix(unit, "app-") && !strings.HasPrefix(unit, "snap.") {
		return nil
	}

	type property struct {
		Name  string
		Value dbus.Variant
	}
	type auxUnit struct {
		Name  string
		Props []property
	}

	pid := uint32(os.Getpid()) //nolint:gosec // pids are well below 2^32
	name := fmt.Sprintf("remotecontrol-app-%d.scope", pid)
	props := []property{{"PIDs", dbus.MakeVariant([]uint32{pid})}}

	return conn.Object("org.freedesktop.systemd1", "/org/freedesktop/systemd1").Call(
		"org.freedesktop.systemd1.Manager.StartTransientUnit", 0, name, "fail", props, []auxUnit{}).Err
}
