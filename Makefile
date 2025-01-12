
all: firefly-wallet

depend:
	git submodule update --init --recursive
	make -C extern/filecoin-ffi all

clean:
	rm firefly-wallet
	make -C extern/filecoin-ffi clean


firefly-wallet: depend
	go build ./
	#go get --ldflags '-extldflags "-Wl,--allow-multiple-definition"' .
	#go build --ldflags '-extldflags "-Wl,--allow-multiple-definition"'
