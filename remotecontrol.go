package remotecontrol

import (
	"context"
	"maps"

	log "github.com/sirupsen/logrus"

	"github.com/xconnio/deskconn/common"
	"github.com/xconnio/wampproto-go"
	"github.com/xconnio/xconn-go"
)

// RemoteControl serves the machine's remote-control procedures: screen brightness and
// lock, screenshots, media players (MPRIS), audio and printing. It registers them on
// deskconnd's local socket, and xlink forwards remote clients' calls to it.
type RemoteControl struct {
	screen  *Screen
	mpris   *MPRIS
	audio   *Audio
	printer *Printer
	desktop bool
}

// New returns a RemoteControl; screen, mpris and audio are only used when desktop is set.
func New(screen *Screen, mpris *MPRIS, audio *Audio, desktop bool) *RemoteControl {
	return &RemoteControl{
		screen:  screen,
		mpris:   mpris,
		audio:   audio,
		printer: NewPrinter(),
		desktop: desktop,
	}
}

func (r *RemoteControl) Register(session *xconn.Session) error {
	handlers := map[string]xconn.InvocationHandler{
		common.ProcedurePrinterList:  r.printer.handleListPrinters,
		common.ProcedurePrinterPrint: r.printer.handlePrint(),
	}

	// Display-related RPCs depend on a session D-Bus connection (screen, MPRIS) or
	// PulseAudio (audio), neither of which is available on a headless server. Skip
	// registering them there.
	if r.desktop {
		maps.Copy(handlers, map[string]xconn.InvocationHandler{
			common.ProcedureScreenBrightnessGet:  r.brightnessGetHandler,
			common.ProcedureScreenBrightnessSet:  r.brightnessSetHandler,
			common.ProcedureScreenLock:           r.lockScreenLockHandler,
			common.ProcedureScreenIsLocked:       r.lockScreenIsLockedHandler,
			common.ProcedureMPRISPlayers:         r.handleListPlayers,
			common.ProcedureMPRISPlayPause:       r.handlePlayPause,
			common.ProcedureMPRISPlay:            r.handlePlay,
			common.ProcedureMPRISPause:           r.handlePause,
			common.ProcedureMPRISNext:            r.handleNext,
			common.ProcedureMPRISPrevious:        r.handlePrevious,
			common.ProcedureAudioMute:            r.handleAudioMute,
			common.ProcedureAudioUnmute:          r.handleAudioUnmute,
			common.ProcedureAudioToggleMute:      r.handleAudioToggleMute,
			common.ProcedureAudioIsMuted:         r.handleAudioIsMuted,
			common.ProcedureScreenshot:           r.handleScreenshot,
			common.ProcedureScreenshotPermission: r.handleScreenShotPermission,
		})
	}

	for uri, handler := range handlers {
		response := session.Register(uri, handler).Invoke(wampproto.InvokeLast).Do()
		if response.Err != nil {
			return response.Err
		}

		log.Printf("Registered procedure %s", uri)
	}
	return nil
}

func (r *RemoteControl) brightnessGetHandler(_ context.Context, _ *xconn.Invocation) *xconn.InvocationResult {
	brightness, err := r.screen.GetBrightness()
	if err != nil {
		return xconn.NewInvocationError(common.ErrInvalidArgument, err.Error())
	}

	return xconn.NewInvocationResult(brightness)
}

func (r *RemoteControl) brightnessSetHandler(_ context.Context, inv *xconn.Invocation) *xconn.InvocationResult {
	brightness, err := inv.ArgInt64(0)
	if err != nil {
		return xconn.NewInvocationError(common.ErrInvalidArgument, err)
	}

	if err := r.screen.SetBrightness(int(brightness)); err != nil {
		return xconn.NewInvocationError(common.ErrOperationFailed, err)
	}

	return xconn.NewInvocationResult()
}

func (r *RemoteControl) lockScreenLockHandler(_ context.Context, _ *xconn.Invocation) *xconn.InvocationResult {
	if err := r.screen.Lock(); err != nil {
		return xconn.NewInvocationError(common.ErrOperationFailed, err)
	}

	return xconn.NewInvocationResult()
}

func (r *RemoteControl) lockScreenIsLockedHandler(_ context.Context, _ *xconn.Invocation) *xconn.InvocationResult {
	isLocked, err := r.screen.IsLocked()
	if err != nil {
		return xconn.NewInvocationError(common.ErrOperationFailed, err)
	}

	return xconn.NewInvocationResult(isLocked)
}

func (r *RemoteControl) handleListPlayers(_ context.Context, _ *xconn.Invocation) *xconn.InvocationResult {
	players, err := r.mpris.ListPlayers()
	if err != nil {
		return xconn.NewInvocationError(common.ErrOperationFailed, err.Error())
	}
	return xconn.NewInvocationResult(players)
}

