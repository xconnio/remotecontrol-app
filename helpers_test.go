package remotecontrol_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/xconnio/deskconn/common"
	remotecontrol "github.com/xconnio/remotecontrol-app"
)

// Changing a setting rewrites only its own section of config.yml.
func TestSettingsKeepOtherConfigSections(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgDirectory := filepath.Join(home, ".deskconn")
	require.NoError(t, os.MkdirAll(cfgDirectory, 0700))
	cfgPath := filepath.Join(cfgDirectory, "config.yml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(`machines:
  - authid: cloud
profiles:
  - name: office
    url: quic://host:18080
    private_key: abc
printing:
  mode: accept
`), 0600))

	require.NoError(t, remotecontrol.SetPrintMode(common.PrintModeHost))

	data, err := os.ReadFile(cfgPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "private_key: abc", "a section this app doesn't know is kept")
	require.Contains(t, string(data), "authid: cloud")
	mode, err := remotecontrol.CurrentPrintMode()
	require.NoError(t, err)
	require.Equal(t, common.PrintModeHost, mode)
}

func TestSettingWithoutConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cfgDirectory := filepath.Join(home, ".deskconn")
	require.NoError(t, os.MkdirAll(cfgDirectory, 0700))
	require.NoError(t, remotecontrol.SetPrintMode(common.PrintModeHost))

	data, err := os.ReadFile(filepath.Join(cfgDirectory, "config.yml"))
	require.NoError(t, err)
	require.Equal(t, "printing:\n    mode: host\n", string(data))
}
