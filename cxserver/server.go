package cxserver

import (
	"fmt"
	"os"

	"github.com/mit-dci/opencx/db/ocxsql"
	"github.com/mit-dci/lit/coinparam"
	"github.com/mit-dci/lit/btcutil/hdkeychain"
	"github.com/mit-dci/lit/lnutil"
	"github.com/mit-dci/lit/uspv"
	"github.com/mit-dci/lit/wire"
	"github.com/mit-dci/lit/wallit"
	"github.com/mit-dci/lit/btcutil/base58"
	"github.com/mit-dci/lit/logging"
)

// OpencxServer is how rpc can query the database and whatnot
type OpencxServer struct {
	OpencxDB             *ocxsql.DB
	OpencxRoot           string
	OpencxPort           int
	// Hehe it's the vault, pls don't steal
	OpencxBTCTestPrivKey *hdkeychain.ExtendedKey
	OpencxVTCTestPrivKey *hdkeychain.ExtendedKey
	OpencxLTCTestPrivKey *hdkeychain.ExtendedKey
	HeightEventChan      chan lnutil.HeightEvent

	// TODO: Put TLS stuff here
	// TODO: Or implement client required signatures and pubkeys instead of usernames
	BTCHook              *uspv.SPVCon
}

// TODO now that I know how to use this hdkeychain stuff, let's figure out how to create addresses to store

// SetupServerKeys just loads a private key from a file wallet
func (server *OpencxServer) SetupServerKeys(keypath string) error {
	privkey, err := lnutil.ReadKeyFile(keypath)
	if err != nil {
		return fmt.Errorf("Error reading key from file: \n%s", err)
	}

	rootBTCKey, err := hdkeychain.NewMaster(privkey[:], &coinparam.TestNet3Params)
	if err != nil {
		return fmt.Errorf("Error creating master BTC Test key from private key: \n%s", err)
	}

	server.OpencxBTCTestPrivKey = rootBTCKey

	rootVTCKey, err := hdkeychain.NewMaster(privkey[:], &coinparam.VertcoinRegTestParams)
	if err != nil {
		return fmt.Errorf("Error creating master VTC Test key from private key: \n%s", err)
	}

	server.OpencxVTCTestPrivKey = rootVTCKey

	rootLTCKey, err := hdkeychain.NewMaster(privkey[:], &coinparam.LiteCoinTestNet4Params)
	if err != nil {
		return fmt.Errorf("Error creating master LTC Test key from private key: \n%s", err)
	}

	server.OpencxLTCTestPrivKey = rootLTCKey

	return nil
}

// SetupWallets ...
func (server *OpencxServer) SetupWallets() error {
	var err error

	btcRoot := server.createSubRoot(coinparam.TestNet3Params.Name)
	// ltcRoot := server.createSubRoot(coinparam.LiteCoinTestNet4Params.Name)
	// vtcRoot := server.createSubRoot(coinparam.VertcoinTestNetParams.Name)

	btcwallet, _, err := wallit.NewWallit(server.OpencxBTCTestPrivKey, coinparam.TestNet3Params.StartHeight, true, "localhost:18333", btcRoot, "", &coinparam.TestNet3Params)
	if err != nil {
		return fmt.Errorf("Error setting up wallet: \n%s", err)
	}

	// vtcwallet, _, err := wallit.NewWallit(server.OpencxVTCTestPrivKey, coinparam.VertcoinTestNetParams.StartHeight, true, "1", vtcRoot, "", &coinparam.VertcoinTestNetParams)
	// if err != nil {
	//	return fmt.Errorf("Error setting up vtc wallet: \n%s", err)
	// }

	// ltcwallet, _, err := wallit.NewWallit(server.OpencxLTCTestPrivKey, coinparam.LiteCoinTestNet4Params.StartHeight, true, "1", ltcRoot, "", &coinparam.LiteCoinTestNet4Params)
	// if err != nil {
	//	return fmt.Errorf("Error setting up ltc wallet: \n%s", err)
	// }

	adrs, err := btcwallet.AdrDump()
	for i, adr := range adrs {
		fmt.Printf("BTC testnet Wallet addr %d: %s\n", i, base58.CheckEncode(adr[:], 0x6f))
	}

	// server.BTCWallet = btcwallet
	// server.LTCWallet = ltcwallet
	// server.VTCWallet = vtcwallet
	return nil
}

// SetupUserAddress sets up and stores an address for the user, and adds it to the addresses to check for when a block comes in
func (server *OpencxServer) SetupUserAddress(username string) error {
	return nil
}

// TODO: check all cases of new() so you properly delete() and don't have any memory leaks, do this before you start using it

// SetupBTCChainhook will be used to watch for events on the chain.
func (server *OpencxServer) SetupBTCChainhook() error {
	btcHook := new(uspv.SPVCon)
	// logging.Debugf(3)

	btcHook.Param = &coinparam.TestNet3Params

	btcRoot := server.createSubRoot(btcHook.Param.Name)

	// Okay now why can I put in "yes" as that parameter or "yup" like that makes no sense as being a remoteNode. "yes" is a remoteNode??
	// maybe isThereAHost should be what its called or something
	logging.Debugf("Starting BTC Chainhook\n")
	_, btcheightChan, err := btcHook.Start(btcHook.Param.StartHeight, "1", btcRoot, "", btcHook.Param)
	if err != nil {
		return fmt.Errorf("Error when starting btc hook: \n%s", err)
	}
	logging.Debugf("BTC Chainhook started\n")

	go server.HeightHandler(btcheightChan, *btcHook.Param)

	server.BTCHook = btcHook
	return nil
}

// HandleBlock handles a block coming in TODO: change coin to not a string if appropriate
func (server *OpencxServer) HandleBlock(block *wire.MsgBlock, coin string) error {
	server.OpencxDB.LogPrintf("Handling block with hash %s for %s chain\n", block.BlockHash().String(), coin)
	return nil
}

// LetMeKnowHeight creates a channel to listen for height events
func(server *OpencxServer) LetMeKnowHeight() chan lnutil.HeightEvent {
	server.HeightEventChan = make(chan lnutil.HeightEvent, 1)
	return server.HeightEventChan
}

// createSubRoot creates sub root directories that hold info for each chain
func (server *OpencxServer) createSubRoot(subRoot string) string {
	subRootDir := server.OpencxRoot + subRoot
	if _, err := os.Stat(subRootDir); os.IsNotExist(err) {
		fmt.Printf("Creating root directory at %s\n", subRootDir)
		os.Mkdir(subRootDir, os.ModePerm)
	}
	return subRootDir
}

// HeightHandler is a handler for different chains' block channels
func (server *OpencxServer) HeightHandler(incomingBlockHeight chan int32, coinType coinparam.Params) {
	for {
		h := <- incomingBlockHeight

		logging.Debugf("A Block on the %s chain came in at height %d!\n", coinType.Name, h)
		if server.HeightEventChan != nil {
			heightEvent := lnutil.HeightEvent{
				Height: h,
				CoinType: coinType.HDCoinType,
			}
			server.HeightEventChan <- heightEvent
		}
	}
}
