
all: firefly-wallet

depend:
	git submodule update --init --recursive
	make -C extern/filecoin-ffi all

clean:
	rm firefly-wallet
	make -C extern/filecoin-ffi clean


firefly-wallet: depend
	go build ./
