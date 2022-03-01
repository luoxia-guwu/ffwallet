package mnemonic

import (
	"encoding/json"
	"golang.org/x/crypto/scrypt"
)

func Decrypt(hiddenData, passwd []byte) ([]byte, error) {

	hid := new(hidden)
	err := json.Unmarshal(hiddenData, hid)
	if err != nil {
		return []byte{}, err
	}
	plainText, err := decryptData(hid.Mnemonic, passwd, hid.Salt, hid.Iv)
	return plainText, err
}

func decryptData(cipherText, auth, salt, iv []byte) ([]byte, error) {

	derivedKey, err := scrypt.Key(auth, salt, scryptN, scryptR, scryptP, scryptDKLen)
	if err != nil {
		return nil, err
	}

	plainText, err := aesCTRXOR(derivedKey[:16], cipherText, iv)
	if err != nil {
		return nil, err
	}
	return plainText, err
}
