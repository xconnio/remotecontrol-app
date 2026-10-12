package remotecontrol_test

import (
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/require"

	"github.com/xconnio/deskconn/common"
	remotecontrol "github.com/xconnio/remotecontrol-app"
	"github.com/xconnio/xconn-go"
)

func setupRouterAndConnectSessions(t *testing.T) (*xconn.Session, *xconn.Session) {
	r, err := xconn.NewRouter(&xconn.RouterConfig{})
	require.NoError(t, err)

	err = r.AddRealm("realm1", xconn.DefaultRealmConfig())
	require.NoError(t, err)

	callee, err := xconn.ConnectInMemory(r, "realm1")
	require.NoError(t, err)

	caller, err := xconn.ConnectInMemory(r, "realm1")
	require.NoError(t, err)

	return callee, caller
}

func TestBrightnessGetSet(t *testing.T) {
	callee, caller := setupRouterAndConnectSessions(t)

	conn, err := dbus.ConnectSystemBus()
	require.NoError(t, err)
	sessionConn, err := dbus.ConnectSessionBus()
	require.NoError(t, err)
	screen := remotecontrol.NewScreen(sessionConn, conn, t.TempDir())
	mpris := remotecontrol.NewMPRIS(sessionConn)
	audio := remotecontrol.NewAudio()
	defer audio.Close()
	r := remotecontrol.New(screen, mpris, audio, true)
	require.NoError(t, r.Register(callee))

	callResp := caller.Call(common.ProcedureScreenBrightnessGet).Do()
	if callResp.Err != nil {
		// Headless / DBus unavailable case
		require.ErrorContains(t, callResp.Err, "brightness device not available")
		return
	}

	initial := int(callResp.ArgInt64Or(0, 0))
	require.GreaterOrEqual(t, initial, 0)
	require.LessOrEqual(t, initial, 100)

	callResp = caller.Call(common.ProcedureScreenBrightnessSet).Do()
	require.ErrorContains(t, callResp.Err, "wamp.error.invalid_argument")

	callResp = caller.Call(common.ProcedureScreenBrightnessSet).Arg(70).Do()
	require.NoError(t, callResp.Err)

	callResp = caller.Call(common.ProcedureScreenBrightnessGet).Do()
	require.NoError(t, callResp.Err)

	updated := int(callResp.ArgInt64Or(0, 0))
	require.GreaterOrEqual(t, updated, 0)
	require.LessOrEqual(t, updated, 100)
}

func TestServerRegistersOnlyPrinter(t *testing.T) {
	callee, caller := setupRouterAndConnectSessions(t)

	r := remotecontrol.New(nil, nil, nil, false)
	require.NoError(t, r.Register(callee))

	callResp := caller.Call(common.ProcedureScreenLock).Do()
	require.ErrorContains(t, callResp.Err, "wamp.error.no_such_procedure")
}
