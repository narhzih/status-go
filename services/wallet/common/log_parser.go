// Moved here because transactions package depends on accounts package which
// depends on appdatabase where this functionality is needed
package common

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
)

// Type type of transaction
type Type string

// Log Event type
type EventType string

const (
	// Transaction types
	EthTransfer        Type = "eth"
	Erc20Transfer      Type = "erc20"
	Erc721Transfer     Type = "erc721"
	UniswapV2Swap      Type = "uniswapV2Swap"
	UniswapV3Swap      Type = "uniswapV3Swap"
	HopBridgeFrom      Type = "HopBridgeFrom"
	HopBridgeTo        Type = "HopBridgeTo"
	unknownTransaction Type = "unknown"

	// Event types
	WETHDepositEventType                      EventType = "wethDepositEvent"
	WETHWithdrawalEventType                   EventType = "wethWithdrawalEvent"
	Erc20TransferEventType                    EventType = "erc20Event"
	Erc721TransferEventType                   EventType = "erc721Event"
	UniswapV2SwapEventType                    EventType = "uniswapV2SwapEvent"
	UniswapV3SwapEventType                    EventType = "uniswapV3SwapEvent"
	HopBridgeTransferSentToL2EventType        EventType = "hopBridgeTransferSentToL2Event"
	HopBridgeTransferFromL1CompletedEventType EventType = "hopBridgeTransferFromL1CompletedEvent"
	HopBridgeWithdrawalBondedEventType        EventType = "hopBridgeWithdrawalBondedEvent"
	HopBridgeTransferSentEventType            EventType = "hopBridgeTransferSentEvent"
	UnknownEventType                          EventType = "unknownEvent"

	// Deposit (index_topic_1 address dst, uint256 wad)
	wethDepositEventSignature = "Deposit(address,uint256)"
	// Withdrawal (index_topic_1 address src, uint256 wad)
	wethWithdrawalEventSignature = "Withdrawal(address,uint256)"

	// Transfer (index_topic_1 address from, index_topic_2 address to, uint256 value)
	// Transfer (index_topic_1 address from, index_topic_2 address to, index_topic_3 uint256 tokenId)
	Erc20_721TransferEventSignature = "Transfer(address,address,uint256)"

	erc20TransferEventIndexedParameters  = 3 // signature, from, to
	erc721TransferEventIndexedParameters = 4 // signature, from, to, tokenId

	// Swap (index_topic_1 address sender, uint256 amount0In, uint256 amount1In, uint256 amount0Out, uint256 amount1Out, index_topic_2 address to)
	uniswapV2SwapEventSignature = "Swap(address,uint256,uint256,uint256,uint256,address)" // also used by SushiSwap
	// Swap (index_topic_1 address sender, index_topic_2 address recipient, int256 amount0, int256 amount1, uint160 sqrtPriceX96, uint128 liquidity, int24 tick)
	uniswapV3SwapEventSignature = "Swap(address,address,int256,int256,uint160,uint128,int24)"

	// TransferSentToL2 (index_topic_1 uint256 chainId, index_topic_2 address recipient, uint256 amount, uint256 amountOutMin, uint256 deadline, index_topic_3 address relayer, uint256 relayerFee)
	hopBridgeTransferSentToL2EventSignature = "TransferSentToL2(uint256,address,uint256,uint256,uint256,address,uint256)"
	// TransferFromL1Completed (index_topic_1 address recipient, uint256 amount, uint256 amountOutMin, uint256 deadline, index_topic_2 address relayer, uint256 relayerFee)
	HopBridgeTransferFromL1CompletedEventSignature = "TransferFromL1Completed(address,uint256,uint256,uint256,address,uint256)"
	// WithdrawalBonded (index_topic_1 bytes32 transferID, uint256 amount)
	hopBridgeWithdrawalBondedEventSignature = "WithdrawalBonded(bytes32,uint256)"
	// TransferSent (index_topic_1 bytes32 transferID, index_topic_2 uint256 chainId, index_topic_3 address recipient, uint256 amount, bytes32 transferNonce, uint256 bonderFee, uint256 index, uint256 amountOutMin, uint256 deadline)
	hopBridgeTransferSentEventSignature = "TransferSent(bytes32,uint256,address,uint256,bytes32,uint256,uint256,uint256,uint256)"
)

var (
	// MaxUint256 is the maximum value that can be represented by a uint256.
	MaxUint256 = new(big.Int).Sub(new(big.Int).Lsh(common.Big1, 256), common.Big1)
)

