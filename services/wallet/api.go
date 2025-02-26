package wallet

import (
	"context"
	"fmt"
	"math/big"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/ethclient"
	"github.com/ethereum/go-ethereum/log"
	gethrpc "github.com/ethereum/go-ethereum/rpc"
	"github.com/status-im/status-go/eth-node/types"
	"github.com/status-im/status-go/params"
	"github.com/status-im/status-go/rpc/network"
	"github.com/status-im/status-go/services/wallet/activity"
	"github.com/status-im/status-go/services/wallet/bridge"
	wcommon "github.com/status-im/status-go/services/wallet/common"
	"github.com/status-im/status-go/services/wallet/currency"
	"github.com/status-im/status-go/services/wallet/history"
	"github.com/status-im/status-go/services/wallet/thirdparty"
	"github.com/status-im/status-go/services/wallet/thirdparty/opensea"
	"github.com/status-im/status-go/services/wallet/token"
	"github.com/status-im/status-go/services/wallet/transfer"
	"github.com/status-im/status-go/transactions"
)

func NewAPI(s *Service) *API {
	router := NewRouter(s)
	return &API{s, s.reader, router}
}

// API is class with methods available over RPC.
type API struct {
	s      *Service
	reader *Reader
	router *Router
}

func (api *API) StartWallet(ctx context.Context) error {
	return api.reader.Start()
}

func (api *API) StopWallet(ctx context.Context) error {
	return api.s.Stop()
}

func (api *API) GetWalletToken(ctx context.Context, addresses []common.Address) (map[common.Address][]Token, error) {
	return api.reader.GetWalletToken(ctx, addresses)
}

func (api *API) GetCachedWalletTokensWithoutMarketData(ctx context.Context) (map[common.Address][]Token, error) {
	return api.reader.GetCachedWalletTokensWithoutMarketData()
}

type DerivedAddress struct {
	Address        common.Address `json:"address"`
	PublicKey      types.HexBytes `json:"public-key,omitempty"`
	Path           string         `json:"path"`
	HasActivity    bool           `json:"hasActivity"`
	AlreadyCreated bool           `json:"alreadyCreated"`
}

// SetInitialBlocksRange sets initial blocks range
func (api *API) SetInitialBlocksRange(ctx context.Context) error {
	return api.s.transferController.SetInitialBlocksRange([]uint64{api.s.rpcClient.UpstreamChainID})
}

func (api *API) SetInitialBlocksRangeForChainIDs(ctx context.Context, chainIDs []uint64) error {
	return api.s.transferController.SetInitialBlocksRange(chainIDs)
}

func (api *API) CheckRecentHistory(ctx context.Context, addresses []common.Address) error {
	return api.s.transferController.CheckRecentHistory([]uint64{api.s.rpcClient.UpstreamChainID}, addresses)
}

func (api *API) CheckRecentHistoryForChainIDs(ctx context.Context, chainIDs []uint64, addresses []common.Address) error {
	return api.s.transferController.CheckRecentHistory(chainIDs, addresses)
}

func hexBigToBN(hexBig *hexutil.Big) *big.Int {
	var bN *big.Int
	if hexBig != nil {
		bN = hexBig.ToInt()
	}
	return bN
}

// GetTransfersByAddress returns transfers for a single address
func (api *API) GetTransfersByAddress(ctx context.Context, address common.Address, toBlock, limit *hexutil.Big, fetchMore bool) ([]transfer.View, error) {
	log.Debug("[WalletAPI:: GetTransfersByAddress] get transfers for an address", "address", address)
	var intLimit = int64(1)
	if limit != nil {
		intLimit = limit.ToInt().Int64()
	}
	return api.s.transferController.GetTransfersByAddress(ctx, api.s.rpcClient.UpstreamChainID, address, hexBigToBN(toBlock), intLimit, fetchMore)
}

// LoadTransferByHash loads transfer to the database
// Only used by status-mobile
func (api *API) LoadTransferByHash(ctx context.Context, address common.Address, hash common.Hash) error {
	log.Debug("[WalletAPI:: LoadTransferByHash] get transfer by hash", "address", address, "hash", hash)
	return api.s.transferController.LoadTransferByHash(ctx, api.s.rpcClient, address, hash)
}

