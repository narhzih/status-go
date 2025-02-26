package pairing

import (
	"fmt"
	"io/fs"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"

	"gopkg.in/go-playground/validator.v9"

	"github.com/status-im/status-go/account/generator"
	"github.com/status-im/status-go/eth-node/keystore"
)

func newValidate() (*validator.Validate, error) {
	var validate = validator.New()
	var keyUIDPattern = regexp.MustCompile(`^0x[0-9a-fA-F]{64}$`)
	if err := validate.RegisterValidation("keyuid", func(fl validator.FieldLevel) bool {
		return keyUIDPattern.MatchString(fl.Field().String())
	}); err != nil {
		return nil, err
	}

	if err := validate.RegisterValidation("keystorepath", func(fl validator.FieldLevel) bool {
		keyUIDField := fl.Parent()
		if keyUIDField.Kind() == reflect.Ptr {
			keyUIDField = keyUIDField.Elem()
		}

		keyUID := keyUIDField.FieldByName("KeyUID").String()
		return strings.HasSuffix(fl.Field().String(), keyUID)
	}); err != nil {
		return nil, err
	}

	if err := validate.RegisterValidation("not_end_keyuid", func(fl validator.FieldLevel) bool {
		keystorePath := fl.Field().String()
		return len(keystorePath) <= 66 || !keyUIDPattern.MatchString(keystorePath[len(keystorePath)-66:])
	}); err != nil {
		return nil, err
	}
	return validate, nil
}

func validateKeys(keys map[string][]byte, password string) error {
	for _, key := range keys {
		k, err := keystore.DecryptKey(key, password)
		if err != nil {
			return err
		}

		err = generator.ValidateKeystoreExtendedKey(k)
		if err != nil {
			return err
		}
	}

	return nil
}

func loadKeys(keys map[string][]byte, keyStorePath string) error {
	fileWalker := func(path string, dirEntry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if dirEntry.IsDir() || filepath.Dir(path) != keyStorePath {
			return nil
		}

		rawKeyFile, err := ioutil.ReadFile(path)
		if err != nil {
			return fmt.Errorf("invalid account key file: %v", err)
		}

		keys[dirEntry.Name()] = rawKeyFile

		return nil
	}

	err := filepath.WalkDir(keyStorePath, fileWalker)
	if err != nil {
		return fmt.Errorf("cannot traverse key store folder: %v", err)
	}

	return nil
}

func validate(s interface{}) error {
	v, err := newValidate()
	if err != nil {
		return err
	}

	return v.Struct(s)
}

func validateAndVerifyPassword(s interface{}, senderConfig *SenderConfig) error {
	err := validate(s)
	if err != nil {
		return err
	}

	keys := make(map[string][]byte)
	err = loadKeys(keys, senderConfig.KeystorePath)
	if err != nil {
		return err
	}

	return validateKeys(keys, senderConfig.Password)
}

func validateAndVerifyNodeConfig(s interface{}, receiverConfig *ReceiverConfig) error {
	if receiverConfig.NodeConfig != nil {
		// we should update with defaults before validation
		err := receiverConfig.NodeConfig.UpdateWithDefaults()
		if err != nil {
			return err
		}
	}

	err := validate(s)
	if err != nil {
		return err
	}

	if receiverConfig.NodeConfig == nil {
		return fmt.Errorf("node config is required for receiver config")
	}

	if receiverConfig.NodeConfig.RootDataDir == "" {
		return fmt.Errorf("root data dir is required for node config")
	}

	return receiverConfig.NodeConfig.Validate()
}

func emptyDir(dir string) error {
	// Open the directory
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()

	// Get all the directory entries
	entries, err := d.Readdir(-1)
	if err != nil {
		return err
	}

	// Remove all the files and directories
	for _, entry := range entries {
		name := entry.Name()
		if name == "." || name == ".." {
			continue
		}
		path := filepath.Join(dir, name)
		if entry.IsDir() {
			err = os.RemoveAll(path)
			if err != nil {
				return err
			}
		} else {
			err = os.Remove(path)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
