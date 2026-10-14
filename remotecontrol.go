package remotecontrol

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"sync"

	log "github.com/sirupsen/logrus"

	"github.com/xconnio/deskconn/common"
	"github.com/xconnio/wampproto-go"
	"github.com/xconnio/xconn-go"
)

// ProcedureKeyExchange is this app's own key exchange. Every other procedure here is
// end-to-end encrypted with the keys it derives.
const ProcedureKeyExchange = "io.xconn.deskconn.deskconnd.remotecontrol.key.exchange"

const (
	ProcedureScreenBrightnessGet = "io.xconn.deskconn.deskconnd.screen.brightness.get"
	ProcedureScreenBrightnessSet = "io.xconn.deskconn.deskconnd.screen.brightness.set"
	ProcedureScreenLock          = "io.xconn.deskconn.deskconnd.screen.lock"
	ProcedureScreenIsLocked      = "io.xconn.deskconn.deskconnd.screen.islocked"

	ProcedureMPRISPlayers   = "io.xconn.deskconn.deskconnd.mpris.players"
	ProcedureMPRISPlayPause = "io.xconn.deskconn.deskconnd.mpris.playpause"
	ProcedureMPRISPlay      = "io.xconn.deskconn.deskconnd.mpris.play"
	ProcedureMPRISPause     = "io.xconn.deskconn.deskconnd.mpris.pause"
	ProcedureMPRISNext      = "io.xconn.deskconn.deskconnd.mpris.next"
	ProcedureMPRISPrevious  = "io.xconn.deskconn.deskconnd.mpris.previous"

	ProcedureAudioMute       = "io.xconn.deskconn.deskconnd.audio.mute"
	ProcedureAudioUnmute     = "io.xconn.deskconn.deskconnd.audio.unmute"
	ProcedureAudioToggleMute = "io.xconn.deskconn.deskconnd.audio.togglemute"
	ProcedureAudioIsMuted    = "io.xconn.deskconn.deskconnd.audio.ismuted"

	ProcedureScreenshot           = "io.xconn.deskconn.deskconnd.screenshot"
	ProcedureScreenshotPermission = "io.xconn.deskconn.deskconnd.screenshot.permission"

	ProcedurePrinterList  = "io.xconn.deskconn.deskconnd.printer.list"
	ProcedurePrinterPrint = "io.xconn.deskconn.deskconnd.printer.print"
)

// RemoteControl serves the machine's remote-control apps: screen brightness and lock,
// screenshots, media players (MPRIS), audio and printing.
//
// It registers them with deskconnd and serves each app's procedures while the app is
// enabled. They're end-to-end encrypted with the keys of this app's own key exchange
// (ProcedureKeyExchange): every procedure takes its request as a single encrypted JSON
// argument, empty or absent when it has none, and returns a single encrypted JSON result.
type RemoteControl struct {
	screen  *Screen
	mpris   *MPRIS
	audio   *Audio
	printer *Printer
	enc     *common.AppEncryption
	desktop bool

	mu      sync.Mutex
	session *xconn.Session
	// regs are the registered procedures, by URI.
	regs map[string]xconn.RegisterResponse
}

// New returns a RemoteControl; screen, mpris and audio are only used when desktop is set.
func New(screen *Screen, mpris *MPRIS, audio *Audio, desktop bool) *RemoteControl {
	return &RemoteControl{
		screen:  screen,
		mpris:   mpris,
		audio:   audio,
		printer: NewPrinter(),
		enc:     common.NewAppEncryption(),
		desktop: desktop,
	}
}

// app is one app as deskconnd's ProcedureAppsRegister takes it, with its procedures.
type app struct {
	ID          string                       `json:"id"`
	Name        string                       `json:"name"`
	KeyExchange string                       `json:"key_exchange"`
	DefaultOff  bool                         `json:"default_off,omitempty"`
	procedures  map[string]common.AppHandler `json:"-"`
}

