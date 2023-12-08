module github.com/filecoin-project/firefly-wallet

go 1.15

require (
	github.com/btcsuite/btcd v0.22.1
	github.com/btcsuite/btcutil v1.0.3-0.20201208143702-a53e38424cce
	github.com/ethereum/go-ethereum v1.10.4
	github.com/fatih/color v1.13.0
	github.com/filecoin-project/filecoin-ffi v0.30.4-0.20220519234331-bfd1f5f9fe38
	github.com/filecoin-project/go-address v1.1.0
	github.com/filecoin-project/go-bitfield v0.2.4
	github.com/filecoin-project/go-fil-markets v1.28.3
	github.com/filecoin-project/go-state-types v0.12.8
	github.com/filecoin-project/index-provider v0.9.1 // indirect
	github.com/filecoin-project/lotus v1.24.0
	github.com/filecoin-project/specs-actors/v2 v2.3.6
	github.com/filecoin-project/specs-actors/v5 v5.0.6
	github.com/filecoin-project/specs-actors/v6 v6.0.2
	github.com/gdamore/tcell/v2 v2.4.0 // indirect
	github.com/hako/durafmt v0.0.0-20200710122514-c0fb7b4da026
	github.com/howeyc/gopass v0.0.0-20190910152052-7cb4b85ec19c
	github.com/ipfs/go-cid v0.4.1
	github.com/ipfs/go-cidutil v0.1.0
	github.com/ipfs/go-ipld-cbor v0.0.6
	github.com/mitchellh/go-homedir v1.1.0
	github.com/multiformats/go-multibase v0.2.0
	github.com/stretchr/testify v1.8.4
	github.com/syndtr/goleveldb v1.0.1-0.20210819022825-2ae1ddf74ef7
	github.com/tyler-smith/go-bip39 v1.1.0
	github.com/urfave/cli/v2 v2.25.5
	github.com/whyrusleeping/cbor-gen v0.0.0-20230923211252-36a87e1ba72f
	golang.org/x/crypto v0.12.0
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2
)

replace github.com/filecoin-project/filecoin-ffi => ./extern/filecoin-ffi

replace github.com/ipfs/go-ipld-cbor v0.0.6-0.20211211231443-5d9b9e1f6fa8 => github.com/ipfs/go-ipld-cbor v0.0.5
