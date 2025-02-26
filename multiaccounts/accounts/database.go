package accounts

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/status-im/status-go/eth-node/types"
	"github.com/status-im/status-go/multiaccounts/common"
	"github.com/status-im/status-go/multiaccounts/settings"
	notificationssettings "github.com/status-im/status-go/multiaccounts/settings_notifications"
	sociallinkssettings "github.com/status-im/status-go/multiaccounts/settings_social_links"
	"github.com/status-im/status-go/nodecfg"
	"github.com/status-im/status-go/params"
)

const (
	statusChatPath         = "m/43'/60'/1581'/0'/0"
	statusWalletRootPath   = "m/44'/60'/0'/0/"
	zeroAddress            = "0x0000000000000000000000000000000000000000"
	SyncedFromBackup       = "backup"        // means a account is coming from backed up data
	SyncedFromLocalPairing = "local-pairing" // means a account is coming from another device when user is reocovering Status account
)

var (
	errDbPassedParameterIsNil                      = errors.New("accounts: passed parameter is nil")
	errDbTransactionIsNil                          = errors.New("accounts: database transaction is nil")
	ErrDbKeypairNotFound                           = errors.New("accounts: keypair is not found")
	ErrDbAccountNotFound                           = errors.New("accounts: account is not found")
	ErrAccountWrongPosition                        = errors.New("accounts: trying to set wrong position to account")
	ErrNotTheSameNumberOdAccountsToApplyReordering = errors.New("accounts: there is different number of accounts between received sync message and db accounts")
	ErrNotTheSameAccountsToApplyReordering         = errors.New("accounts: there are differences between accounts in received sync message and db accounts")
	ErrMovingAccountToWrongPosition                = errors.New("accounts: trying to move account to a wrong position")
	ErrKeypairDifferentAccountsKeyUID              = errors.New("cannot store keypair with different accounts' key uid than keypair's key uid")
	ErrKeypairWithoutAccounts                      = errors.New("cannot store keypair without accounts")
)

type Keypair struct {
	KeyUID                  string      `json:"key-uid"`
	Name                    string      `json:"name"`
	Type                    KeypairType `json:"type"`
	DerivedFrom             string      `json:"derived-from"`
	LastUsedDerivationIndex uint64      `json:"last-used-derivation-index,omitempty"`
	SyncedFrom              string      `json:"synced-from,omitempty"` // keeps an info which device this keypair is added from can be one of two values defined in constants or device name (custom)
	Clock                   uint64      `json:"clock,omitempty"`
	Accounts                []*Account  `json:"accounts,omitempty"`
	Keycards                []*Keycard  `json:"keycards,omitempty"`
	Removed                 bool        `json:"removed,omitempty"`
}

type Account struct {
	Address   types.Address             `json:"address"`
	KeyUID    string                    `json:"key-uid"`
	Wallet    bool                      `json:"wallet"`
	Chat      bool                      `json:"chat"`
	Type      AccountType               `json:"type,omitempty"`
	Path      string                    `json:"path,omitempty"`
	PublicKey types.HexBytes            `json:"public-key,omitempty"`
	Name      string                    `json:"name"`
	Emoji     string                    `json:"emoji"`
	ColorID   common.CustomizationColor `json:"colorId,omitempty"`
	Hidden    bool                      `json:"hidden"`
	Clock     uint64                    `json:"clock,omitempty"`
	Removed   bool                      `json:"removed,omitempty"`
	Operable  AccountOperable           `json:"operable"` // describes an account's operability (read an explanation at the top of this file)
	CreatedAt int64                     `json:"createdAt"`
	Position  int64                     `json:"position"`
}

type KeypairType string
type AccountType string
type AccountOperable string

func (a KeypairType) String() string {
	return string(a)
}

func (a AccountType) String() string {
	return string(a)
}

func (a AccountOperable) String() string {
	return string(a)
}

const (
	KeypairTypeProfile KeypairType = "profile"
	KeypairTypeKey     KeypairType = "key"
	KeypairTypeSeed    KeypairType = "seed"
)

const (
	AccountTypeGenerated AccountType = "generated"
	AccountTypeKey       AccountType = "key"
	AccountTypeSeed      AccountType = "seed"
	AccountTypeWatch     AccountType = "watch"
)

const (
	AccountNonOperable       AccountOperable = "no"        // an account is non operable it is not a keycard account and there is no keystore file for it and no keystore file for the address it is derived from
	AccountPartiallyOperable AccountOperable = "partially" // an account is partially operable if it is not a keycard account and there is created keystore file for the address it is derived from
	AccountFullyOperable     AccountOperable = "fully"     // an account is fully operable if it is not a keycard account and there is a keystore file for it
)

