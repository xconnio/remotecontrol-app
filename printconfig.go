package remotecontrol

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/xconnio/deskconn/common"
)

// The print mode is a purely local machine setting, read and written straight to
// config.yml with no network round trip.

type PrinterInfo struct {
	Name     string `json:"name"`
	PPDModel string `json:"ppd"`
}

type PrintJobStatus struct {
	JobID     string `json:"job_id"`
	Printer   string `json:"printer"`
	State     string `json:"state"`
	Message   string `json:"message,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

func CurrentPrintMode() (common.PrintMode, error) {
	cfgDirectory, err := common.CfgDirectory()
	if err != nil {
		return common.PrintModeDisabled, err
	}
	data, err := os.ReadFile(filepath.Join(cfgDirectory, "config.yml"))
	if err != nil {
		if os.IsNotExist(err) {
			return common.PrintModeDisabled, nil
		}
		return common.PrintModeDisabled, err
	}
	var config common.Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return common.PrintModeDisabled, err
	}
	switch config.Printing.Mode {
	case common.PrintModeAccept, common.PrintModeHost:
		return config.Printing.Mode, nil
	case common.PrintModeDisabled, "":
		return common.PrintModeDisabled, nil
	default:
		return common.PrintModeDisabled, fmt.Errorf("unsupported print mode %q", config.Printing.Mode)
	}
}

func EnablePrinting() error {
	return SetPrintMode(common.PrintModeAccept)
}

func EnablePrinterHosting() error {
	return SetPrintMode(common.PrintModeHost)
}

func SetPrintMode(mode common.PrintMode) error {
	switch mode {
	case common.PrintModeAccept, common.PrintModeHost, common.PrintModeDisabled:
		return updatePrintingConfig(mode)
	default:
		return fmt.Errorf("unsupported print mode %q", mode)
	}
}

func DisablePrinting() error {
	return SetPrintMode(common.PrintModeDisabled)
}

func updatePrintingConfig(mode common.PrintMode) error {
	cfgDirectory, err := common.CfgDirectory()
	if err != nil {
		return err
	}
	return setConfigSection(cfgDirectory, "printing", common.PrintingConfig{Mode: mode})
}
