package remotecontrol

import (
	"context"
	"sync"

	"github.com/xconnio/deskconn/common"
	"github.com/xconnio/xconn-go"
)

// ProcedureKeyExchange is this app's own key exchange, the same as deskconnd's
// common.ProcedureRemoteControlKeyExchange, which xlink exposes to remote clients. Every
// other procedure here is end-to-end encrypted with the keys it derives.
const ProcedureKeyExchange = "io.xconn.deskconn.deskconnd.remotecontrol.key.exchange"

const metaTopicSessionLeave = "wamp.session.on_leave"

// sessionKeys are one caller's keys, from this side: sendKey encrypts results and
// receiveKey decrypts requests.
type sessionKeys struct {
	sendKey    []byte
	receiveKey []byte
}

// keyStore holds each caller's keys, by its session ID.
type keyStore struct {
	mu   sync.Mutex
	keys map[uint64]*sessionKeys
}

func newKeyStore() *keyStore {
	return &keyStore{keys: map[uint64]*sessionKeys{}}
}

func (k *keyStore) store(sessionID uint64, keys *sessionKeys) {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.keys[sessionID] = keys
}

func (k *keyStore) fetch(sessionID uint64) (*sessionKeys, bool) {
	k.mu.Lock()
	defer k.mu.Unlock()
	keys, ok := k.keys[sessionID]
	return keys, ok
}

func (k *keyStore) delete(sessionID uint64) {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.keys, sessionID)
}

func (r *RemoteControl) handleKeyExchange(_ context.Context, inv *xconn.Invocation) *xconn.InvocationResult {
	clientPublicKey, err := inv.ArgBytes(0)
	if err != nil {
		return xconn.NewInvocationError(common.ErrInvalidArgument, err.Error())
	}

	serverPublicKey, keys, err := serverKeyExchange(clientPublicKey)
	if err != nil {
		return xconn.NewInvocationError(common.ErrOperationFailed, err.Error())
	}
	r.keys.store(inv.Caller(), keys)

	return xconn.NewInvocationResult(serverPublicKey)
}

// serverKeyExchange is deskconnd's key exchange: X25519 with a new key pair, then the
// same HKDF labels, so clients derive their keys the same way for both.
func serverKeyExchange(clientPublicKey []byte) ([]byte, *sessionKeys, error) {
	serverPublicKey, serverPrivateKey, err := common.CreateX25519KeyPair()
	if err != nil {
		return nil, nil, err
	}
	sharedSecret, err := common.PerformKeyExchange(serverPrivateKey, clientPublicKey)
	if err != nil {
		return nil, nil, err
	}
	sendKey, err := common.DeriveKeyHKDF(sharedSecret, []byte("backendToFrontend"))
	if err != nil {
		return nil, nil, err
	}
	receiveKey, err := common.DeriveKeyHKDF(sharedSecret, []byte("frontendToBackend"))
	if err != nil {
		return nil, nil, err
	}
	return serverPublicKey, &sessionKeys{sendKey: sendKey, receiveKey: receiveKey}, nil
}

// handleSessionLeave drops the keys of a caller that left.
func (r *RemoteControl) handleSessionLeave(event *xconn.Event) {
	sessionID, err := event.ArgUInt64(0)
	if err != nil {
		return
	}
	r.keys.delete(sessionID)
}