// IsOwnAccount returns true if this is an account we have the private key for
// NOTE: Wallet flag can't be used as it actually indicates that it's the default
// Wallet
func (a *Account) IsOwnAccount() bool {
	return a.Wallet || a.Type == AccountTypeSeed || a.Type == AccountTypeGenerated || a.Type == AccountTypeKey
}

func (a *Account) MarshalJSON() ([]byte, error) {
	item := struct {
		Address          types.Address             `json:"address"`
		MixedcaseAddress string                    `json:"mixedcase-address"`
		KeyUID           string                    `json:"key-uid"`
		Wallet           bool                      `json:"wallet"`
		Chat             bool                      `json:"chat"`
		Type             AccountType               `json:"type"`
		Path             string                    `json:"path"`
		PublicKey        types.HexBytes            `json:"public-key"`
		Name             string                    `json:"name"`
		Emoji            string                    `json:"emoji"`
		ColorID          common.CustomizationColor `json:"colorId"`
		Hidden           bool                      `json:"hidden"`
		Clock            uint64                    `json:"clock"`
		Removed          bool                      `json:"removed"`
		Operable         AccountOperable           `json:"operable"`
		CreatedAt        int64                     `json:"createdAt"`
		Position         int64                     `json:"position"`
	}{
		Address:          a.Address,
		MixedcaseAddress: a.Address.Hex(),
		KeyUID:           a.KeyUID,
		Wallet:           a.Wallet,
		Chat:             a.Chat,
		Type:             a.Type,
		Path:             a.Path,
		PublicKey:        a.PublicKey,
		Name:             a.Name,
		Emoji:            a.Emoji,
		ColorID:          a.ColorID,
		Hidden:           a.Hidden,
		Clock:            a.Clock,
		Removed:          a.Removed,
		Operable:         a.Operable,
		CreatedAt:        a.CreatedAt,
		Position:         a.Position,
	}

	return json.Marshal(item)
}

func (a *Keypair) MarshalJSON() ([]byte, error) {
	item := struct {
		KeyUID                  string      `json:"key-uid"`
		Name                    string      `json:"name"`
		Type                    KeypairType `json:"type"`
		DerivedFrom             string      `json:"derived-from"`
		LastUsedDerivationIndex uint64      `json:"last-used-derivation-index"`
		SyncedFrom              string      `json:"synced-from"`
		Clock                   uint64      `json:"clock"`
		Accounts                []*Account  `json:"accounts"`
		Keycards                []*Keycard  `json:"keycards"`
		Removed                 bool        `json:"removed"`
	}{
		KeyUID:                  a.KeyUID,
		Name:                    a.Name,
		Type:                    a.Type,
		DerivedFrom:             a.DerivedFrom,
		LastUsedDerivationIndex: a.LastUsedDerivationIndex,
		SyncedFrom:              a.SyncedFrom,
		Clock:                   a.Clock,
		Accounts:                a.Accounts,
		Keycards:                a.Keycards,
		Removed:                 a.Removed,
	}

	return json.Marshal(item)
}

func (a *Keypair) CopyKeypair() *Keypair {
	kp := &Keypair{
		Clock:                   a.Clock,
		KeyUID:                  a.KeyUID,
		Name:                    a.Name,
		Type:                    a.Type,
		DerivedFrom:             a.DerivedFrom,
		LastUsedDerivationIndex: a.LastUsedDerivationIndex,
		SyncedFrom:              a.SyncedFrom,
		Accounts:                make([]*Account, len(a.Accounts)),
		Keycards:                make([]*Keycard, len(a.Keycards)),
		Removed:                 a.Removed,
	}

	for i, acc := range a.Accounts {
		kp.Accounts[i] = &Account{
			Address:   acc.Address,
			KeyUID:    acc.KeyUID,
			Wallet:    acc.Wallet,
			Chat:      acc.Chat,
			Type:      acc.Type,
			Path:      acc.Path,
			PublicKey: acc.PublicKey,
			Name:      acc.Name,
			Emoji:     acc.Emoji,
			ColorID:   acc.ColorID,
			Hidden:    acc.Hidden,
			Clock:     acc.Clock,
			Removed:   acc.Removed,
			Operable:  acc.Operable,
			CreatedAt: acc.CreatedAt,
			Position:  acc.Position,
		}
	}

	for i, kc := range a.Keycards {
		kp.Keycards[i] = &Keycard{
			KeycardUID:        kc.KeycardUID,
			KeycardName:       kc.KeycardName,
			KeycardLocked:     kc.KeycardLocked,
			AccountsAddresses: kc.AccountsAddresses,
			KeyUID:            kc.KeyUID,
		}
	}

	return kp
}

