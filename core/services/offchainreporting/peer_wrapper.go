package offchainreporting

import (
	"github.com/jinzhu/gorm"
	"github.com/pkg/errors"
	"github.com/smartcontractkit/chainlink/core/logger"
	"github.com/smartcontractkit/chainlink/core/store/models"
	"github.com/smartcontractkit/chainlink/core/store/orm"
	ocrnetworking "github.com/smartcontractkit/libocr/networking"
	ocrtypes "github.com/smartcontractkit/libocr/offchainreporting/types"
)

// PeerWrapper is a convenient wrapper encapsulating both types of ocr factory
type (
	peer interface {
		ocrtypes.BootstrapperFactory
		ocrtypes.BinaryNetworkEndpointFactory
	}

	SingletonPeerWrapper struct {
		pstoreWrapper *Pstorewrapper
		PeerID        models.PeerID
		Peer          peer
	}
)

func (p SingletonPeerWrapper) Start() error {
	return p.pstoreWrapper.Start()
}
func (p SingletonPeerWrapper) Close() error {
	return p.pstoreWrapper.Close()
}

// NewSingletonPeerWrapper creates a new peer based on the p2p keys in the keystore
// It currently only supports one peerID/key
func NewSingletonPeerWrapper(keyStore *KeyStore, config *orm.Config, db *gorm.DB) (*SingletonPeerWrapper, error) {
	p2pkeys := keyStore.DecryptedP2PKeys()
	listenPort := config.P2PListenPort()
	if listenPort == 0 {
		return nil, errors.New("failed to instantiate oracle or bootstrapper service, P2P_LISTEN_PORT is required and must be set to a non-zero value")
	}

	// If the P2PAnnounceIP is set we must also set the P2PAnnouncePort
	// Fallback to P2PListenPort if it wasn't made explicit
	var announcePort uint16
	if config.P2PAnnounceIP() != nil && config.P2PAnnouncePort() != 0 {
		announcePort = config.P2PAnnouncePort()
	} else if config.P2PAnnounceIP() != nil {
		announcePort = listenPort
	}

	peerLogger := NewLogger(logger.Default, config.OCRTraceLogging(), func(string) {})

	if len(p2pkeys) == 0 {
		return nil, errors.New("no p2p keys found")
	} else if len(p2pkeys) > 1 {
		return nil, errors.New("more than one p2p key is not currently supported")
	} else {
		key := p2pkeys[0]
		peerID, err := key.GetPeerID()
		if err != nil {
			return nil, errors.Wrap(err, "could not get peer ID")
		}
		pstorewrapper, err := NewPeerstoreWrapper(db, config.P2PPeerstoreWriteInterval(), peerID)
		if err != nil {
			return nil, errors.Wrap(err, "could not make new pstorewrapper")
		}

		peer, err := ocrnetworking.NewPeer(ocrnetworking.PeerConfig{
			PrivKey:      key.PrivKey,
			ListenIP:     config.P2PListenIP(),
			ListenPort:   listenPort,
			AnnounceIP:   config.P2PAnnounceIP(),
			AnnouncePort: announcePort,
			Logger:       peerLogger,
			Peerstore:    pstorewrapper.Peerstore,
			EndpointConfig: ocrnetworking.EndpointConfig{
				IncomingMessageBufferSize: config.OCRIncomingMessageBufferSize(),
				OutgoingMessageBufferSize: config.OCROutgoingMessageBufferSize(),
				NewStreamTimeout:          config.OCRNewStreamTimeout(),
				DHTLookupInterval:         config.OCRDHTLookupInterval(),
				BootstrapCheckInterval:    config.OCRBootstrapCheckInterval(),
			},
			DHTAnnouncementCounterUserPrefix: config.P2PDHTAnnouncementCounterUserPrefix(),
		})
		if err != nil {
			return nil, errors.Wrap(err, "error calling NewPeer")
		}
		return &SingletonPeerWrapper{
				pstorewrapper,
				key.MustGetPeerID(),
				peer,
			},
			nil
	}
}
