/*
Copyright SecureKey Technologies Inc. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/
package chconfig

import (
	reqContext "context"
	"testing"

	"time"

	"github.com/golang/protobuf/proto"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/providers/context"
	"github.com/hyperledger/fabric-sdk-go/pkg/common/providers/fab"
	contextImpl "github.com/hyperledger/fabric-sdk-go/pkg/context"
	"github.com/hyperledger/fabric-sdk-go/pkg/fab/mocks"
	"github.com/hyperledger/fabric-sdk-go/pkg/fab/orderer"
	mspmocks "github.com/hyperledger/fabric-sdk-go/pkg/msp/test/mockmsp"
	"github.com/pkg/errors"

	"strings"

	"github.com/hyperledger/fabric-sdk-go/pkg/common/errors/retry"
	"github.com/stretchr/testify/assert"
)

const (
	channelID = "testChannel"
)

func TestChannelConfigWithPeer(t *testing.T) {

	ctx := setupTestContext()
	peer := getPeerWithConfigBlockPayload(t)

	channelConfig, err := New(channelID, WithPeers([]fab.Peer{peer}), WithMinResponses(1), WithMaxTargets(1))
	if err != nil {
		t.Fatalf("Failed to create new channel client: %s", err)
	}

	reqCtx, cancel := contextImpl.NewRequest(ctx, contextImpl.WithTimeout(10*time.Second))
	defer cancel()

	cfg, err := channelConfig.Query(reqCtx)
	if err != nil {
		t.Fatalf(err.Error())
	}

	if cfg.ID() != channelID {
		t.Fatalf("Channel name error. Expecting %s, got %s ", channelID, cfg.ID())
	}
}

func TestChannelConfigWithPeerWithRetries(t *testing.T) {

	numberOfAttempts := 7
	user := mspmocks.NewMockSigningIdentity("test", "test")
	ctx := mocks.NewMockContext(user)

	defRetryOpts := retry.DefaultOpts
	defRetryOpts.Attempts = numberOfAttempts
	defRetryOpts.InitialBackoff = 5 * time.Millisecond
	defRetryOpts.BackoffFactor = 1.0

	chConfig := &fab.ChannelNetworkConfig{
		Policies: fab.ChannelPolicies{QueryChannelConfig: fab.QueryChannelConfigPolicy{
			MinResponses: 2,
			MaxTargets:   1, //Ignored since we pass targets
			RetryOpts:    defRetryOpts,
		}},
	}

	mockConfig := &customMockConfig{MockConfig: &mocks.MockConfig{}, chConfig: chConfig}
	ctx.SetEndpointConfig(mockConfig)

	peer1 := getPeerWithConfigBlockPayload(t)
	peer2 := getPeerWithConfigBlockPayload(t)

	channelConfig, err := New(channelID, WithPeers([]fab.Peer{peer1, peer2}))
	if err != nil {
		t.Fatalf("Failed to create new channel client: %s", err)
	}

	//Set custom retry handler for tracking number of attempts
	retryHandler := retry.New(defRetryOpts)
	overrideRetryHandler = &customRetryHandler{handler: retryHandler, retries: 0}

	reqCtx, cancel := contextImpl.NewRequest(ctx, contextImpl.WithTimeout(100*time.Second))
	defer cancel()

	_, err = channelConfig.Query(reqCtx)
	if err == nil || !strings.Contains(err.Error(), "ENDORSEMENT_MISMATCH") {
		t.Fatalf("Supposed to fail with ENDORSEMENT_MISMATCH. Description: payloads for config block do not match")
	}

	assert.True(t, overrideRetryHandler.(*customRetryHandler).retries-1 == numberOfAttempts, "number of attempts missmatching")
}

func TestChannelConfigWithPeerError(t *testing.T) {

	ctx := setupTestContext()
	peer := getPeerWithConfigBlockPayload(t)

	channelConfig, err := New(channelID, WithPeers([]fab.Peer{peer}), WithMinResponses(2))
	if err != nil {
		t.Fatalf("Failed to create new channel client: %s", err)
	}

	reqCtx, cancel := contextImpl.NewRequest(ctx, contextImpl.WithTimeout(10*time.Second))
	defer cancel()

	_, err = channelConfig.Query(reqCtx)
	if err == nil {
		t.Fatalf("Should have failed with since there's one endorser and at least two are required")
	}
}

func TestChannelConfigWithOrdererError(t *testing.T) {

	ctx := setupTestContext()
	o, err := orderer.New(ctx.EndpointConfig(), orderer.WithURL("localhost:9999"))
	assert.Nil(t, err)
	channelConfig, err := New(channelID, WithOrderer(o))
	if err != nil {
		t.Fatalf("Failed to create new channel client: %s", err)
	}

	reqCtx, cancel := contextImpl.NewRequest(ctx, contextImpl.WithTimeout(1*time.Second))
	defer cancel()

	// Expecting error since orderer is not setup
	_, err = channelConfig.Query(reqCtx)
	if err == nil {
		t.Fatalf("Should have failed since orderer is not available")
	}

}

func TestRandomMaxTargetsSelections(t *testing.T) {

	testTargets := []fab.ProposalProcessor{
		&mockProposalProcessor{"ONE"}, &mockProposalProcessor{"TWO"}, &mockProposalProcessor{"THREE"},
		&mockProposalProcessor{"FOUR"}, &mockProposalProcessor{"FIVE"}, &mockProposalProcessor{"SIX"},
		&mockProposalProcessor{"SEVEN"}, &mockProposalProcessor{"EIGHT"}, &mockProposalProcessor{"NINE"},
	}

	max := 3
	before := ""
	for _, v := range testTargets[:max] {
		before = before + v.(*mockProposalProcessor).name
	}

	responseTargets := randomMaxTargets(testTargets, max)
	assert.True(t, responseTargets != nil && len(responseTargets) == max, "response target not as expected")

	after := ""
	for _, v := range responseTargets {
		after = after + v.(*mockProposalProcessor).name
	}
	//make sure it is random
	assert.False(t, before == after, "response targets are not random")

	max = 0 //when zero minimum supplied, result should be empty
	responseTargets = randomMaxTargets(testTargets, max)
	assert.True(t, responseTargets != nil && len(responseTargets) == max, "response target not as expected")

	max = 12 //greater than targets length
	responseTargets = randomMaxTargets(testTargets, max)
	assert.True(t, responseTargets != nil && len(responseTargets) == len(testTargets), "response target not as expected")

}

func TestResolveOptsFromConfig(t *testing.T) {
	user := mspmocks.NewMockSigningIdentity("test", "test")
	ctx := mocks.NewMockContext(user)

	defRetryOpts := retry.DefaultOpts

	chConfig := &fab.ChannelNetworkConfig{
		Policies: fab.ChannelPolicies{QueryChannelConfig: fab.QueryChannelConfigPolicy{
			MinResponses: 8,
			MaxTargets:   9,
			RetryOpts:    defRetryOpts,
		}},
	}

	mockConfig := &customMockConfig{MockConfig: &mocks.MockConfig{}, chConfig: chConfig}
	ctx.SetEndpointConfig(mockConfig)

	channelConfig, err := New(channelID, WithPeers([]fab.Peer{}))
	if err != nil {
		t.Fatal("Failed to create channel config")
	}

	assert.True(t, channelConfig.opts.MaxTargets == 0, "supposed to be zero when not resolved")
	assert.True(t, channelConfig.opts.MinResponses == 0, "supposed to be zero when not resolved")
	assert.True(t, channelConfig.opts.RetryOpts.RetryableCodes == nil, "supposed to be nil when not resolved")

	channelConfig, err = New(channelID, WithPeers([]fab.Peer{}), WithMinResponses(2))
	if err != nil {
		t.Fatal("Failed to create channel config")
	}

	assert.True(t, channelConfig.opts.MaxTargets == 0, "supposed to be zero when not resolved")
	assert.True(t, channelConfig.opts.MinResponses == 2, "supposed to be loaded with options")
	assert.True(t, channelConfig.opts.RetryOpts.RetryableCodes == nil, "supposed to be nil when not resolved")

	mockConfig.called = false
	channelConfig.resolveOptsFromConfig(ctx)

	assert.True(t, channelConfig.opts.MaxTargets == 9, "supposed to be loaded once opts resolved from config")
	assert.True(t, channelConfig.opts.MinResponses == 2, "supposed to be updated once loaded with non zero value")
	assert.True(t, channelConfig.opts.RetryOpts.RetryableCodes != nil, "supposed to be loaded once opts resolved from config")
	assert.True(t, mockConfig.called, "config.ChannelConfig() not used by resolve opts function")

	//Try again, opts shouldnt get reloaded from config once loaded
	mockConfig.called = false
	channelConfig.resolveOptsFromConfig(ctx)
	assert.False(t, mockConfig.called, "config.ChannelConfig() should not be used by resolve opts function once opts are loaded")
}

func TestResolveOptsDefaultValues(t *testing.T) {
	user := mspmocks.NewMockSigningIdentity("test", "test")
	ctx := mocks.NewMockContext(user)

	mockConfig := &customMockConfig{MockConfig: &mocks.MockConfig{}, chConfig: nil}
	ctx.SetEndpointConfig(mockConfig)

	channelConfig, err := New(channelID, WithPeers([]fab.Peer{}))
	if err != nil {
		t.Fatal("Failed to create channel config")
	}
	err = channelConfig.resolveOptsFromConfig(ctx)
	if err != nil {
		t.Fatal("Failed to resolve opts from config")
	}
	assert.True(t, channelConfig.opts.MaxTargets == 2, "supposed to be loaded once opts resolved from config")
	assert.True(t, channelConfig.opts.MinResponses == 1, "supposed to be loaded once opts resolved from config")
	assert.True(t, channelConfig.opts.RetryOpts.RetryableCodes != nil, "supposed to be loaded once opts resolved from config")
}

func setupTestContext() context.Client {
	user := mspmocks.NewMockSigningIdentity("test", "test")
	ctx := mocks.NewMockContext(user)
	ctx.SetEndpointConfig(mocks.NewMockEndpointConfig())
	return ctx
}

func getPeerWithConfigBlockPayload(t *testing.T) fab.Peer {

	// create config block builder in order to create valid payload
	builder := &mocks.MockConfigBlockBuilder{
		MockConfigGroupBuilder: mocks.MockConfigGroupBuilder{
			ModPolicy: "Admins",
			MSPNames: []string{
				"Org1MSP",
				"Org2MSP",
			},
			OrdererAddress: "localhost:7054",
			RootCA:         validRootCA,
		},
		Index:           0,
		LastConfigIndex: 0,
	}

	payload, err := proto.Marshal(builder.Build())
	if err != nil {
		t.Fatalf("Failed to marshal mock block")
	}

	// peer with valid config block payload
	peer := &mocks.MockPeer{MockName: "Peer1", MockURL: "http://peer1.com", MockRoles: []string{}, MockCert: nil, Payload: payload, Status: 200}

	return peer
}

//mockProposalProcessor to mock proposal processor for random max target test
type mockProposalProcessor struct {
	name string
}

func (pp *mockProposalProcessor) ProcessTransactionProposal(reqCtx reqContext.Context, request fab.ProcessProposalRequest) (*fab.TransactionProposalResponse, error) {
	return nil, errors.New("not implemented, just mock")
}

//customMockConfig to mock config to override channel configuration options
type customMockConfig struct {
	*mocks.MockConfig
	chConfig *fab.ChannelNetworkConfig
	called   bool
}

func (c *customMockConfig) ChannelConfig(name string) (*fab.ChannelNetworkConfig, error) {
	c.called = true
	return c.chConfig, nil
}

//customRetryHandler is wrapper around retry handler which keeps count of attempts for unit-testing
type customRetryHandler struct {
	handler retry.Handler
	retries int
}

func (c *customRetryHandler) Required(err error) bool {
	c.retries++
	return c.handler.Required(err)
}

var validRootCA = `-----BEGIN CERTIFICATE-----
MIICYjCCAgmgAwIBAgIUB3CTDOU47sUC5K4kn/Caqnh114YwCgYIKoZIzj0EAwIw
fzELMAkGA1UEBhMCVVMxEzARBgNVBAgTCkNhbGlmb3JuaWExFjAUBgNVBAcTDVNh
biBGcmFuY2lzY28xHzAdBgNVBAoTFkludGVybmV0IFdpZGdldHMsIEluYy4xDDAK
BgNVBAsTA1dXVzEUMBIGA1UEAxMLZXhhbXBsZS5jb20wHhcNMTYxMDEyMTkzMTAw
WhcNMjExMDExMTkzMTAwWjB/MQswCQYDVQQGEwJVUzETMBEGA1UECBMKQ2FsaWZv
cm5pYTEWMBQGA1UEBxMNU2FuIEZyYW5jaXNjbzEfMB0GA1UEChMWSW50ZXJuZXQg
V2lkZ2V0cywgSW5jLjEMMAoGA1UECxMDV1dXMRQwEgYDVQQDEwtleGFtcGxlLmNv
bTBZMBMGByqGSM49AgEGCCqGSM49AwEHA0IABKIH5b2JaSmqiQXHyqC+cmknICcF
i5AddVjsQizDV6uZ4v6s+PWiJyzfA/rTtMvYAPq/yeEHpBUB1j053mxnpMujYzBh
MA4GA1UdDwEB/wQEAwIBBjAPBgNVHRMBAf8EBTADAQH/MB0GA1UdDgQWBBQXZ0I9
qp6CP8TFHZ9bw5nRtZxIEDAfBgNVHSMEGDAWgBQXZ0I9qp6CP8TFHZ9bw5nRtZxI
EDAKBggqhkjOPQQDAgNHADBEAiAHp5Rbp9Em1G/UmKn8WsCbqDfWecVbZPQj3RK4
oG5kQQIgQAe4OOKYhJdh3f7URaKfGTf492/nmRmtK+ySKjpHSrU=
-----END CERTIFICATE-----
`