func (a *Keypair) GetChatPublicKey() types.HexBytes {
	for _, acc := range a.Accounts {
		if acc.Chat {
			return acc.PublicKey
		}
	}

	return nil
}

// Database sql wrapper for operations with browser objects.
type Database struct {
	*settings.Database
	*notificationssettings.NotificationsSettings
	*sociallinkssettings.SocialLinksSettings
	db *sql.DB
}

// NewDB returns a new instance of *Database
func NewDB(db *sql.DB) (*Database, error) {
	sDB, err := settings.MakeNewDB(db)
	if err != nil {
		return nil, err
	}
	sn := notificationssettings.NewNotificationsSettings(db)
	ssl := sociallinkssettings.NewSocialLinksSettings(db)

	return &Database{sDB, sn, ssl, db}, nil
}

// DB Gets db sql.DB
func (db *Database) DB() *sql.DB {
	return db.db
}

// Close closes database.
func (db *Database) Close() error {
	return db.db.Close()
}

func GetAccountTypeForKeypairType(kpType KeypairType) AccountType {
	switch kpType {
	case KeypairTypeProfile:
		return AccountTypeGenerated
	case KeypairTypeKey:
		return AccountTypeKey
	case KeypairTypeSeed:
		return AccountTypeSeed
	default:
		return AccountTypeWatch
	}
}

func (db *Database) processRows(rows *sql.Rows) ([]*Keypair, []*Account, error) {
	keypairMap := make(map[string]*Keypair)
	allAccounts := []*Account{}

	var (
		kpKeyUID                  sql.NullString
		kpName                    sql.NullString
		kpType                    sql.NullString
		kpDerivedFrom             sql.NullString
		kpLastUsedDerivationIndex sql.NullInt64
		kpSyncedFrom              sql.NullString
		kpClock                   sql.NullInt64
	)

	var (
		accAddress   sql.NullString
		accKeyUID    sql.NullString
		accPath      sql.NullString
		accName      sql.NullString
		accColorID   sql.NullString
		accEmoji     sql.NullString
		accWallet    sql.NullBool
		accChat      sql.NullBool
		accHidden    sql.NullBool
		accOperable  sql.NullString
		accClock     sql.NullInt64
		accCreatedAt sql.NullTime
		accPosition  sql.NullInt64
	)

	for rows.Next() {
		kp := &Keypair{}
		acc := &Account{}
		pubkey := []byte{}
		err := rows.Scan(
			&kpKeyUID, &kpName, &kpType, &kpDerivedFrom, &kpLastUsedDerivationIndex, &kpSyncedFrom, &kpClock,
			&accAddress, &accKeyUID, &pubkey, &accPath, &accName, &accColorID, &accEmoji,
			&accWallet, &accChat, &accHidden, &accOperable, &accClock, &accCreatedAt, &accPosition)
		if err != nil {
			return nil, nil, err
		}

		// check keypair fields
		if kpKeyUID.Valid {
			kp.KeyUID = kpKeyUID.String
		}
		if kpName.Valid {
			kp.Name = kpName.String
		}
		if kpType.Valid {
			kp.Type = KeypairType(kpType.String)
		}
		if kpDerivedFrom.Valid {
			kp.DerivedFrom = kpDerivedFrom.String
		}
		if kpLastUsedDerivationIndex.Valid {
			kp.LastUsedDerivationIndex = uint64(kpLastUsedDerivationIndex.Int64)
		}
		if kpSyncedFrom.Valid {
			kp.SyncedFrom = kpSyncedFrom.String
		}
		if kpClock.Valid {
			kp.Clock = uint64(kpClock.Int64)
		}
		// check keypair accounts fields
		if accAddress.Valid {
			acc.Address = types.BytesToAddress([]byte(accAddress.String))
		}
		if accKeyUID.Valid {
			acc.KeyUID = accKeyUID.String
		}
		if accPath.Valid {
			acc.Path = accPath.String
		}
		if accName.Valid {
			acc.Name = accName.String
		}
		if accColorID.Valid {
			acc.ColorID = common.CustomizationColor(accColorID.String)
		}
		if accEmoji.Valid {
			acc.Emoji = accEmoji.String
		}
		if accWallet.Valid {
			acc.Wallet = accWallet.Bool
		}
		if accChat.Valid {
			acc.Chat = accChat.Bool
		}
		if accHidden.Valid {
			acc.Hidden = accHidden.Bool
		}
		if accOperable.Valid {
			acc.Operable = AccountOperable(accOperable.String)
		}
		if accClock.Valid {
			acc.Clock = uint64(accClock.Int64)
		}
		if accCreatedAt.Valid {
			acc.CreatedAt = accCreatedAt.Time.UnixMilli()
		}
		if accPosition.Valid {
			acc.Position = accPosition.Int64
		}
		if lth := len(pubkey); lth > 0 {
			acc.PublicKey = make(types.HexBytes, lth)
			copy(acc.PublicKey, pubkey)
		}
		acc.Type = GetAccountTypeForKeypairType(kp.Type)

		if kp.KeyUID != "" {
			if _, ok := keypairMap[kp.KeyUID]; !ok {
				keypairMap[kp.KeyUID] = kp
			}
			keypairMap[kp.KeyUID].Accounts = append(keypairMap[kp.KeyUID].Accounts, acc)
		}
		allAccounts = append(allAccounts, acc)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, err
	}

	// Convert map to list
	keypairs := make([]*Keypair, 0, len(keypairMap))
	for _, keypair := range keypairMap {
		keypairs = append(keypairs, keypair)
	}

	return keypairs, allAccounts, nil
}

