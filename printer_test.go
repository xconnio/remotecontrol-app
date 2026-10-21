package remotecontrol_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"

	"github.com/xconnio/deskconn/appkit"
	remotecontrol "github.com/xconnio/remotecontrol-app"
)

func saveConfigFile(t *testing.T) func() {
	t.Helper()
	cfgDir, err := appkit.CfgDirectory()
	require.NoError(t, err)
	cfgPath := filepath.Join(cfgDir, "config.yml")

	original, readErr := os.ReadFile(cfgPath)
	return func() {
		if readErr == nil {
			_ = os.WriteFile(cfgPath, original, 0600)
		} else {
			_ = os.Remove(cfgPath)
		}
	}
}

func TestCurrentPrintModeNoConfig(t *testing.T) {
	t.Cleanup(saveConfigFile(t))

	cfgDir, err := appkit.CfgDirectory()
	require.NoError(t, err)
	_ = os.Remove(filepath.Join(cfgDir, "config.yml"))

	mode, err := remotecontrol.CurrentPrintMode()
	require.NoError(t, err)
	require.Equal(t, remotecontrol.PrintModeDisabled, mode)
}

func TestCurrentPrintModeAccept(t *testing.T) {
	t.Cleanup(saveConfigFile(t))
	require.NoError(t, remotecontrol.SetPrintMode(remotecontrol.PrintModeAccept))

	mode, err := remotecontrol.CurrentPrintMode()
	require.NoError(t, err)
	require.Equal(t, remotecontrol.PrintModeAccept, mode)
}

func TestCurrentPrintModeHost(t *testing.T) {
	t.Cleanup(saveConfigFile(t))
	require.NoError(t, remotecontrol.SetPrintMode(remotecontrol.PrintModeHost))

	mode, err := remotecontrol.CurrentPrintMode()
	require.NoError(t, err)
	require.Equal(t, remotecontrol.PrintModeHost, mode)
}

func TestCurrentPrintModeDisabled(t *testing.T) {
	t.Cleanup(saveConfigFile(t))
	require.NoError(t, remotecontrol.SetPrintMode(remotecontrol.PrintModeAccept))
	require.NoError(t, remotecontrol.SetPrintMode(remotecontrol.PrintModeDisabled))

	mode, err := remotecontrol.CurrentPrintMode()
	require.NoError(t, err)
	require.Equal(t, remotecontrol.PrintModeDisabled, mode)
}

func TestCurrentPrintModeEmptyModeField(t *testing.T) {
	t.Cleanup(saveConfigFile(t))

	cfgDir, err := appkit.CfgDirectory()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(cfgDir, "config.yml"),
		[]byte("machines: []\n"),
		0600,
	))

	mode, err := remotecontrol.CurrentPrintMode()
	require.NoError(t, err)
	require.Equal(t, remotecontrol.PrintModeDisabled, mode)
}

func TestCurrentPrintModeUnknownMode(t *testing.T) {
	t.Cleanup(saveConfigFile(t))

	cfgDir, err := appkit.CfgDirectory()
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(
		filepath.Join(cfgDir, "config.yml"),
		[]byte("printing:\n  mode: bogus\n"),
		0600,
	))

	_, err = remotecontrol.CurrentPrintMode()
	require.ErrorContains(t, err, "unsupported print mode")
}

func TestSetPrintModeUnsupported(t *testing.T) {
	err := remotecontrol.SetPrintMode("invalid-mode")
	require.ErrorContains(t, err, "unsupported print mode")
}

func TestSetPrintModePreservesExistingConfig(t *testing.T) {
	t.Cleanup(saveConfigFile(t))

	cfgDir, err := appkit.CfgDirectory()
	require.NoError(t, err)

	type minConfig struct {
		Machines []map[string]string `yaml:"machines"`
	}
	seed := minConfig{Machines: []map[string]string{{"authid": "dev1", "id": "id1"}}}
	data, err := yaml.Marshal(seed)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(cfgDir, "config.yml"), data, 0600))

	require.NoError(t, remotecontrol.SetPrintMode(remotecontrol.PrintModeAccept))

	data, err = os.ReadFile(filepath.Join(cfgDir, "config.yml"))
	require.NoError(t, err)
	var saved minConfig
	require.NoError(t, yaml.Unmarshal(data, &saved))
	require.Equal(t, seed.Machines, saved.Machines, "SetPrintMode must not erase other config sections")
}

func TestEnablePrinting(t *testing.T) {
	t.Cleanup(saveConfigFile(t))
	require.NoError(t, remotecontrol.EnablePrinting())

	mode, err := remotecontrol.CurrentPrintMode()
	require.NoError(t, err)
	require.Equal(t, remotecontrol.PrintModeAccept, mode)
}

func TestEnablePrinterHosting(t *testing.T) {
	t.Cleanup(saveConfigFile(t))
	require.NoError(t, remotecontrol.EnablePrinterHosting())

	mode, err := remotecontrol.CurrentPrintMode()
	require.NoError(t, err)
	require.Equal(t, remotecontrol.PrintModeHost, mode)
}

func TestDisablePrinting(t *testing.T) {
	t.Cleanup(saveConfigFile(t))
	require.NoError(t, remotecontrol.EnablePrinting())

	require.NoError(t, remotecontrol.DisablePrinting())

	mode, err := remotecontrol.CurrentPrintMode()
	require.NoError(t, err)
	require.Equal(t, remotecontrol.PrintModeDisabled, mode)
}

func TestDisablePrintingIdempotent(t *testing.T) {
	t.Cleanup(saveConfigFile(t))
	require.NoError(t, remotecontrol.DisablePrinting())
	require.NoError(t, remotecontrol.DisablePrinting())

	mode, err := remotecontrol.CurrentPrintMode()
	require.NoError(t, err)
	require.Equal(t, remotecontrol.PrintModeDisabled, mode)
}