// Detect event type for a cetain item from the Events Log
func GetEventType(log *types.Log) EventType {
	wethDepositEventSignatureHash := GetEventSignatureHash(wethDepositEventSignature)
	wethWithdrawalEventSignatureHash := GetEventSignatureHash(wethWithdrawalEventSignature)
	erc20_721TransferEventSignatureHash := GetEventSignatureHash(Erc20_721TransferEventSignature)
	uniswapV2SwapEventSignatureHash := GetEventSignatureHash(uniswapV2SwapEventSignature)
	uniswapV3SwapEventSignatureHash := GetEventSignatureHash(uniswapV3SwapEventSignature)
	hopBridgeTransferSentToL2EventSignatureHash := GetEventSignatureHash(hopBridgeTransferSentToL2EventSignature)
	hopBridgeTransferFromL1CompletedEventSignatureHash := GetEventSignatureHash(HopBridgeTransferFromL1CompletedEventSignature)
	hopBridgeWithdrawalBondedEventSignatureHash := GetEventSignatureHash(hopBridgeWithdrawalBondedEventSignature)
	hopBridgeTransferSentEventSignatureHash := GetEventSignatureHash(hopBridgeTransferSentEventSignature)

	if len(log.Topics) > 0 {
		switch log.Topics[0] {
		case wethDepositEventSignatureHash:
			return WETHDepositEventType
		case wethWithdrawalEventSignatureHash:
			return WETHWithdrawalEventType
		case erc20_721TransferEventSignatureHash:
			switch len(log.Topics) {
			case erc20TransferEventIndexedParameters:
				return Erc20TransferEventType
			case erc721TransferEventIndexedParameters:
				return Erc721TransferEventType
			}
		case uniswapV2SwapEventSignatureHash:
			return UniswapV2SwapEventType
		case uniswapV3SwapEventSignatureHash:
			return UniswapV3SwapEventType
		case hopBridgeTransferSentToL2EventSignatureHash:
			return HopBridgeTransferSentToL2EventType
		case hopBridgeTransferFromL1CompletedEventSignatureHash:
			return HopBridgeTransferFromL1CompletedEventType
		case hopBridgeWithdrawalBondedEventSignatureHash:
			return HopBridgeWithdrawalBondedEventType
		case hopBridgeTransferSentEventSignatureHash:
			return HopBridgeTransferSentEventType
		}
	}

	return UnknownEventType
}

func EventTypeToSubtransactionType(eventType EventType) Type {
	switch eventType {
	case Erc20TransferEventType:
		return Erc20Transfer
	case Erc721TransferEventType:
		return Erc721Transfer
	case UniswapV2SwapEventType:
		return UniswapV2Swap
	case UniswapV3SwapEventType:
		return UniswapV3Swap
	case HopBridgeTransferSentToL2EventType, HopBridgeTransferSentEventType:
		return HopBridgeFrom
	case HopBridgeTransferFromL1CompletedEventType, HopBridgeWithdrawalBondedEventType:
		return HopBridgeTo
	}

	return unknownTransaction
}

func GetFirstEvent(logs []*types.Log) (EventType, *types.Log) {
	for _, log := range logs {
		eventType := GetEventType(log)
		if eventType != UnknownEventType {
			return eventType, log
		}
	}

	return UnknownEventType, nil
}

func IsTokenTransfer(logs []*types.Log) bool {
	eventType, _ := GetFirstEvent(logs)
	return eventType == Erc20TransferEventType
}

func ParseWETHDepositLog(ethlog *types.Log) (src common.Address, amount *big.Int) {
	amount = new(big.Int)

	if len(ethlog.Topics) < 2 {
		log.Warn("not enough topics for WETH deposit", "topics", ethlog.Topics)
		return
	}

	if len(ethlog.Topics[1]) != 32 {
		log.Warn("second topic is not padded to 32 byte address", "topic", ethlog.Topics[1])
		return
	}
	copy(src[:], ethlog.Topics[1][12:])

	if len(ethlog.Data) != 32 {
		log.Warn("data is not padded to 32 byte big int", "data", ethlog.Data)
		return
	}
	amount.SetBytes(ethlog.Data)

	return
}

func ParseWETHWithdrawLog(ethlog *types.Log) (dst common.Address, amount *big.Int) {
	amount = new(big.Int)

	if len(ethlog.Topics) < 2 {
		log.Warn("not enough topics for WETH withdraw", "topics", ethlog.Topics)
		return
	}

	if len(ethlog.Topics[1]) != 32 {
		log.Warn("second topic is not padded to 32 byte address", "topic", ethlog.Topics[1])
		return
	}
	copy(dst[:], ethlog.Topics[1][12:])

	if len(ethlog.Data) != 32 {
		log.Warn("data is not padded to 32 byte big int", "data", ethlog.Data)
		return
	}
	amount.SetBytes(ethlog.Data)

	return
}