// If `keyUID` is passed only keypairs which match the passed `keyUID` will be returned, if `keyUID` is empty, all keypairs will be returned.
func (db *Database) getKeypairs(tx *sql.Tx, keyUID string) ([]*Keypair, error) {
	var (
		rows  *sql.Rows
		err   error
		where string
	)
	if tx == nil {
		tx, err = db.db.Begin()
		defer func() {
			if err == nil {
				err = tx.Commit()
				return
			}
			_ = tx.Rollback()
		}()

		if err != nil {
			return nil, err
		}
	}

	if keyUID != "" {
		where = "WHERE k.key_uid = ?"
	}
	query := fmt.Sprintf( // nolint: gosec
		`
		SELECT
			k.*,
			ka.address,
			ka.key_uid,
			ka.pubkey,
			ka.path,
			ka.name,
			ka.color,
			ka.emoji,
			ka.wallet,
			ka.chat,
			ka.hidden,
			ka.operable,
			ka.clock,
			ka.created_at,
			ka.position
		FROM
			keypairs k
		LEFT JOIN
			keypairs_accounts ka
		ON
			k.key_uid = ka.key_uid
		%s
		ORDER BY
			ka.position`, where)

	stmt, err := tx.Prepare(query)
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	if where != "" {
		rows, err = stmt.Query(keyUID)
	} else {
		rows, err = stmt.Query()
	}
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	keypairs, _, err := db.processRows(rows)
	if err != nil {
		return nil, err
	}

	for _, kp := range keypairs {
		keycards, err := db.getKeycards(tx, kp.KeyUID, "")
		if err != nil {
			return nil, err
		}

		kp.Keycards = keycards
	}

	return keypairs, nil
}

func (db *Database) getKeypairByKeyUID(tx *sql.Tx, keyUID string) (*Keypair, error) {
	keypairs, err := db.getKeypairs(tx, keyUID)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	if len(keypairs) == 0 {
		return nil, ErrDbKeypairNotFound
	}

	return keypairs[0], nil
}

// If `address` is passed only accounts which match the passed `address` will be returned, if `address` is empty, all accounts will be returned.
func (db *Database) getAccounts(tx *sql.Tx, address types.Address) ([]*Account, error) {
	var (
		rows  *sql.Rows
		err   error
		where string
	)
	if address.String() != zeroAddress {
		where = "WHERE ka.address = ?"
	}

	query := fmt.Sprintf( // nolint: gosec
		`
		SELECT
			k.*,
			ka.address,
			ka.key_uid,
			ka.pubkey,
			ka.path,
			ka.name,
			ka.color,
			ka.emoji,
			ka.wallet,
			ka.chat,
			ka.hidden,
			ka.operable,
			ka.clock,
			ka.created_at,
			ka.position
		FROM
			keypairs_accounts ka
		LEFT JOIN
			keypairs k
		ON
			ka.key_uid = k.key_uid
		%s
		ORDER BY
			ka.position`, where)

	if tx == nil {
		if where != "" {
			rows, err = db.db.Query(query, address)
		} else {
			rows, err = db.db.Query(query)
		}
		if err != nil {
			return nil, err
		}
	} else {
		stmt, err := tx.Prepare(query)
		if err != nil {
			return nil, err
		}
		defer stmt.Close()

		if where != "" {
			rows, err = stmt.Query(address)
		} else {
			rows, err = stmt.Query()
		}
		if err != nil {
			return nil, err
		}
	}

	defer rows.Close()
	_, allAccounts, err := db.processRows(rows)
	if err != nil {
		return nil, err
	}

	return allAccounts, nil
}

