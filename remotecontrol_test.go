package remotecontrol_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/require"

	"github.com/xconnio/deskconn/appkit"
	remotecontrol "github.com/xconnio/remotecontrol-app"
	"github.com/xconnio/xconn-go"
)

// setupRouterAndConnectSessions connects n sessions to a new router.
func setupRouterAndConnectSessions(t *testing.T, n int) []*xconn.Session {
	r, err := xconn.NewRouter(&xconn.RouterConfig{})
	require.NoError(t, err)

	// Like deskconnd's local realm, so each caller gets its own keys.
	realmConfig := xconn.DefaultRealmConfig()
	realmConfig.AutoDiscloseCaller = true
	require.NoError(t, r.AddRealm("realm1", realmConfig))

	sessions := make([]*xconn.Session, n)
	for i := range sessions {
		sessions[i], err = xconn.ConnectInMemory(r, "realm1")
		require.NoError(t, err)
	}

	// deskconnd's side: every app registered is listed, enabled.
	deskconnd, err := xconn.ConnectInMemory(r, "realm1")
	require.NoError(t, err)
	var apps []map[string]any
	require.NoError(t, deskconnd.Register(appkit.ProcedureAppsRegister,
		func(_ context.Context, inv *xconn.Invocation) *xconn.InvocationResult {
			payload, err := inv.ArgBytes(0)
			require.NoError(t, err)
			require.NoError(t, json.Unmarshal(payload, &apps))
			for _, app := range apps {
				require.Equal(t, remotecontrol.ProcedureKeyExchange, app["key_exchange"])
				app["enabled"] = true
			}
			return xconn.NewInvocationResult()
		}).Do().Err)
	require.NoError(t, deskconnd.Register(appkit.ProcedureCapabilitiesList,
		func(_ context.Context, _ *xconn.Invocation) *xconn.InvocationResult {
			return xconn.NewInvocationResult(apps)
		}).Do().Err)
	return sessions
}

// keyExchange performs the app's key exchange as a client would.
func keyExchange(t *testing.T, caller *xconn.Session) *appkit.SessionKeys {
	t.Helper()
	keys, err := appkit.ClientKeyExchange(caller, remotecontrol.ProcedureKeyExchange)
	require.NoError(t, err)
	return keys
}

// call calls procedure as a client would: after a key exchange, with req as encrypted
// JSON, and returns the decrypted JSON result.
func call(t *testing.T, caller *xconn.Session, procedure string, req any) ([]byte, error) {
	t.Helper()
	var payload []byte
	if req != nil {
		var err error
		payload, err = json.Marshal(req)
		require.NoError(t, err)
	}
	return appkit.EncryptedCall(caller, procedure, payload, keyExchange(t, caller))
}

func TestBrightnessGetSet(t *testing.T) {
	sessions := setupRouterAndConnectSessions(t, 2)
	callee, caller := sessions[0], sessions[1]

	conn, err := dbus.ConnectSystemBus()
	require.NoError(t, err)
	sessionConn, err := dbus.ConnectSessionBus()
	require.NoError(t, err)
	screen := remotecontrol.NewScreen(sessionConn, conn)
	mpris := remotecontrol.NewMPRIS(sessionConn)
	audio := remotecontrol.NewAudio()
	defer audio.Close()
	r := remotecontrol.New(screen, mpris, audio, true)
	require.NoError(t, r.Register(callee))

	result, err := call(t, caller, remotecontrol.ProcedureScreenBrightnessGet, nil)
	if err != nil {
		// Headless / DBus unavailable case
		require.ErrorContains(t, err, "brightness device not available")
		return
	}

	var initial int
	require.NoError(t, json.Unmarshal(result, &initial))
	require.GreaterOrEqual(t, initial, 0)
	require.LessOrEqual(t, initial, 100)

	_, err = call(t, caller, remotecontrol.ProcedureScreenBrightnessSet, nil)
	require.ErrorContains(t, err, "wamp.error.invalid_argument")

	_, err = call(t, caller, remotecontrol.ProcedureScreenBrightnessSet, 70)
	require.NoError(t, err)

	result, err = call(t, caller, remotecontrol.ProcedureScreenBrightnessGet, nil)
	require.NoError(t, err)
	var updated int
	require.NoError(t, json.Unmarshal(result, &updated))
	require.GreaterOrEqual(t, updated, 0)
	require.LessOrEqual(t, updated, 100)
}

func TestServerRegistersOnlyPrinter(t *testing.T) {
	sessions := setupRouterAndConnectSessions(t, 2)
	callee, caller := sessions[0], sessions[1]

	r := remotecontrol.New(nil, nil, nil, false)
	require.NoError(t, r.Register(callee))

	_, err := call(t, caller, remotecontrol.ProcedureScreenLock, nil)
	require.ErrorContains(t, err, "wamp.error.no_such_procedure")

	// Registered, though refused while printing is off.
	_, err = call(t, caller, remotecontrol.ProcedurePrinterPrint,
		map[string]any{"printer": "p", "filename": "f.txt", "data": []byte("x")})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "no_such_procedure")
}

func TestRequiresKeyExchange(t *testing.T) {
	sessions := setupRouterAndConnectSessions(t, 3)
	callee, caller, other := sessions[0], sessions[1], sessions[2]

	r := remotecontrol.New(nil, nil, nil, false)
	require.NoError(t, r.Register(callee))

	resp := caller.Call(remotecontrol.ProcedurePrinterList).Do()
	require.ErrorContains(t, resp.Err, "call key exchange first")

	// Keys from another caller's exchange don't count.
	keys := keyExchange(t, caller)
	_, err := appkit.EncryptedCall(other, remotecontrol.ProcedurePrinterList, nil, keys)
	require.ErrorContains(t, err, "call key exchange first")
}

func TestFollowsEnabledApps(t *testing.T) {
	sessions := setupRouterAndConnectSessions(t, 3)
	callee, caller, deskconnd := sessions[0], sessions[1], sessions[2]

	r := remotecontrol.New(nil, nil, nil, false)
	require.NoError(t, r.Register(callee))

	setPrinter := func(enabled bool) {
		apps := []map[string]any{{"id": "printer", "enabled": enabled}}
		require.NoError(t, deskconnd.Publish(appkit.TopicCapabilitiesChanged).Args(apps).Do().Err)
	}
	listPrinters := func() error {
		_, err := call(t, caller, remotecontrol.ProcedurePrinterList, nil)
		return err
	}

	setPrinter(false)
	require.Eventually(t, func() bool {
		err := listPrinters()
		return err != nil && strings.Contains(err.Error(), "no_such_procedure")
	}, time.Second, 10*time.Millisecond)

	setPrinter(true)
	require.Eventually(t, func() bool {
		err := listPrinters()
		return err == nil || !strings.Contains(err.Error(), "no_such_procedure")
	}, time.Second, 10*time.Millisecond)
}
