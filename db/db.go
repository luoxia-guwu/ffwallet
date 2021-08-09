package db

import (
	"fmt"
	"github.com/syndtr/goleveldb/leveldb"
	"strings"
)

type LocalDb struct {
	db *leveldb.DB
}

func Init(path string) (*LocalDb, error) {

	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, err
	}
	return &LocalDb{db: db}, nil
}

func (lb *LocalDb) Add(keytype KeyType, key string, value []byte) error {
	return lb.db.Put([]byte(lb.getKey(keytype, key)), value, nil)
}

func (lb *LocalDb) Get(keytype KeyType, key string) ([]byte, error) {

	return lb.db.Get([]byte(lb.getKey(keytype, key)), nil)
}

func (lb *LocalDb) Del(keytype KeyType, key string) error {

	return lb.db.Delete([]byte(lb.getKey(keytype, key)), nil)
}

type KeyType string

func (k KeyType) String() string {
	return string(k)
}

const (
	KeyAddr   KeyType = "filAddr"
	KeyIndex  KeyType = "filIndex"
	KeyCommon KeyType = "commonKey"
	KeyPriKey KeyType = "filPriKey"
)

func (lb *LocalDb) getKey(keyType KeyType, key string) string {
	return fmt.Sprintf("%s-%s", keyType, key)
}

func (lb *LocalDb) GetAll(keyType KeyType) (map[string]string, error) {
	mapRlt := map[string]string{}
	iter := lb.db.NewIterator(nil, nil)
	for iter.Next() {
		// Remember that the contents of the returned slice should not be modified, and
		// only valid until the next call to Next.
		key := iter.Key()
		value := iter.Value()

		keyElems := strings.Split(string(key), "-")
		if len(keyElems) < 1 || keyElems[0] != keyType.String() {
			continue
		}
		mapRlt[strings.Join(keyElems[1:], "-")] = string(value)
	}
	iter.Release()
	err := iter.Error()
	if err != nil {
		fmt.Printf(err.Error())
	}
	return mapRlt, err
}
