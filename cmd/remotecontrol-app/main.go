package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
	log "github.com/sirupsen/logrus"

	"github.com/xconnio/deskconn/common"
	remotecontrol "github.com/xconnio/remotecontrol-app"
	"github.com/xconnio/xconn-go"
)

// reconnectDelay is how long to wait before dialing deskconnd's socket again after it
// went away or wasn't up yet.
const reconnectDelay = 5 * time.Second

func main() {
	cfgDirectory, err := common.CfgDirectory()
	if err != nil {
		log.Fatal(err)
	}

	isDesktop := os.Getenv("DISPLAY") != "" || os.Getenv("WAYLAND_DISPLAY") != ""

	var screen *remotecontrol.Screen
	var mpris *remotecontrol.MPRIS
	var audio *remotecontrol.Audio

	if isDesktop {
		systemBus, err := dbus.ConnectSystemBus()
		if err != nil {
			log.Fatal(err)
		}
		defer systemBus.Close()

		sessionBus, err := dbus.ConnectSessionBus()
		if err != nil {
			log.Fatal(err)
		}
		defer sessionBus.Close()

		if err := remotecontrol.LeaveAppScope(sessionBus); err != nil {
			log.WithError(err).Warn("couldn't leave the app scope, the screenshot permission prompt may not show")
		}

		screen = remotecontrol.NewScreen(sessionBus, systemBus)
		mpris = remotecontrol.NewMPRIS(sessionBus)
		audio = remotecontrol.NewAudio()
		defer audio.Close()
	} else {
		log.Println("no display detected (DISPLAY/WAYLAND_DISPLAY unset), " +
			"running in server mode: only printing is served")
	}

	app := remotecontrol.New(screen, mpris, audio, isDesktop)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	socketURI := fmt.Sprintf("unix://%s", filepath.Join(cfgDirectory, "deskconn.sock"))
	for {
		if err := serve(ctx, app, socketURI); err != nil {
			log.WithError(err).Warn("not connected to deskconnd, retrying")
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(reconnectDelay):
		}
	}
}

// serve registers app's procedures on deskconnd's local socket and returns once that
// connection ends or ctx is done.
func serve(ctx context.Context, app *remotecontrol.RemoteControl, socketURI string) error {
	session, err := xconn.ConnectAnonymous(ctx, socketURI, common.LocalRealm)
	if err != nil {
		return err
	}
	defer func() { _ = session.Leave() }()

	if err := app.Register(session); err != nil {
		return fmt.Errorf("failed to register procedures: %w", err)
	}
	log.Println("registered remote-control procedures with deskconnd")

	select {
	case <-ctx.Done():
		return nil
	case <-session.Done():
		return fmt.Errorf("connection to deskconnd closed")
	}
}
