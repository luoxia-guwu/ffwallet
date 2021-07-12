
all: firefly-wallet

depend:
	make -C extern/filecoin-ffi all

clean:
	rm firefly-wallet
	make -C extern/filecoin-ffi clean


firefly-wallet: depend
	go build ./
