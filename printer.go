package remotecontrol

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/phin1x/go-ipp"

	"github.com/xconnio/deskconn/common"
)

type Printer struct {
	cups         *printerCUPSClient
	retryDelay   time.Duration
	printingMode func() (common.PrintMode, error)
}

func NewPrinter() *Printer {
	return &Printer{
		cups:       newPrinterCUPSClient("localhost"),
		retryDelay: 2 * time.Second,
	}
}

func (p *Printer) listPrinters(ctx context.Context, _ []byte) (any, error) {
	mode, err := p.currentPrintMode()
	if err != nil {
		return nil, err
	}
	if mode != common.PrintModeHost {
		return nil, errors.New("printer hosting disabled; run `desk print enable --host-printers` to enable")
	}

	infos, err := p.cups.PrintersInfo(ctx)
	if err != nil {
		return nil, err
	}
	return infos, nil
}

// printRequest is ProcedurePrinterPrint's request; Data is base64 in JSON.
type printRequest struct {
	Printer  string `json:"printer"`
	Filename string `json:"filename"`
	Data     []byte `json:"data"`
}

func (p *Printer) print(ctx context.Context, req []byte) (any, error) {
	mode, err := p.currentPrintMode()
	if err != nil {
		return nil, err
	}
	if mode == common.PrintModeDisabled {
		return nil, errors.New("printing disabled; run `desk print enable` to enable")
	}

	var args printRequest
	if err := decodeRequest(req, &args); err != nil {
		return nil, err
	}
	if strings.TrimSpace(args.Printer) == "" {
		return nil, invalidArgument(errors.New("empty printer name"))
	}
	if len(args.Data) == 0 {
		return nil, invalidArgument(errors.New("empty print payload"))
	}

	jobID := fmt.Sprintf("%s-%d", args.Printer, time.Now().UnixNano())
	if err := p.ExecutePrint(ctx, args.Printer, args.Filename, args.Data); err != nil {
		return nil, err
	}
	return jobID, nil
}

func (p *Printer) currentPrintMode() (common.PrintMode, error) {
	if p.printingMode != nil {
		return p.printingMode()
	}
	return CurrentPrintMode()
}

func (p *Printer) ExecutePrint(ctx context.Context, printer string, filename string, data []byte) error {
	tmp, err := os.CreateTemp("", "deskconn-print-*-"+safePrintTempName(filename))
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(tmp.Name()) }()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}

	_, err = p.cups.PrintRaw(ctx, printer, tmp)
	return err
}

type printerCUPSClient struct {
	host string
}

func newPrinterCUPSClient(host string) *printerCUPSClient {
	return &printerCUPSClient{host: host}
}

func (c *printerCUPSClient) cupsClient() *ipp.CUPSClient {
	return ipp.NewCUPSClientWithAdapter("", ipp.NewSocketAdapter(c.host, false))
}

func (c *printerCUPSClient) ippClient() *ipp.IPPClient {
	return ipp.NewIPPClientWithAdapter("", ipp.NewSocketAdapter(c.host, false))
}

func (c *printerCUPSClient) PrintersInfo(ctx context.Context) ([]PrinterInfo, error) {
	printers, err := c.cupsClient().GetPrintersContext(ctx, []string{"printer-name", "ppd-name"})
	if err != nil {
		return nil, err
	}

	result := make([]PrinterInfo, 0, len(printers))
	for name, attrs := range printers {
		result = append(result, PrinterInfo{Name: name, PPDModel: attrString(attrs, "ppd-name")})
	}
	return result, nil
}

func (c *printerCUPSClient) PrintRaw(ctx context.Context, printer string, f *os.File) (int, error) {
	mimeType, err := detectPrintMimeType(f)
	if err != nil {
		return -1, err
	}

	stat, err := f.Stat()
	if err != nil {
		return -1, err
	}

	return c.ippClient().PrintDocumentsContext(ctx, []ipp.Document{{
		Document: f,
		Name:     filepath.Base(f.Name()),
		Size:     int(stat.Size()),
		MimeType: mimeType,
	}}, printer, nil)
}

func detectPrintMimeType(r io.ReadSeeker) (string, error) {
	header := make([]byte, 8)
	n, err := r.Read(header)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	if _, err := r.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	sample := string(header[:n])
	switch {
	case strings.HasPrefix(sample, "%PDF-"):
		return "application/pdf", nil
	case strings.HasPrefix(sample, "%!PS-Adobe-"), strings.HasPrefix(sample, "%!PS"):
		return "application/postscript", nil
	default:
		return "application/octet-stream", nil
	}
}

func attrString(attrs ipp.Attributes, key string) string {
	if vals, ok := attrs[key]; ok && len(vals) > 0 {
		if s, ok := vals[0].Value.(string); ok {
			return s
		}
	}
	return ""
}

func safePrintTempName(filename string) string {
	name := filepath.Base(strings.TrimSpace(filename))
	name = strings.ReplaceAll(name, string(filepath.Separator), "_")
	if name == "" || name == "." {
		return "print-job"
	}
	return name
}
