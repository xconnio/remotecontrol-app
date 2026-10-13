package remotecontrol_test

import (
	"encoding/json"
	"testing"

	"github.com/godbus/dbus/v5"
	"github.com/stretchr/testify/require"

	"github.com/xconnio/deskconn/common"
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
	return sessions
}

// keyExchange performs the app's key exchange as a client would.
func keyExchange(t *testing.T, caller *xconn.Session) *common.SessionKeys {
	t.Helper()
	publicKey, privateKey, err := common.CreateX25519KeyPair()
	require.NoError(t, err)
	resp := caller.Call(remotecontrol.ProcedureKeyExchange).Arg(publicKey).Do()
	require.NoError(t, resp.Err)
	serverPublicKey, err := resp.ArgBytes(0)
	require.NoError(t, err)
	sendKey, receiveKey, err := common.ClientKeyExchangeKeys(privateKey, serverPublicKey)
	require.NoError(t, err)
	return &common.SessionKeys{SendKey: sendKey, ReceiveKey: receiveKey}
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
	return common.EncryptedCall(caller, procedure, payload, keyExchange(t, caller))
}

func TestBrightnessGetSet(t *testing.T) {
	sessions := setupRouterAndConnectSessions(t, 2)
	callee, caller := sessions[0], sessions[1]

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

	result, err := call(t, caller, common.ProcedureScreenBrightnessGet, nil)
	if err != nil {
		// Headless / DBus unavailable case
		require.ErrorContains(t, err, "brightness device not available")
		return
	}

	var initial int
	require.NoError(t, json.Unmarshal(result, &initial))
	require.GreaterOrEqual(t, initial, 0)
	require.LessOrEqual(t, initial, 100)

	_, err = call(t, caller, common.ProcedureScreenBrightnessSet, nil)
	require.ErrorContains(t, err, "wamp.error.invalid_argument")

	_, err = call(t, caller, common.ProcedureScreenBrightnessSet, 70)
	require.NoError(t, err)

	result, err = call(t, caller, common.ProcedureScreenBrightnessGet, nil)
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

	_, err := call(t, caller, common.ProcedureScreenLock, nil)
	require.ErrorContains(t, err, "wamp.error.no_such_procedure")

	// Registered, though refused while printing is off.
	_, err = call(t, caller, common.ProcedurePrinterPrint,
		map[string]any{"printer": "p", "filename": "f.txt", "data": []byte("x")})
	require.Error(t, err)
	require.NotContains(t, err.Error(), "no_such_procedure")
}

func TestRequiresKeyExchange(t *testing.T) {
	sessions := setupRouterAndConnectSessions(t, 3)
	callee, caller, other := sessions[0], sessions[1], sessions[2]

	r := remotecontrol.New(nil, nil, nil, false)
	require.NoError(t, r.Register(callee))

	resp := caller.Call(common.ProcedurePrinterList).Do()
	require.ErrorContains(t, resp.Err, "call key exchange first")

	// Keys from another caller's exchange don't count.
	keys := keyExchange(t, caller)
	_, err := common.EncryptedCall(other, common.ProcedurePrinterList, nil, keys)
	require.ErrorContains(t, err, "call key exchange first")
}