func ParseErc20TransferLog(ethlog *types.Log) (from, to common.Address, amount *big.Int) {
	amount = new(big.Int)
	if len(ethlog.Topics) < 3 {
		log.Warn("not enough topics for erc20 transfer", "topics", ethlog.Topics)
		return
	}
	if len(ethlog.Topics[1]) != 32 {
		log.Warn("second topic is not padded to 32 byte address", "topic", ethlog.Topics[1])
		return
	}
	if len(ethlog.Topics[2]) != 32 {
		log.Warn("third topic is not padded to 32 byte address", "topic", ethlog.Topics[2])
		return
	}
	copy(from[:], ethlog.Topics[1][12:])
	copy(to[:], ethlog.Topics[2][12:])
	if len(ethlog.Data) != 32 {
		log.Warn("data is not padded to 32 byts big int", "data", ethlog.Data)
		return
	}
	amount.SetBytes(ethlog.Data)

	return
}

func ParseErc721TransferLog(ethlog *types.Log) (from, to common.Address, tokenID *big.Int) {
	tokenID = new(big.Int)
	if len(ethlog.Topics) < 4 {
		log.Warn("not enough topics for erc721 transfer", "topics", ethlog.Topics)
		return
	}
	if len(ethlog.Topics[1]) != 32 {
		log.Warn("second topic is not padded to 32 byte address", "topic", ethlog.Topics[1])
		return
	}
	if len(ethlog.Topics[2]) != 32 {
		log.Warn("third topic is not padded to 32 byte address", "topic", ethlog.Topics[2])
		return
	}
	if len(ethlog.Topics[3]) != 32 {
		log.Warn("fourth topic is not 32 byte tokenId", "topic", ethlog.Topics[3])
		return
	}
	copy(from[:], ethlog.Topics[1][12:])
	copy(to[:], ethlog.Topics[2][12:])
	tokenID.SetBytes(ethlog.Topics[3][:])

	return
}

func ParseUniswapV2Log(ethlog *types.Log) (pairAddress common.Address, from common.Address, to common.Address, amount0In *big.Int, amount1In *big.Int, amount0Out *big.Int, amount1Out *big.Int, err error) {
	amount0In = new(big.Int)
	amount1In = new(big.Int)
	amount0Out = new(big.Int)
	amount1Out = new(big.Int)

	if len(ethlog.Topics) < 3 {
		err = fmt.Errorf("not enough topics for uniswapV2 swap %s, %v", "topics", ethlog.Topics)
		return
	}

	pairAddress = ethlog.Address

	if len(ethlog.Topics[1]) != 32 {
		err = fmt.Errorf("second topic is not padded to 32 byte address %s, %v", "topic", ethlog.Topics[1])
		return
	}
	if len(ethlog.Topics[2]) != 32 {
		err = fmt.Errorf("third topic is not padded to 32 byte address %s, %v", "topic", ethlog.Topics[2])
		return
	}
	copy(from[:], ethlog.Topics[1][12:])
	copy(to[:], ethlog.Topics[2][12:])
	if len(ethlog.Data) != 32*4 {
		err = fmt.Errorf("data is not padded to 4 * 32 bytes big int %s, %v", "data", ethlog.Data)
		return
	}
	amount0In.SetBytes(ethlog.Data[0:32])
	amount1In.SetBytes(ethlog.Data[32:64])
	amount0Out.SetBytes(ethlog.Data[64:96])
	amount1Out.SetBytes(ethlog.Data[96:128])

	return
}

func readInt256(b []byte) *big.Int {
	// big.SetBytes can't tell if a number is negative or positive in itself.
	// On EVM, if the returned number > max int256, it is negative.
	// A number is > max int256 if the bit at position 255 is set.
	ret := new(big.Int).SetBytes(b)
	if ret.Bit(255) == 1 {
		ret.Add(MaxUint256, new(big.Int).Neg(ret))
		ret.Add(ret, common.Big1)
		ret.Neg(ret)
	}
	return ret
}