func (db *Database) getAccountByAddress(tx *sql.Tx, address types.Address) (*Account, error) {
	accounts, err := db.getAccounts(tx, address)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	if len(accounts) == 0 {
		return nil, ErrDbAccountNotFound
	}

	return accounts[0], nil
}

func (db *Database) deleteKeypair(tx *sql.Tx, keyUID string) error {
	keypairs, err := db.getKeypairs(tx, keyUID)
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	if len(keypairs) == 0 {
		return ErrDbKeypairNotFound
	}

	query := `
		DELETE
		FROM
			keypairs
		WHERE
			key_uid = ?
	`

	if tx == nil {
		_, err := db.db.Exec(query, keyUID)
		return err
	}

	stmt, err := tx.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(keyUID)
	return err
}

func (db *Database) GetKeypairs() ([]*Keypair, error) {
	return db.getKeypairs(nil, "")
}

func (db *Database) GetKeypairByKeyUID(keyUID string) (*Keypair, error) {
	return db.getKeypairByKeyUID(nil, keyUID)
}

func (db *Database) GetAccounts() ([]*Account, error) {
	return db.getAccounts(nil, types.Address{})
}

func (db *Database) GetAccountByAddress(address types.Address) (*Account, error) {
	return db.getAccountByAddress(nil, address)
}

func (db *Database) GetWatchOnlyAccounts() (res []*Account, err error) {
	accounts, err := db.getAccounts(nil, types.Address{})
	if err != nil {
		return nil, err
	}
	for _, acc := range accounts {
		if acc.Type == AccountTypeWatch {
			res = append(res, acc)
		}
	}
	return
}

func (db *Database) IsAnyAccountPartiallyOrFullyOperableForKeyUID(keyUID string) (bool, error) {
	kp, err := db.getKeypairByKeyUID(nil, keyUID)
	if err != nil {
		return false, err
	}

	for _, acc := range kp.Accounts {
		if acc.Operable != AccountNonOperable {
			return true, nil
		}
	}
	return false, nil
}

func (db *Database) DeleteKeypair(keyUID string) error {
	return db.deleteKeypair(nil, keyUID)
}

func (db *Database) DeleteAccount(address types.Address, clock uint64) error {
	tx, err := db.db.Begin()
	defer func() {
		if err == nil {
			err = tx.Commit()
			return
		}
		_ = tx.Rollback()
	}()

	if err != nil {
		return err
	}

	acc, err := db.getAccountByAddress(tx, address)
	if err != nil {
		return err
	}

	kp, err := db.getKeypairByKeyUID(tx, acc.KeyUID)
	if err != nil && err != ErrDbKeypairNotFound {
		return err
	}

	if kp != nil && len(kp.Accounts) == 1 && kp.Accounts[0].Address == address {
		return db.deleteKeypair(tx, acc.KeyUID)
	}

	delete, err := tx.Prepare(`
		DELETE
		FROM
			keypairs_accounts
		WHERE
			address = ?
	`)
	if err != nil {
		return err
	}
	defer delete.Close()

	_, err = delete.Exec(address)
	if err != nil {
		return err
	}

	// Update keypair clock if any but the watch only account was deleted.
	if kp != nil {
		err = db.updateKeypairClock(tx, acc.KeyUID, clock)
		return err
	}

	return nil
}

func updateKeypairLastUsedIndex(tx *sql.Tx, keyUID string, index uint64, clock uint64, updateKeypairClock bool) error {
	if tx == nil {
		return errDbTransactionIsNil
	}
	var (
		err      error
		setClock string
	)
	if updateKeypairClock {
		setClock = ", clock = ?"
	}

	query := fmt.Sprintf( // nolint: gosec
		`
		UPDATE
				keypairs
			SET
				last_used_derivation_index = ?
				%s
			WHERE
				key_uid = ?`, setClock)

	if setClock != "" {
		_, err = tx.Exec(query, index, clock, keyUID)
	} else {
		_, err = tx.Exec(query, index, keyUID)
	}

	return err
}

func (db *Database) updateKeypairClock(tx *sql.Tx, keyUID string, clock uint64) error {
	if tx == nil {
		return errDbTransactionIsNil
	}

	_, err := tx.Exec(`
			UPDATE
				keypairs
			SET
				clock = ?
			WHERE
				key_uid = ?`,
		clock, keyUID)

	return err
}