func (r *RemoteControl) apps() []*app {
	apps := []*app{{ID: "printer", Name: "Printer", procedures: map[string]common.AppHandler{
		ProcedurePrinterList:  r.printer.listPrinters,
		ProcedurePrinterPrint: r.printer.print,
	}}}

	// Display-related apps depend on a session D-Bus connection (screen, MPRIS) or
	// PulseAudio (audio), neither of which is available on a headless server.
	if r.desktop {
		apps = append(apps,
			&app{ID: "screen", Name: "Screen", procedures: map[string]common.AppHandler{
				ProcedureScreenBrightnessGet: r.brightnessGet,
				ProcedureScreenBrightnessSet: r.brightnessSet,
				ProcedureScreenLock:          r.screenLock,
				ProcedureScreenIsLocked:      r.screenIsLocked,
			}},
			// Off until an owner or admin enables it: it shows what's on the screen.
			&app{ID: "screenshot", Name: "Screenshot", DefaultOff: true, procedures: map[string]common.AppHandler{
				ProcedureScreenshot:           r.screenshot,
				ProcedureScreenshotPermission: r.screenshotPermission,
			}},
			&app{ID: "media-player", Name: "Media Player", procedures: map[string]common.AppHandler{
				ProcedureMPRISPlayers:   r.listPlayers,
				ProcedureMPRISPlayPause: r.playerAction(r.mpris.PlayPause, r.mpris.PlayPausePlayer),
				ProcedureMPRISPlay:      r.playerAction(r.mpris.Play, r.mpris.PlayPlayer),
				ProcedureMPRISPause:     r.playerAction(r.mpris.Pause, r.mpris.PausePlayer),
				ProcedureMPRISNext:      r.playerAction(r.mpris.Next, r.mpris.NextPlayer),
				ProcedureMPRISPrevious:  r.playerAction(r.mpris.Previous, r.mpris.PreviousPlayer),
			}},
			&app{ID: "audio", Name: "Audio", procedures: map[string]common.AppHandler{
				ProcedureAudioMute:       r.audioMute,
				ProcedureAudioUnmute:     r.audioUnmute,
				ProcedureAudioToggleMute: r.audioToggleMute,
				ProcedureAudioIsMuted:    r.audioIsMuted,
			}},
		)
	}
	for _, a := range apps {
		a.KeyExchange = ProcedureKeyExchange
	}
	return apps
}

// Register registers the apps with deskconnd on session, its local socket, and serves the
// enabled ones' procedures there, following deskconnd as they're enabled and disabled.
func (r *RemoteControl) Register(session *xconn.Session) error {
	r.mu.Lock()
	r.session = session
	r.regs = map[string]xconn.RegisterResponse{}
	r.mu.Unlock()

	if err := r.enc.Register(session, ProcedureKeyExchange); err != nil {
		return err
	}
	if resp := session.Subscribe(common.TopicCapabilitiesChanged, r.handleCapabilitiesChanged).Do(); resp.Err != nil {
		return resp.Err
	}
	payload, err := json.Marshal(r.apps())
	if err != nil {
		return err
	}
	if resp := session.Call(common.ProcedureAppsRegister).Args(payload).Do(); resp.Err != nil {
		return resp.Err
	}
	resp := session.Call(common.ProcedureCapabilitiesList).Do()
	if resp.Err != nil {
		return resp.Err
	}
	if len(resp.Args()) == 0 {
		return errors.New("deskconnd returned no apps list")
	}
	return r.subscribe(resp.Args()[0])
}

func (r *RemoteControl) handleCapabilitiesChanged(event *xconn.Event) {
	if len(event.Args()) == 0 {
		return
	}
	if err := r.subscribe(event.Args()[0]); err != nil {
		log.WithError(err).Error("failed to follow enabled apps")
	}
}

// appState is an app as deskconnd's apps list has it.
type appState struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