func ParseUniswapV3Log(ethlog *types.Log) (poolAddress common.Address, sender common.Address, recipient common.Address, amount0 *big.Int, amount1 *big.Int, err error) {
	amount0 = new(big.Int)
	amount1 = new(big.Int)

	if len(ethlog.Topics) < 3 {
		err = fmt.Errorf("not enough topics for uniswapV3 swap %s, %v", "topics", ethlog.Topics)
		return
	}

	poolAddress = ethlog.Address

	if len(ethlog.Topics[1]) != 32 {
		err = fmt.Errorf("second topic is not padded to 32 byte address %s, %v", "topic", ethlog.Topics[1])
		return
	}
	if len(ethlog.Topics[2]) != 32 {
		err = fmt.Errorf("third topic is not padded to 32 byte address %s, %v", "topic", ethlog.Topics[2])
		return
	}
	copy(sender[:], ethlog.Topics[1][12:])
	copy(recipient[:], ethlog.Topics[2][12:])
	if len(ethlog.Data) != 32*5 {
		err = fmt.Errorf("data is not padded to 5 * 32 bytes big int %s, %v", "data", ethlog.Data)
		return
	}
	amount0 = readInt256(ethlog.Data[0:32])
	amount1 = readInt256(ethlog.Data[32:64])

	return
}

func ParseHopBridgeTransferSentToL2Log(ethlog *types.Log) (chainID uint64, recipient common.Address, relayer common.Address, amount *big.Int, err error) {
	chainIDInt := new(big.Int)
	amount = new(big.Int)

	if len(ethlog.Topics) < 4 {
		err = fmt.Errorf("not enough topics for HopBridgeTransferSentToL2 event %s, %v", "topics", ethlog.Topics)
		return
	}

	if len(ethlog.Topics[1]) != 32 {
		err = fmt.Errorf("second topic is not padded to 32 byte address %s, %v", "topic", ethlog.Topics[1])
		return
	}
	chainIDInt.SetBytes(ethlog.Topics[1][:])
	chainID = chainIDInt.Uint64()

	if len(ethlog.Topics[2]) != 32 {
		err = fmt.Errorf("third topic is not padded to 32 byte address %s, %v", "topic", ethlog.Topics[2])
		return
	}
	copy(recipient[:], ethlog.Topics[2][12:])

	if len(ethlog.Topics[3]) != 32 {
		err = fmt.Errorf("fourth topic is not padded to 32 byte address %s, %v", "topic", ethlog.Topics[3])
		return
	}
	copy(relayer[:], ethlog.Topics[3][12:])

	if len(ethlog.Data) != 32*4 {
		err = fmt.Errorf("data is not padded to 4 * 32 bytes big int %s, %v", "data", ethlog.Data)
		return
	}

	amount.SetBytes(ethlog.Data[0:32])

	return
}

func ParseHopBridgeTransferFromL1CompletedLog(ethlog *types.Log) (recipient common.Address, relayer common.Address, amount *big.Int, err error) {
	amount = new(big.Int)

	if len(ethlog.Topics) < 3 {
		err = fmt.Errorf("not enough topics for HopBridgeTransferFromL1Completed event %s, %v", "topics", ethlog.Topics)
		return
	}

	if len(ethlog.Topics[1]) != 32 {
		err = fmt.Errorf("second topic is not padded to 32 byte address %s, %v", "topic", ethlog.Topics[1])
		return
	}
	copy(recipient[:], ethlog.Topics[1][12:])

	if len(ethlog.Topics[2]) != 32 {
		err = fmt.Errorf("third topic is not padded to 32 byte address %s, %v", "topic", ethlog.Topics[2])
		return
	}
	copy(relayer[:], ethlog.Topics[2][12:])

	if len(ethlog.Data) != 32*4 {
		err = fmt.Errorf("data is not padded to 4 * 32 bytes big int %s, %v", "data", ethlog.Data)
		return
	}

	amount.SetBytes(ethlog.Data[0:32])

	return
}

func ParseHopWithdrawalBondedLog(ethlog *types.Log) (transferID *big.Int, amount *big.Int, err error) {
	transferID = new(big.Int)
	amount = new(big.Int)

	if len(ethlog.Topics) < 2 {
		err = fmt.Errorf("not enough topics for HopWithdrawalBonded event %s, %v", "topics", ethlog.Topics)
		return
	}

	if len(ethlog.Topics[1]) != 32 {
		err = fmt.Errorf("second topic is not padded to 32 byte address %s, %v", "topic", ethlog.Topics[1])
		return
	}
	transferID.SetBytes(ethlog.Topics[1][:])

	if len(ethlog.Data) != 32*1 {
		err = fmt.Errorf("data is not padded to 1 * 32 bytes big int %s, %v", "data", ethlog.Data)
		return
	}

	amount.SetBytes(ethlog.Data[0:32])

	return
}