func (api *API) GetTransfersByAddressAndChainID(ctx context.Context, chainID uint64, address common.Address, toBlock, limit *hexutil.Big, fetchMore bool) ([]transfer.View, error) {
	log.Debug("[WalletAPI:: GetTransfersByAddressAndChainIDs] get transfers for an address", "address", address)
	return api.s.transferController.GetTransfersByAddress(ctx, chainID, address, hexBigToBN(toBlock), limit.ToInt().Int64(), fetchMore)
}

func (api *API) GetTransfersForIdentities(ctx context.Context, identities []transfer.TransactionIdentity) ([]transfer.View, error) {
	log.Debug("wallet.api.GetTransfersForIdentities", "identities.len", len(identities))

	return api.s.transferController.GetTransfersForIdentities(ctx, identities)
}

func (api *API) FetchDecodedTxData(ctx context.Context, data string) (*thirdparty.DataParsed, error) {
	log.Debug("[Wallet: FetchDecodedTxData]")

	return api.s.decoder.Decode(data)
}

// GetTokensBalances return mapping of token balances for every account.
func (api *API) GetTokensBalances(ctx context.Context, accounts, addresses []common.Address) (map[common.Address]map[common.Address]*hexutil.Big, error) {
	chainClients, err := api.s.rpcClient.EthClients([]uint64{api.s.rpcClient.UpstreamChainID})
	if err != nil {
		return nil, err
	}
	return api.s.tokenManager.GetBalances(ctx, chainClients, accounts, addresses)
}

func (api *API) GetTokensBalancesForChainIDs(ctx context.Context, chainIDs []uint64, accounts, addresses []common.Address) (map[common.Address]map[common.Address]*hexutil.Big, error) {
	clients, err := api.s.rpcClient.EthClients(chainIDs)
	if err != nil {
		return nil, err
	}
	return api.s.tokenManager.GetBalances(ctx, clients, accounts, addresses)
}

func (api *API) UpdateVisibleTokens(ctx context.Context, symbols []string) error {
	api.s.history.UpdateVisibleTokens(symbols)
	return nil
}

// GetBalanceHistory retrieves token balance history for token identity on multiple chains
func (api *API) GetBalanceHistory(ctx context.Context, chainIDs []uint64, address common.Address, tokenSymbol string, currencySymbol string, timeInterval history.TimeInterval) ([]*history.ValuePoint, error) {
	endTimestamp := time.Now().UTC().Unix()
	return api.s.history.GetBalanceHistory(ctx, chainIDs, address, tokenSymbol, currencySymbol, endTimestamp, timeInterval)
}

func (api *API) GetTokens(ctx context.Context, chainID uint64) ([]*token.Token, error) {
	log.Debug("call to get tokens")
	rst, err := api.s.tokenManager.GetTokens(chainID)
	log.Debug("result from token store", "len", len(rst))
	return rst, err
}

func (api *API) GetCustomTokens(ctx context.Context) ([]*token.Token, error) {
	log.Debug("call to get custom tokens")
	rst, err := api.s.tokenManager.GetCustoms()
	log.Debug("result from database for custom tokens", "len", len(rst))
	return rst, err
}

func (api *API) DiscoverToken(ctx context.Context, chainID uint64, address common.Address) (*token.Token, error) {
	log.Debug("call to get discover token")
	token, err := api.s.tokenManager.DiscoverToken(ctx, chainID, address)
	return token, err
}

func (api *API) GetVisibleTokens(chainIDs []uint64) (map[uint64][]*token.Token, error) {
	log.Debug("call to get visible tokens")
	rst, err := api.s.tokenManager.GetVisible(chainIDs)
	log.Debug("result from database for visible tokens", "len", len(rst))
	return rst, err
}