// subscribe registers the procedures of the apps that list, deskconnd's apps list, has
// enabled, and unsubscribes the rest.
func (r *RemoteControl) subscribe(list any) error {
	data, err := json.Marshal(list)
	if err != nil {
		return err
	}
	var states []appState
	if err := json.Unmarshal(data, &states); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for _, a := range r.apps() {
		enabled := slices.ContainsFunc(states, func(s appState) bool { return s.ID == a.ID && s.Enabled })
		if !enabled {
			if err := r.unsubscribe(a); err != nil {
				return err
			}
			continue
		}
		for uri, h := range a.procedures {
			if _, ok := r.regs[uri]; ok {
				continue
			}
			resp := r.session.Register(uri, r.enc.Encrypted(h)).Invoke(wampproto.InvokeLast).Do()
			if resp.Err != nil {
				return resp.Err
			}
			r.regs[uri] = resp
			log.Printf("Registered procedure %s", uri)
		}
	}
	return nil
}

// unsubscribe unregisters a's procedures. r must be locked.
func (r *RemoteControl) unsubscribe(a *app) error {
	for uri := range a.procedures {
		reg, ok := r.regs[uri]
		if !ok {
			continue
		}
		if err := reg.Unregister(); err != nil {
			return err
		}
		delete(r.regs, uri)
		log.Printf("Unregistered procedure %s", uri)
	}
	return nil
}

// invalidArgument marks err as the caller's, returned as ErrInvalidArgument.
func invalidArgument(err error) error {
	return &xconn.Error{URI: common.ErrInvalidArgument, Args: []any{err.Error()}}
}

// decodeRequest unmarshals req into out, reporting a missing or malformed request as the
// caller's error.
func decodeRequest(req []byte, out any) error {
	if req == nil {
		return invalidArgument(errors.New("missing request"))
	}
	if err := json.Unmarshal(req, out); err != nil {
		return invalidArgument(err)
	}
	return nil
}

func (r *RemoteControl) brightnessGet(_ context.Context, _ []byte) (any, error) {
	return r.screen.GetBrightness()
}

func (r *RemoteControl) brightnessSet(_ context.Context, req []byte) (any, error) {
	var brightness int
	if err := decodeRequest(req, &brightness); err != nil {
		return nil, err
	}
	return nil, r.screen.SetBrightness(brightness)
}

func (r *RemoteControl) screenLock(_ context.Context, _ []byte) (any, error) {
	return nil, r.screen.Lock()
}

func (r *RemoteControl) screenIsLocked(_ context.Context, _ []byte) (any, error) {
	return r.screen.IsLocked()
}

func (r *RemoteControl) listPlayers(_ context.Context, _ []byte) (any, error) {
	return r.mpris.ListPlayers()
}

// playerAction runs all on every player, or one on the player the request names.
func (r *RemoteControl) playerAction(all func() error, one func(string) error) common.AppHandler {
	return func(_ context.Context, req []byte) (any, error) {
		if req == nil {
			return nil, all()
		}
		var player string
		if err := decodeRequest(req, &player); err != nil {
			return nil, err
		}
		return nil, one(player)
	}
}

func (r *RemoteControl) audioMute(_ context.Context, _ []byte) (any, error) {
	return nil, r.audio.Mute()
}

func (r *RemoteControl) audioUnmute(_ context.Context, _ []byte) (any, error) {
	return nil, r.audio.Unmute()
}

func (r *RemoteControl) audioToggleMute(_ context.Context, _ []byte) (any, error) {
	return r.audio.ToggleMute()
}

func (r *RemoteControl) audioIsMuted(_ context.Context, _ []byte) (any, error) {
	return r.audio.IsMuted()
}

// imageResult is an image as the procedures return it; Data is base64 in JSON.
type imageResult struct {
	Mime string `json:"mime"`
	Data []byte `json:"data"`
}

func (r *RemoteControl) screenshot(_ context.Context, _ []byte) (any, error) {
	data, err := r.screen.Screenshot()
	if err != nil {
		log.WithError(err).Error("screenshot failed")
		return nil, err
	}
	return imageResult{Mime: http.DetectContentType(data), Data: data}, nil
}

func (r *RemoteControl) screenshotPermission(_ context.Context, _ []byte) (any, error) {
	_, err := CaptureScreenshot(r.screen.sessionBus)
	return nil, err
}