func ParseHopBridgeTransferSentLog(ethlog *types.Log) (transferID *big.Int, chainID uint64, recipient common.Address, amount *big.Int, transferNonce *big.Int, bonderFee *big.Int, index *big.Int, amountOutMin *big.Int, deadline *big.Int, err error) {
	transferID = new(big.Int)
	chainIDInt := new(big.Int)
	amount = new(big.Int)
	transferNonce = new(big.Int)
	bonderFee = new(big.Int)
	index = new(big.Int)
	amountOutMin = new(big.Int)
	deadline = new(big.Int)

	if len(ethlog.Topics) < 4 {
		err = fmt.Errorf("not enough topics for HopBridgeTransferSent event %s, %v", "topics", ethlog.Topics)
		return
	}

	if len(ethlog.Topics[1]) != 32 {
		err = fmt.Errorf("second topic is not padded to 32 byte big int %s, %v", "topic", ethlog.Topics[1])
		return
	}
	transferID.SetBytes(ethlog.Topics[1][:])

	if len(ethlog.Topics[2]) != 32 {
		err = fmt.Errorf("third topic is not padded to 32 byte big int %s, %v", "topic", ethlog.Topics[2])
		return
	}
	chainIDInt.SetBytes(ethlog.Topics[2][:])
	chainID = chainIDInt.Uint64()

	if len(ethlog.Topics[3]) != 32 {
		err = fmt.Errorf("fourth topic is not padded to 32 byte address %s, %v", "topic", ethlog.Topics[3])
		return
	}
	copy(recipient[:], ethlog.Topics[2][12:])

	if len(ethlog.Data) != 32*6 {
		err = fmt.Errorf("data is not padded to 6 * 32 bytes big int %s, %v", "data", ethlog.Data)
		return
	}

	amount.SetBytes(ethlog.Data[0:32])
	transferNonce.SetBytes(ethlog.Data[32:64])
	bonderFee.SetBytes(ethlog.Data[64:96])
	index.SetBytes(ethlog.Data[96:128])
	amountOutMin.SetBytes(ethlog.Data[128:160])
	deadline.SetBytes(ethlog.Data[160:192])

	return
}

func GetEventSignatureHash(signature string) common.Hash {
	return crypto.Keccak256Hash([]byte(signature))
}

func ExtractTokenIdentity(dbEntryType Type, log *types.Log, tx *types.Transaction) (correctType Type, tokenAddress *common.Address, txTokenID *big.Int, txValue *big.Int, txFrom *common.Address, txTo *common.Address) {
	// erc721 transfers share signature with erc20 ones, so they both used to be categorized as erc20
	// by the Downloader. We fix this here since they might be mis-categorized in the db.
	if dbEntryType == Erc20Transfer {
		eventType := GetEventType(log)
		correctType = EventTypeToSubtransactionType(eventType)
	} else {
		correctType = dbEntryType
	}

	switch correctType {
	case EthTransfer:
		if tx != nil {
			txValue = new(big.Int).Set(tx.Value())
		}
	case Erc20Transfer:
		tokenAddress = new(common.Address)
		*tokenAddress = log.Address
		from, to, value := ParseErc20TransferLog(log)
		txValue = value
		txFrom = &from
		txTo = &to
	case Erc721Transfer:
		tokenAddress = new(common.Address)
		*tokenAddress = log.Address
		from, to, tokenID := ParseErc721TransferLog(log)
		txTokenID = tokenID
		txFrom = &from
		txTo = &to
	}

	return
}

func TxDataContainsAddress(txType uint8, txData []byte, address common.Address) bool {
	// First 4 bytes are related to the methodID
	const methodIDLen int = 4
	const paramLen int = 32

	var paramOffset int = 0
	switch txType {
	case types.OptimismDepositTxType:
		// Offset for relayMessage data.
		// I actually don't know what the 2x32 + 4 bytes mean, but it seems to be constant in all transactions I've
		// checked. Will update the comment when I find out more about it.
		paramOffset = 5*32 + 2*32 + 4
	}

	// Check if address is contained in any 32-byte parameter
	for paramStart := methodIDLen + paramOffset; paramStart < len(txData); paramStart += paramLen {
		paramEnd := paramStart + paramLen

		if paramEnd > len(txData) {
			break
		}

		// Address bytes should be in the last addressLen positions
		paramBytes := txData[paramStart:paramEnd]
		paramAddress := common.BytesToAddress(paramBytes)
		if address == paramAddress {
			return true
		}
	}
	return false
}