func (db *Database) saveOrUpdateAccounts(tx *sql.Tx, accounts []*Account, updateKeypairClock bool) (err error) {
	if tx == nil {
		return errDbTransactionIsNil
	}

	for _, acc := range accounts {
		var relatedKeypair *Keypair
		// only watch only accounts have an empty `KeyUID` field
		var keyUID *string
		if acc.KeyUID != "" {
			relatedKeypair, err = db.getKeypairByKeyUID(tx, acc.KeyUID)
			if err != nil {
				if err == sql.ErrNoRows {
					// all accounts, except watch only accounts, must have a row in `keypairs` table with the same key uid
					continue
				}
				return err
			}
			keyUID = &acc.KeyUID
		}

		_, err = tx.Exec(`
			INSERT OR IGNORE INTO
				keypairs_accounts (address, key_uid, pubkey, path, wallet, chat, created_at, updated_at)
			VALUES
				(?, ?, ?, ?, ?, ?, datetime('now'), datetime('now'));

			UPDATE
				keypairs_accounts
			SET
				name = ?,
				color = ?,
				emoji = ?,
				hidden = ?,
				operable = ?,
				clock = ?,
				position = ?,
				updated_at = datetime('now')
			WHERE
				address = ?;
		`,
			acc.Address, keyUID, acc.PublicKey, acc.Path, acc.Wallet, acc.Chat,
			acc.Name, acc.ColorID, acc.Emoji, acc.Hidden, acc.Operable, acc.Clock, acc.Position, acc.Address)

		if err != nil {
			return err
		}

		// Update positions change clock when adding new/updating account
		err = db.setClockOfLastAccountsPositionChange(tx, acc.Clock)
		if err != nil {
			return err
		}

		// Update keypair clock if any but the watch only account has changed.
		if relatedKeypair != nil && updateKeypairClock {
			err = db.updateKeypairClock(tx, acc.KeyUID, acc.Clock)
			if err != nil {
				return err
			}
		}

		if strings.HasPrefix(acc.Path, statusWalletRootPath) {
			accIndex, err := strconv.ParseUint(acc.Path[len(statusWalletRootPath):], 0, 64)
			if err != nil {
				return err
			}

			accountsContainPath := func(accounts []*Account, path string) bool {
				for _, acc := range accounts {
					if acc.Path == path {
						return true
					}
				}
				return false
			}

			expectedNewKeypairIndex := uint64(0)
			if relatedKeypair != nil {
				expectedNewKeypairIndex = relatedKeypair.LastUsedDerivationIndex
				for {
					expectedNewKeypairIndex++
					if !accountsContainPath(relatedKeypair.Accounts, statusWalletRootPath+strconv.FormatUint(expectedNewKeypairIndex, 10)) {
						break
					}
				}
			}

			if accIndex == expectedNewKeypairIndex {
				err = updateKeypairLastUsedIndex(tx, acc.KeyUID, accIndex, acc.Clock, updateKeypairClock)
				if err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// Saves accounts, if an account already exists, it will be updated.
func (db *Database) SaveOrUpdateAccounts(accounts []*Account, updateKeypairClock bool) error {
	if len(accounts) == 0 {
		return errors.New("no provided accounts to save/update")
	}

	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err == nil {
			err = tx.Commit()
			return
		}
		_ = tx.Rollback()
	}()
	err = db.saveOrUpdateAccounts(tx, accounts, updateKeypairClock)
	return err
}

// Saves a keypair and its accounts, if a keypair with `key_uid` already exists, it will be updated,
// if any of its accounts exists it will be updated as well, otherwise it will be added.
// Since keypair type contains `Keycards` as well, they are excluded from the saving/updating this way regardless they
// are set or not.
func (db *Database) SaveOrUpdateKeypair(keypair *Keypair) error {
	if keypair == nil {
		return errDbPassedParameterIsNil
	}

	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err == nil {
			err = tx.Commit()
			return
		}
		_ = tx.Rollback()
	}()

	// If keypair is being saved, not updated, then it must be at least one account and all accounts must have the same key uid.
	dbKeypair, err := db.getKeypairByKeyUID(tx, keypair.KeyUID)
	if err != nil && err != ErrDbKeypairNotFound {
		return err
	}
	if dbKeypair == nil {
		if len(keypair.Accounts) == 0 {
			return ErrKeypairWithoutAccounts
		}
		for _, acc := range keypair.Accounts {
			if acc.KeyUID == "" || acc.KeyUID != keypair.KeyUID {
				return ErrKeypairDifferentAccountsKeyUID
			}
		}
	}

	_, err = tx.Exec(`
		INSERT OR IGNORE INTO
			keypairs (key_uid, type, derived_from)
		VALUES
			(?, ?, ?);

		UPDATE
			keypairs
		SET
			name = ?,
			last_used_derivation_index = ?,
			synced_from = ?,
			clock = ?
		WHERE
			key_uid = ?;
	`, keypair.KeyUID, keypair.Type, keypair.DerivedFrom,
		keypair.Name, keypair.LastUsedDerivationIndex, keypair.SyncedFrom, keypair.Clock, keypair.KeyUID)
	if err != nil {
		return err
	}
	return db.saveOrUpdateAccounts(tx, keypair.Accounts, false)
}

func (db *Database) UpdateKeypairName(keyUID string, name string, clock uint64, updateChatAccountName bool) error {
	tx, err := db.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err == nil {
			err = tx.Commit()
			return
		}
		_ = tx.Rollback()
	}()

	_, err = db.getKeypairByKeyUID(tx, keyUID)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		UPDATE
			keypairs
		SET
			name = ?,
			clock = ?
		WHERE
			key_uid = ?;
	`, name, clock, keyUID)
	if err != nil {
		return err
	}

	if updateChatAccountName {
		_, err = tx.Exec(`
			UPDATE
				keypairs_accounts
			SET
				name = ?,
				clock = ?
			WHERE
				key_uid = ?
			AND
				path = ?;
		`, name, clock, keyUID, statusChatPath)
		return err
	}

	return nil
}

func (db *Database) GetWalletAddress() (rst types.Address, err error) {
	err = db.db.QueryRow("SELECT address FROM keypairs_accounts WHERE wallet = 1").Scan(&rst)
	return
}

func (db *Database) GetWalletAddresses() (rst []types.Address, err error) {
	rows, err := db.db.Query("SELECT address FROM keypairs_accounts WHERE chat = 0 ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		addr := types.Address{}
		err = rows.Scan(&addr)
		if err != nil {
			return nil, err
		}
		rst = append(rst, addr)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return rst, nil
}

func (db *Database) GetChatAddress() (rst types.Address, err error) {
	err = db.db.QueryRow("SELECT address FROM keypairs_accounts WHERE chat = 1").Scan(&rst)
	return
}

func (db *Database) GetAddresses() (rst []types.Address, err error) {
	rows, err := db.db.Query("SELECT address FROM keypairs_accounts ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		addr := types.Address{}
		err = rows.Scan(&addr)
		if err != nil {
			return nil, err
		}
		rst = append(rst, addr)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return rst, nil
}

func (db *Database) keypairExists(tx *sql.Tx, keyUID string) (exists bool, err error) {
	query := `SELECT EXISTS (SELECT 1 FROM keypairs WHERE key_uid = ?)`

	if tx == nil {
		err = db.db.QueryRow(query, keyUID).Scan(&exists)
	} else {
		err = tx.QueryRow(query, keyUID).Scan(&exists)
	}

	return exists, err
}

// KeypairExists returns true if given address is stored in database.
func (db *Database) KeypairExists(keyUID string) (exists bool, err error) {
	return db.keypairExists(nil, keyUID)
}

// AddressExists returns true if given address is stored in database.
func (db *Database) AddressExists(address types.Address) (exists bool, err error) {
	err = db.db.QueryRow("SELECT EXISTS (SELECT 1 FROM keypairs_accounts WHERE address = ?)", address).Scan(&exists)
	return exists, err
}

// GetPath returns true if account with given address was recently key and doesn't have a key yet
func (db *Database) GetPath(address types.Address) (path string, err error) {
	err = db.db.QueryRow("SELECT path FROM keypairs_accounts WHERE address = ?", address).Scan(&path)
	return path, err
}

func (db *Database) GetNodeConfig() (*params.NodeConfig, error) {
	return nodecfg.GetNodeConfigFromDB(db.db)
}

// This function should not update the clock, cause it marks accounts locally.
func (db *Database) SetAccountOperability(address types.Address, operable AccountOperable) (err error) {
	tx, err := db.db.Begin()
	defer func() {
		if err == nil {
			err = tx.Commit()
			return
		}
		_ = tx.Rollback()
	}()

	if err != nil {
		return err
	}

	_, err = db.getAccountByAddress(tx, address)
	if err != nil {
		return err
	}

	_, err = tx.Exec(`UPDATE keypairs_accounts SET operable = ?	WHERE address = ?`, operable, address)
	return err
}

func (db *Database) GetPositionForNextNewAccount() (int64, error) {
	var pos sql.NullInt64
	err := db.db.QueryRow("SELECT MAX(position) FROM keypairs_accounts").Scan(&pos)
	if err != nil {
		return 0, err
	}
	if pos.Valid {
		return pos.Int64 + 1, nil
	}
	return 0, nil
}

// This function should not be used directly, it is called from the functions which reorders accounts.
func (db *Database) setClockOfLastAccountsPositionChange(tx *sql.Tx, clock uint64) error {
	if tx == nil {
		return nil
	}
	_, err := tx.Exec("UPDATE settings SET wallet_accounts_position_change_clock = ? WHERE synthetic_id = 'id'", clock)
	return err
}

func (db *Database) GetClockOfLastAccountsPositionChange() (result uint64, err error) {
	query := "SELECT wallet_accounts_position_change_clock FROM settings WHERE synthetic_id = 'id'"
	err = db.db.QueryRow(query).Scan(&result)
	if err != nil {
		return 0, err
	}
	return result, err
}

// Updates positions of accounts respecting current order.
func (db *Database) ResolveAccountsPositions(clock uint64) (err error) {
	tx, err := db.db.Begin()
	defer func() {
		if err == nil {
			err = tx.Commit()
			return
		}
		_ = tx.Rollback()
	}()

	// returns all accounts ordered by position
	dbAccounts, err := db.getAccounts(tx, types.Address{})
	if err != nil {
		return err
	}

	// starting from -1, cause `getAccounts` returns chat account as well
	for i := 0; i < len(dbAccounts); i++ {
		expectedPosition := int64(i - 1)
		if dbAccounts[i].Position != expectedPosition {
			_, err = tx.Exec("UPDATE keypairs_accounts SET position = ? WHERE address = ?", expectedPosition, dbAccounts[i].Address)
			if err != nil {
				return err
			}
		}
	}

	return db.setClockOfLastAccountsPositionChange(tx, clock)
}

// Sets positions for passed accounts.
func (db *Database) SetWalletAccountsPositions(accounts []*Account, clock uint64) (err error) {
	if len(accounts) == 0 {
		return nil
	}
	for _, acc := range accounts {
		if acc.Position < 0 {
			return ErrAccountWrongPosition
		}
	}
	tx, err := db.db.Begin()
	defer func() {
		if err == nil {
			err = tx.Commit()
			return
		}
		_ = tx.Rollback()
	}()

	dbAccounts, err := db.getAccounts(tx, types.Address{})
	if err != nil {
		return err
	}

	// we need to subtract 1, because of the chat account
	if len(dbAccounts)-1 != len(accounts) {
		return ErrNotTheSameNumberOdAccountsToApplyReordering
	}

	for _, dbAcc := range dbAccounts {
		if dbAcc.Chat {
			continue
		}
		found := false
		for _, acc := range accounts {
			if dbAcc.Address == acc.Address {
				found = true
				break
			}
		}
		if !found {
			return ErrNotTheSameAccountsToApplyReordering
		}
	}

	for _, acc := range accounts {
		_, err = tx.Exec("UPDATE keypairs_accounts SET position = ? WHERE address = ?", acc.Position, acc.Address)
		if err != nil {
			return err
		}
	}

	return db.setClockOfLastAccountsPositionChange(tx, clock)
}

// Moves wallet account fromPosition to toPosition.
func (db *Database) MoveWalletAccount(fromPosition int64, toPosition int64, clock uint64) (err error) {
	if fromPosition < 0 || toPosition < 0 || fromPosition == toPosition {
		return ErrMovingAccountToWrongPosition
	}
	tx, err := db.db.Begin()
	defer func() {
		if err == nil {
			err = tx.Commit()
			return
		}
		_ = tx.Rollback()
	}()

	var (
		newMaxPosition int64
		newMinPosition int64
	)
	err = tx.QueryRow("SELECT MAX(position), MIN(position) FROM keypairs_accounts").Scan(&newMaxPosition, &newMinPosition)
	if err != nil {
		return err
	}
	newMaxPosition++
	newMinPosition--

	if toPosition > fromPosition {
		_, err = tx.Exec("UPDATE keypairs_accounts SET position = ? WHERE position = ?", newMaxPosition, fromPosition)
		if err != nil {
			return err
		}
		for i := fromPosition + 1; i <= toPosition; i++ {
			_, err = tx.Exec("UPDATE keypairs_accounts SET position = ? WHERE position = ?", i-1, i)
			if err != nil {
				return err
			}
		}
		_, err = tx.Exec("UPDATE keypairs_accounts SET position = ? WHERE position = ?", toPosition, newMaxPosition)
		if err != nil {
			return err
		}
	} else {
		_, err = tx.Exec("UPDATE keypairs_accounts SET position = ? WHERE position = ?", newMinPosition, fromPosition)
		if err != nil {
			return err
		}
		for i := fromPosition - 1; i >= toPosition; i-- {
			_, err = tx.Exec("UPDATE keypairs_accounts SET position = ? WHERE position = ?", i+1, i)
			if err != nil {
				return err
			}
		}
		_, err = tx.Exec("UPDATE keypairs_accounts SET position = ? WHERE position = ?", toPosition, newMinPosition)
		if err != nil {
			return err
		}
	}

	return db.setClockOfLastAccountsPositionChange(tx, clock)
}
