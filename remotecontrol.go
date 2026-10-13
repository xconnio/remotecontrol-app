package remotecontrol

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"net/http"

	log "github.com/sirupsen/logrus"

	"github.com/xconnio/deskconn/common"
	"github.com/xconnio/wampproto-go"
	"github.com/xconnio/xconn-go"
)

// RemoteControl serves the machine's remote-control procedures: screen brightness and
// lock, screenshots, media players (MPRIS), audio and printing.
//
// They're end-to-end encrypted with the keys of this app's own key exchange
// (ProcedureKeyExchange): every procedure takes its request as a single encrypted JSON
// argument, empty or absent when it has none, and returns a single encrypted JSON result.
type RemoteControl struct {
	screen  *Screen
	mpris   *MPRIS
	audio   *Audio
	printer *Printer
	keys    *keyStore
	desktop bool
}

// New returns a RemoteControl; screen, mpris and audio are only used when desktop is set.
func New(screen *Screen, mpris *MPRIS, audio *Audio, desktop bool) *RemoteControl {
	return &RemoteControl{
		screen:  screen,
		mpris:   mpris,
		audio:   audio,
		printer: NewPrinter(),
		keys:    newKeyStore(),
		desktop: desktop,
	}
}

// handler serves one remote-control procedure: req is the request's JSON, nil when there
// is none, and the result is marshaled to JSON.
type handler func(ctx context.Context, req []byte) (any, error)

// invalidArgumentError marks an error as the caller's, returned as ErrInvalidArgument.
type invalidArgumentError struct{ error }

func (r *RemoteControl) Register(session *xconn.Session) error {
	handlers := map[string]handler{
		common.ProcedurePrinterList:  r.printer.listPrinters,
		common.ProcedurePrinterPrint: r.printer.print,
	}

	// Display-related RPCs depend on a session D-Bus connection (screen, MPRIS) or
	// PulseAudio (audio), neither of which is available on a headless server. Skip
	// registering them there.
	if r.desktop {
		maps.Copy(handlers, map[string]handler{
			common.ProcedureScreenBrightnessGet:  r.brightnessGet,
			common.ProcedureScreenBrightnessSet:  r.brightnessSet,
			common.ProcedureScreenLock:           r.screenLock,
			common.ProcedureScreenIsLocked:       r.screenIsLocked,
			common.ProcedureMPRISPlayers:         r.listPlayers,
			common.ProcedureMPRISPlayPause:       r.playerAction(r.mpris.PlayPause, r.mpris.PlayPausePlayer),
			common.ProcedureMPRISPlay:            r.playerAction(r.mpris.Play, r.mpris.PlayPlayer),
			common.ProcedureMPRISPause:           r.playerAction(r.mpris.Pause, r.mpris.PausePlayer),
			common.ProcedureMPRISNext:            r.playerAction(r.mpris.Next, r.mpris.NextPlayer),
			common.ProcedureMPRISPrevious:        r.playerAction(r.mpris.Previous, r.mpris.PreviousPlayer),
			common.ProcedureAudioMute:            r.audioMute,
			common.ProcedureAudioUnmute:          r.audioUnmute,
			common.ProcedureAudioToggleMute:      r.audioToggleMute,
			common.ProcedureAudioIsMuted:         r.audioIsMuted,
			common.ProcedureScreenshot:           r.screenshot,
			common.ProcedureScreenshotPermission: r.screenshotPermission,
		})
	}

	invocationHandlers := map[string]xconn.InvocationHandler{ProcedureKeyExchange: r.handleKeyExchange}
	for procedure, h := range handlers {
		invocationHandlers[procedure] = r.encrypted(h)
	}
	for uri, h := range invocationHandlers {
		response := session.Register(uri, h).Invoke(wampproto.InvokeLast).Do()
		if response.Err != nil {
			return response.Err
		}

		log.Printf("Registered procedure %s", uri)
	}

	if resp := session.Subscribe(metaTopicSessionLeave, r.handleSessionLeave).Do(); resp.Err != nil {
		return resp.Err
	}
	return nil
}

// encrypted adapts h to a WAMP procedure: it decrypts the request with the caller's keys
// and encrypts h's JSON result.
func (r *RemoteControl) encrypted(h handler) xconn.InvocationHandler {
	return func(ctx context.Context, inv *xconn.Invocation) *xconn.InvocationResult {
		keys, ok := r.keys.fetch(inv.Caller())
		if !ok {
			return xconn.NewInvocationError(common.ErrInvalidArgument, "no session keys found, call key exchange first")
		}

		var req []byte
		if len(inv.Args()) > 0 {
			encrypted, err := inv.ArgBytes(0)
			if err != nil {
				return xconn.NewInvocationError(common.ErrInvalidArgument, err.Error())
			}
			if req, err = common.DecryptPayload(encrypted, keys.receiveKey); err != nil {
				return xconn.NewInvocationError(common.ErrInvalidArgument, err.Error())
			}
			// An empty request, e.g. a client encrypting no payload, is no request.
			if len(req) == 0 {
				req = nil
			}
		}

		result, err := h(ctx, req)
		if err != nil {
			var argErr invalidArgumentError
			if errors.As(err, &argErr) {
				return xconn.NewInvocationError(common.ErrInvalidArgument, err.Error())
			}
			return xconn.NewInvocationError(common.ErrOperationFailed, err.Error())
		}

		data, err := json.Marshal(result)
		if err != nil {
			return xconn.NewInvocationError(common.ErrOperationFailed, err.Error())
		}
		encryptedResult, err := common.EncryptPayload(data, keys.sendKey)
		if err != nil {
			return xconn.NewInvocationError(common.ErrOperationFailed, err.Error())
		}
		return xconn.NewInvocationResult(encryptedResult)
	}
}

// decodeRequest unmarshals req into out, reporting a missing or malformed request as the
// caller's error.
func decodeRequest(req []byte, out any) error {
	if req == nil {
		return invalidArgumentError{errors.New("missing request")}
	}
	if err := json.Unmarshal(req, out); err != nil {
		return invalidArgumentError{err}
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
func (r *RemoteControl) playerAction(all func() error, one func(string) error) handler {
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
