package remotecontrol_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/require"

	remotecontrol "github.com/xconnio/remotecontrol-app"
)

func TestLock(t *testing.T) {
	ls := &remotecontrol.Screen{}

	err := ls.Lock()
	require.EqualError(t, err, "screen lock provider not initialized")
}

func TestIsLocked(t *testing.T) {
	ls := &remotecontrol.Screen{}

	_, err := ls.IsLocked()
	require.EqualError(t, err, "screen lock provider not initialized")
}

func mockBacklightDir(t *testing.T) string {
	t.Helper()

	tmp := t.TempDir()

	dev := filepath.Join(tmp, "intel_backlight")
	err := os.Mkdir(dev, 0755)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dev, "max_brightness"), []byte("100"), 0600)
	require.NoError(t, err)

	err = os.WriteFile(filepath.Join(dev, "brightness"), []byte("20"), 0600)
	require.NoError(t, err)

	old := remotecontrol.BacklightBasePath
	t.Cleanup(func() { remotecontrol.BacklightBasePath = old })
	remotecontrol.BacklightBasePath = tmp

	return tmp
}

func TestNewBrightnessDeviceFound(t *testing.T) {
	mockBacklightDir(t)

	conn, err := dbus.ConnectSystemBus()
	require.NoError(t, err)
	sessionConn, err := dbus.ConnectSessionBus()
	require.NoError(t, err)
	b := remotecontrol.NewScreen(sessionConn, conn, t.TempDir())

	brightness, err := b.GetBrightness()
	require.NoError(t, err)
	require.Equal(t, 20, brightness)
}

func TestNewBrightnessNoDevice(t *testing.T) {
	old := remotecontrol.BacklightBasePath
	defer func() { remotecontrol.BacklightBasePath = old }()

	remotecontrol.BacklightBasePath = t.TempDir()

	conn, err := dbus.ConnectSystemBus()
	require.NoError(t, err)
	sessionConn, err := dbus.ConnectSessionBus()
	require.NoError(t, err)
	b := remotecontrol.NewScreen(sessionConn, conn, t.TempDir())

	err = b.SetBrightness(70)
	require.EqualError(t, err, "brightness device not available")

	_, err = b.GetBrightness()
	require.EqualError(t, err, "brightness device not available")
}

func TestGetBrightness(t *testing.T) {
	mockBacklightDir(t)

	conn, err := dbus.ConnectSystemBus()
	require.NoError(t, err)
	sessionConn, err := dbus.ConnectSessionBus()
	require.NoError(t, err)
	b := remotecontrol.NewScreen(sessionConn, conn, t.TempDir())
	value, err := b.GetBrightness()
	require.NoError(t, err)
	require.Equal(t, 20, value)
}

func TestGetBrightnessFileError(t *testing.T) {
	tmp := mockBacklightDir(t)

	// Remove brightness file to trigger error
	err := os.Remove(filepath.Join(tmp, "intel_backlight", "brightness"))
	require.NoError(t, err)

	remotecontrol.BacklightBasePath = tmp

	conn, err := dbus.ConnectSystemBus()
	require.NoError(t, err)
	sessionConn, err := dbus.ConnectSessionBus()
	require.NoError(t, err)
	b := remotecontrol.NewScreen(sessionConn, conn, t.TempDir())

	_, err = b.GetBrightness()
	require.Error(t, err)
}