func (r *RemoteControl) handlePlayPause(_ context.Context, inv *xconn.Invocation) *xconn.InvocationResult {
	player, err := inv.ArgString(0)

	var playPauseErr error
	if err != nil {
		playPauseErr = r.mpris.PlayPause()
	} else {
		playPauseErr = r.mpris.PlayPausePlayer(player)
	}

	if playPauseErr != nil {
		return xconn.NewInvocationError(common.ErrOperationFailed, playPauseErr.Error())
	}

	return xconn.NewInvocationResult()
}

func (r *RemoteControl) handlePlay(_ context.Context, inv *xconn.Invocation) *xconn.InvocationResult {
	player, err := inv.ArgString(0)

	var playErr error
	if err != nil {
		playErr = r.mpris.Play()
	} else {
		playErr = r.mpris.PlayPlayer(player)
	}

	if playErr != nil {
		return xconn.NewInvocationError(common.ErrOperationFailed, playErr.Error())
	}

	return xconn.NewInvocationResult()
}

func (r *RemoteControl) handlePause(_ context.Context, inv *xconn.Invocation) *xconn.InvocationResult {
	player, err := inv.ArgString(0)

	var pauseErr error
	if err != nil {
		pauseErr = r.mpris.Pause()
	} else {
		pauseErr = r.mpris.PausePlayer(player)
	}

	if pauseErr != nil {
		return xconn.NewInvocationError(common.ErrOperationFailed, pauseErr.Error())
	}

	return xconn.NewInvocationResult()
}

func (r *RemoteControl) handleNext(_ context.Context, inv *xconn.Invocation) *xconn.InvocationResult {
	player, err := inv.ArgString(0)

	var nextErr error
	if err != nil {
		nextErr = r.mpris.Next()
	} else {
		nextErr = r.mpris.NextPlayer(player)
	}

	if nextErr != nil {
		return xconn.NewInvocationError(common.ErrOperationFailed, nextErr.Error())
	}

	return xconn.NewInvocationResult()
}

func (r *RemoteControl) handlePrevious(_ context.Context, inv *xconn.Invocation) *xconn.InvocationResult {
	player, err := inv.ArgString(0)

	var previousErr error
	if err != nil {
		previousErr = r.mpris.Previous()
	} else {
		previousErr = r.mpris.PreviousPlayer(player)
	}

	if previousErr != nil {
		return xconn.NewInvocationError(common.ErrOperationFailed, previousErr.Error())
	}

	return xconn.NewInvocationResult()
}

func (r *RemoteControl) handleAudioMute(_ context.Context, _ *xconn.Invocation) *xconn.InvocationResult {
	if err := r.audio.Mute(); err != nil {
		return xconn.NewInvocationError(common.ErrOperationFailed, err.Error())
	}
	return xconn.NewInvocationResult()
}

func (r *RemoteControl) handleAudioUnmute(_ context.Context, _ *xconn.Invocation) *xconn.InvocationResult {
	if err := r.audio.Unmute(); err != nil {
		return xconn.NewInvocationError(common.ErrOperationFailed, err.Error())
	}
	return xconn.NewInvocationResult()
}

func (r *RemoteControl) handleAudioToggleMute(_ context.Context, _ *xconn.Invocation) *xconn.InvocationResult {
	muted, err := r.audio.ToggleMute()
	if err != nil {
		return xconn.NewInvocationError(common.ErrOperationFailed, err.Error())
	}
	return xconn.NewInvocationResult(muted)
}

func (r *RemoteControl) handleAudioIsMuted(_ context.Context, _ *xconn.Invocation) *xconn.InvocationResult {
	muted, err := r.audio.IsMuted()
	if err != nil {
		return xconn.NewInvocationError(common.ErrOperationFailed, err.Error())
	}
	return xconn.NewInvocationResult(muted)
}

func (r *RemoteControl) handleScreenshot(_ context.Context, _ *xconn.Invocation) *xconn.InvocationResult {
	data, err := r.screen.Screenshot()
	if err != nil {
		log.WithError(err).Error("screenshot failed")
		return xconn.NewInvocationError(common.ErrOperationFailed, err.Error())
	}
	return xconn.NewInvocationResult(data)
}

func (r *RemoteControl) handleScreenShotPermission(_ context.Context, _ *xconn.Invocation) *xconn.InvocationResult {
	_, err := CaptureScreenshot(r.screen.sessionBus)
	if err != nil {
		return xconn.NewInvocationError(common.ErrOperationFailed, err.Error())
	}

	return xconn.NewInvocationResult()
}
