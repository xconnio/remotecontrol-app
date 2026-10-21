package remotecontrol

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/xconnio/deskconn/appkit"
)

// The print mode is a purely local machine setting, read and written straight to
// config.yml with no network round trip.

// PrintMode is how the machine's printers are shared, kept in config.yml's printing section.
type PrintMode string

const (
	PrintModeDisabled PrintMode = "disabled"
	PrintModeAccept   PrintMode = "accept"
	PrintModeHost     PrintMode = "host"
)

// printingConfig is config.yml's printing section.
type printingConfig struct {
	Mode PrintMode `yaml:"mode,omitempty"`
}

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

func CurrentPrintMode() (PrintMode, error) {
	cfgDirectory, err := appkit.CfgDirectory()
	if err != nil {
		return PrintModeDisabled, err
	}
	data, err := os.ReadFile(filepath.Join(cfgDirectory, "config.yml"))
	if err != nil {
		if os.IsNotExist(err) {
			return PrintModeDisabled, nil
		}
		return PrintModeDisabled, err
	}
	var config struct {
		Printing printingConfig `yaml:"printing"`
	}
	if err := yaml.Unmarshal(data, &config); err != nil {
		return PrintModeDisabled, err
	}
	switch config.Printing.Mode {
	case PrintModeAccept, PrintModeHost:
		return config.Printing.Mode, nil
	case PrintModeDisabled, "":
		return PrintModeDisabled, nil
	default:
		return PrintModeDisabled, fmt.Errorf("unsupported print mode %q", config.Printing.Mode)
	}
}

func EnablePrinting() error {
	return SetPrintMode(PrintModeAccept)
}

func EnablePrinterHosting() error {
	return SetPrintMode(PrintModeHost)
}

func SetPrintMode(mode PrintMode) error {
	switch mode {
	case PrintModeAccept, PrintModeHost, PrintModeDisabled:
		return updatePrintingConfig(mode)
	default:
		return fmt.Errorf("unsupported print mode %q", mode)
	}
}

func DisablePrinting() error {
	return SetPrintMode(PrintModeDisabled)
}

func updatePrintingConfig(mode PrintMode) error {
	cfgDirectory, err := appkit.CfgDirectory()
	if err != nil {
		return err
	}
	return setConfigSection(cfgDirectory, "printing", printingConfig{Mode: mode})
}