func (api *API) ToggleVisibleToken(ctx context.Context, chainID uint64, address common.Address) (bool, error) {
	log.Debug("call to toggle visible tokens")
	err := api.s.tokenManager.Toggle(chainID, address)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (api *API) AddCustomToken(ctx context.Context, token token.Token) error {
	log.Debug("call to create or edit custom token")
	if token.ChainID == 0 {
		token.ChainID = api.s.rpcClient.UpstreamChainID
	}
	err := api.s.tokenManager.UpsertCustom(token)
	log.Debug("result from database for create or edit custom token", "err", err)
	return err
}

func (api *API) DeleteCustomToken(ctx context.Context, address common.Address) error {
	log.Debug("call to remove custom token")
	err := api.s.tokenManager.DeleteCustom(api.s.rpcClient.UpstreamChainID, address)
	log.Debug("result from database for remove custom token", "err", err)
	return err
}

func (api *API) DeleteCustomTokenByChainID(ctx context.Context, chainID uint64, address common.Address) error {
	log.Debug("call to remove custom token")
	err := api.s.tokenManager.DeleteCustom(chainID, address)
	log.Debug("result from database for remove custom token", "err", err)
	return err
}

func (api *API) GetSavedAddresses(ctx context.Context) ([]SavedAddress, error) {
	log.Debug("call to get saved addresses")
	rst, err := api.s.savedAddressesManager.GetSavedAddresses()
	log.Debug("result from database for saved addresses", "len", len(rst))
	return rst, err
}

func (api *API) AddSavedAddress(ctx context.Context, sa SavedAddress) error {
	log.Debug("call to create or edit saved address")
	_, err := api.s.savedAddressesManager.UpdateMetadataAndUpsertSavedAddress(sa)
	log.Debug("result from database for create or edit saved address", "err", err)
	return err
}

func (api *API) DeleteSavedAddress(ctx context.Context, address common.Address, ens string, isTest bool) error {
	log.Debug("call to remove saved address")
	_, err := api.s.savedAddressesManager.DeleteSavedAddress(address, ens, isTest, uint64(time.Now().Unix()))
	log.Debug("result from database for remove saved address", "err", err)
	return err
}

func (api *API) GetPendingTransactions(ctx context.Context) ([]*transactions.PendingTransaction, error) {
	log.Debug("call to get pending transactions")
	rst, err := api.s.pendingTxManager.GetAllPending([]uint64{api.s.rpcClient.UpstreamChainID})
	log.Debug("result from database for pending transactions", "len", len(rst))
	return rst, err
}

func (api *API) GetPendingTransactionsByChainIDs(ctx context.Context, chainIDs []uint64) ([]*transactions.PendingTransaction, error) {
	log.Debug("call to get pending transactions")
	rst, err := api.s.pendingTxManager.GetAllPending(chainIDs)
	log.Debug("result from database for pending transactions", "len", len(rst))
	return rst, err
}

func (api *API) GetPendingTransactionsForIdentities(ctx context.Context, identities []transfer.TransactionIdentity) (
	result []*transactions.PendingTransaction, err error) {

	log.Debug("call to GetPendingTransactionsForIdentities")

	result = make([]*transactions.PendingTransaction, 0, len(identities))
	var pt *transactions.PendingTransaction
	for _, identity := range identities {
		pt, err = api.s.pendingTxManager.GetPendingEntry(uint64(identity.ChainID), identity.Hash)
		result = append(result, pt)
	}

	log.Debug("result from GetPendingTransactionsForIdentities", "len", len(result))
	return
}

func (api *API) GetPendingOutboundTransactionsByAddress(ctx context.Context, address common.Address) (
	[]*transactions.PendingTransaction, error) {

	log.Debug("call to get pending outbound transactions by address")
	rst, err := api.s.pendingTxManager.GetPendingByAddress([]uint64{api.s.rpcClient.UpstreamChainID}, address)
	log.Debug("result from database for pending transactions by address", "len", len(rst))
	return rst, err
}

func (api *API) GetPendingOutboundTransactionsByAddressAndChainID(ctx context.Context, chainIDs []uint64,
	address common.Address) ([]*transactions.PendingTransaction, error) {

	log.Debug("call to get pending outbound transactions by address")
	rst, err := api.s.pendingTxManager.GetPendingByAddress(chainIDs, address)
	log.Debug("result from database for pending transactions by address", "len", len(rst))
	return rst, err
}

func (api *API) WatchTransaction(ctx context.Context, transactionHash common.Hash) error {
	chainClient, err := api.s.rpcClient.EthClient(api.s.rpcClient.UpstreamChainID)
	if err != nil {
		return err
	}
	return api.s.pendingTxManager.Watch(ctx, transactionHash, chainClient)
}

func (api *API) WatchTransactionByChainID(ctx context.Context, chainID uint64, transactionHash common.Hash) error {
	chainClient, err := api.s.rpcClient.EthClient(chainID)
	if err != nil {
		return err
	}
	return api.s.pendingTxManager.Watch(ctx, transactionHash, chainClient)
}

func (api *API) GetCryptoOnRamps(ctx context.Context) ([]CryptoOnRamp, error) {
	return api.s.cryptoOnRampManager.Get()
}

func (api *API) GetOpenseaCollectionsByOwner(ctx context.Context, chainID uint64, owner common.Address) ([]opensea.OwnedCollection, error) {
	log.Debug("call to get opensea collections")
	return api.s.collectiblesManager.FetchAllCollectionsByOwner(chainID, owner)
}

// Kept for compatibility with mobile app
func (api *API) GetOpenseaAssetsByOwnerAndCollection(ctx context.Context, chainID uint64, owner common.Address, collectionSlug string, limit int) ([]opensea.Asset, error) {
	container, err := api.GetOpenseaAssetsByOwnerAndCollectionWithCursor(ctx, chainID, owner, collectionSlug, "", limit)
	if err != nil {
		return nil, err
	}
	return container.Assets, nil
}

func (api *API) GetOpenseaAssetsByOwnerAndCollectionWithCursor(ctx context.Context, chainID uint64, owner common.Address, collectionSlug string, cursor string, limit int) (*opensea.AssetContainer, error) {
	log.Debug("call to get opensea assets")
	return api.s.collectiblesManager.FetchAllAssetsByOwnerAndCollection(chainID, owner, collectionSlug, cursor, limit)
}

func (api *API) GetOpenseaAssetsByOwnerWithCursor(ctx context.Context, chainID uint64, owner common.Address, cursor string, limit int) (*opensea.AssetContainer, error) {
	log.Debug("call to FetchAllAssetsByOwner")
	return api.s.collectiblesManager.FetchAllAssetsByOwner(chainID, owner, cursor, limit)
}

func (api *API) GetOpenseaAssetsByOwnerAndContractAddressWithCursor(ctx context.Context, chainID uint64, owner common.Address, contractAddresses []common.Address, cursor string, limit int) (*opensea.AssetContainer, error) {
	log.Debug("call to GetOpenseaAssetsByOwnerAndContractAddressWithCursor")
	return api.s.collectiblesManager.FetchAllAssetsByOwnerAndContractAddress(chainID, owner, contractAddresses, cursor, limit)
}

func (api *API) GetOpenseaAssetsByNFTUniqueID(ctx context.Context, chainID uint64, uniqueIDs []thirdparty.NFTUniqueID, limit int) (*opensea.AssetContainer, error) {
	log.Debug("call to GetOpenseaAssetsByNFTUniqueID")
	return api.s.collectiblesManager.FetchAssetsByNFTUniqueID(chainID, uniqueIDs, limit)
}

func (api *API) GetCollectibleOwnersByContractAddress(chainID uint64, contractAddress common.Address) (*thirdparty.NFTContractOwnership, error) {
	log.Debug("call to GetCollectibleOwnersByContractAddress")
	return api.s.collectiblesManager.FetchNFTOwnersByContractAddress(chainID, contractAddress)
}

func (api *API) FetchBalancesByOwnerAndContractAddress(chainID uint64, ownerAddress common.Address, contractAddresses []common.Address) (thirdparty.TokenBalancesPerContractAddress, error) {
	log.Debug("call to FetchBalancesByOwnerAndContractAddress")
	return api.s.collectiblesManager.FetchBalancesByOwnerAndContractAddress(chainID, ownerAddress, contractAddresses)
}

func (api *API) AddEthereumChain(ctx context.Context, network params.Network) error {
	log.Debug("call to AddEthereumChain")
	return api.s.rpcClient.NetworkManager.Upsert(&network)
}

func (api *API) DeleteEthereumChain(ctx context.Context, chainID uint64) error {
	log.Debug("call to DeleteEthereumChain")
	return api.s.rpcClient.NetworkManager.Delete(chainID)
}

func (api *API) GetEthereumChains(ctx context.Context) ([]*network.CombinedNetwork, error) {
	log.Debug("call to GetEthereumChains")
	return api.s.rpcClient.NetworkManager.GetCombinedNetworks()
}

func (api *API) FetchPrices(ctx context.Context, symbols []string, currencies []string) (map[string]map[string]float64, error) {
	log.Debug("call to FetchPrices")
	return api.s.marketManager.FetchPrices(symbols, currencies)
}

func (api *API) FetchMarketValues(ctx context.Context, symbols []string, currency string) (map[string]thirdparty.TokenMarketValues, error) {
	log.Debug("call to FetchMarketValues")
	return api.s.marketManager.FetchTokenMarketValues(symbols, currency)
}

func (api *API) GetHourlyMarketValues(ctx context.Context, symbol string, currency string, limit int, aggregate int) ([]thirdparty.HistoricalPrice, error) {
	log.Debug("call to GetHourlyMarketValues")
	return api.s.marketManager.FetchHistoricalHourlyPrices(symbol, currency, limit, aggregate)
}

func (api *API) GetDailyMarketValues(ctx context.Context, symbol string, currency string, limit int, allData bool, aggregate int) ([]thirdparty.HistoricalPrice, error) {
	log.Debug("call to GetDailyMarketValues")
	return api.s.marketManager.FetchHistoricalDailyPrices(symbol, currency, limit, allData, aggregate)
}

func (api *API) FetchTokenDetails(ctx context.Context, symbols []string) (map[string]thirdparty.TokenDetails, error) {
	log.Debug("call to FetchTokenDetails")
	return api.s.marketManager.FetchTokenDetails(symbols)
}

func (api *API) GetSuggestedFees(ctx context.Context, chainID uint64) (*SuggestedFees, error) {
	log.Debug("call to GetSuggestedFees")
	return api.s.feesManager.suggestedFees(ctx, chainID)
}

func (api *API) GetTransactionEstimatedTime(ctx context.Context, chainID uint64, maxFeePerGas *big.Float) (TransactionEstimation, error) {
	log.Debug("call to getTransactionEstimatedTime")
	return api.s.feesManager.transactionEstimatedTime(ctx, chainID, maxFeePerGas), nil
}

func (api *API) GetSuggestedRoutes(
	ctx context.Context,
	sendType SendType,
	account common.Address,
	amountIn *hexutil.Big,
	tokenSymbol string,
	disabledFromChainIDs,
	disabledToChaindIDs,
	preferedChainIDs []uint64,
	gasFeeMode GasFeeMode,
	fromLockedAmount map[uint64]*hexutil.Big,
) (*SuggestedRoutes, error) {
	log.Debug("call to GetSuggestedRoutes")
	return api.router.suggestedRoutes(ctx, sendType, account, amountIn.ToInt(), tokenSymbol, disabledFromChainIDs, disabledToChaindIDs, preferedChainIDs, gasFeeMode, fromLockedAmount)
}

// Generates addresses for the provided paths, response doesn't include `HasActivity` value (if you need it check `GetAddressDetails` function)
func (api *API) GetDerivedAddresses(ctx context.Context, password string, derivedFrom string, paths []string) ([]*DerivedAddress, error) {
	info, err := api.s.gethManager.AccountsGenerator().LoadAccount(derivedFrom, password)
	if err != nil {
		return nil, err
	}

	return api.getDerivedAddresses(info.ID, paths)
}

// Generates addresses for the provided paths derived from the provided mnemonic, response doesn't include `HasActivity` value (if you need it check `GetAddressDetails` function)
func (api *API) GetDerivedAddressesForMnemonic(ctx context.Context, mnemonic string, paths []string) ([]*DerivedAddress, error) {
	mnemonicNoExtraSpaces := strings.Join(strings.Fields(mnemonic), " ")

	info, err := api.s.gethManager.AccountsGenerator().ImportMnemonic(mnemonicNoExtraSpaces, "")
	if err != nil {
		return nil, err
	}

	return api.getDerivedAddresses(info.ID, paths)
}

// Generates addresses for the provided paths, response doesn't include `HasActivity` value (if you need it check `GetAddressDetails` function)
func (api *API) getDerivedAddresses(id string, paths []string) ([]*DerivedAddress, error) {
	addedAccounts, err := api.s.accountsDB.GetAccounts()
	if err != nil {
		return nil, err
	}

	info, err := api.s.gethManager.AccountsGenerator().DeriveAddresses(id, paths)
	if err != nil {
		return nil, err
	}

	derivedAddresses := make([]*DerivedAddress, 0)
	for accPath, acc := range info {

		derivedAddress := &DerivedAddress{
			Address:   common.HexToAddress(acc.Address),
			PublicKey: types.Hex2Bytes(acc.PublicKey),
			Path:      accPath,
		}

		for _, account := range addedAccounts {
			if types.Address(derivedAddress.Address) == account.Address {
				derivedAddress.AlreadyCreated = true
				break
			}
		}

		derivedAddresses = append(derivedAddresses, derivedAddress)
	}

	return derivedAddresses, nil
}

func (api *API) AddressExists(ctx context.Context, address types.Address) (bool, error) {
	return api.s.accountsDB.AddressExists(address)
}

// Returns details for the passed address (response doesn't include derivation path)
func (api *API) GetAddressDetails(ctx context.Context, chainID uint64, address string) (*DerivedAddress, error) {
	commonAddr := common.HexToAddress(address)
	addressExists, err := api.s.accountsDB.AddressExists(types.Address(commonAddr))
	if err != nil {
		return nil, err
	}

	chainClient, err := api.s.rpcClient.EthClient(chainID)
	if err != nil {
		return nil, err
	}

	balance, err := api.s.tokenManager.GetChainBalance(ctx, chainClient, commonAddr)
	if err != nil {
		return nil, err
	}

	return &DerivedAddress{
		Address:        commonAddr,
		Path:           "",
		AlreadyCreated: addressExists,
		HasActivity:    balance.Cmp(big.NewInt(0)) != 0,
	}, nil
}

func (api *API) CreateMultiTransaction(ctx context.Context, multiTransactionCommand *transfer.MultiTransactionCommand, data []*bridge.TransactionBridge, password string) (*transfer.MultiTransactionCommandResult, error) {
	log.Debug("[WalletAPI:: CreateMultiTransaction] create multi transaction")
	return api.s.transactionManager.CreateMultiTransactionFromCommand(ctx, multiTransactionCommand, data, api.router.bridges, password)
}

func (api *API) GetMultiTransactions(ctx context.Context, transactionIDs []transfer.MultiTransactionIDType) ([]*transfer.MultiTransaction, error) {
	log.Debug("wallet.api.GetMultiTransactions", "IDs.len", len(transactionIDs))
	return api.s.transactionManager.GetMultiTransactions(ctx, transactionIDs)
}

func (api *API) GetCachedCurrencyFormats() (currency.FormatPerSymbol, error) {
	log.Debug("call to GetCachedCurrencyFormats")
	return api.s.currency.GetCachedCurrencyFormats()
}

func (api *API) FetchAllCurrencyFormats() (currency.FormatPerSymbol, error) {
	log.Debug("call to FetchAllCurrencyFormats")
	return api.s.currency.FetchAllCurrencyFormats()
}

func (api *API) FilterActivityAsync(ctx context.Context, addresses []common.Address, chainIDs []wcommon.ChainID, filter activity.Filter, offset int, limit int) error {
	log.Debug("wallet.api.FilterActivityAsync", "addr.count", len(addresses), "chainIDs.count", len(chainIDs), "offset", offset, "limit", limit)

	api.s.activity.FilterActivityAsync(ctx, addresses, chainIDs, filter, offset, limit)
	return nil
}

func (api *API) GetRecipientsAsync(ctx context.Context, offset int, limit int) (ignored bool, err error) {
	log.Debug("wallet.api.GetRecipientsAsync", "offset", offset, "limit", limit)

	ignored = api.s.activity.GetRecipientsAsync(ctx, offset, limit)
	return ignored, err
}

func (api *API) GetOldestActivityTimestampAsync(ctx context.Context, addresses []common.Address) error {
	log.Debug("wallet.api.GetOldestActivityTimestamp", "addresses.len", len(addresses))

	api.s.activity.GetOldestTimestampAsync(ctx, addresses)
	return nil
}

func (api *API) FetchChainIDForURL(ctx context.Context, rpcURL string) (*big.Int, error) {
	log.Debug("wallet.api.VerifyURL", rpcURL)

	rpcClient, err := gethrpc.Dial(rpcURL)
	if err != nil {
		return nil, fmt.Errorf("dial upstream server: %s", err)
	}
	client := ethclient.NewClient(rpcClient)
	return client.ChainID(ctx)
}
