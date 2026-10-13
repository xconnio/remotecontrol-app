package remotecontrol

import (
	"github.com/godbus/dbus/v5"

	"github.com/xconnio/deskconn/common"
)

// EnableScreenshot/DisableScreenshot/ScreenshotEnabled are purely local
// machine settings (config.yml read/write, no machine/network round trip),
// so they are read and written directly.
func EnableScreenshot(cfgDirectory string) error {
	return updateScreenshotConfig(cfgDirectory, true)
}

func DisableScreenshot(cfgDirectory string) error {
	return updateScreenshotConfig(cfgDirectory, false)
}

func updateScreenshotConfig(cfgDirectory string, enabled bool) error {
	return setConfigSection(cfgDirectory, "screenshot", common.ScreenshotConfig{Enabled: enabled})
}

// RevokeScreenshotPermission clears the screenshot entry from the portal
// permission store, e.g. when the machine rejects a permission-check call.
func RevokeScreenshotPermission(conn *dbus.Conn) error {
	obj := conn.Object("org.freedesktop.impl.portal.PermissionStore",
		"/org/freedesktop/impl/portal/PermissionStore")

	return obj.Call("org.freedesktop.impl.portal.PermissionStore.Delete", 0, "screenshot", "screenshot").Err
}
