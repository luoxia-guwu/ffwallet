package impl

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCreateUsdtAddress(t *testing.T) {
	mnemonic := "sort inmate response auto magic kidney industry cook famous timber cousin section"
	addr, err := CreateUsdtAddress(0, mnemonic)
	assert.Nil(t, err, "CreateUsdtAddress")
	fmt.Println("addr: ", addr)
}

