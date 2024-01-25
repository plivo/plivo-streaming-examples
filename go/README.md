## Go Installation 
```
# ARCH=$(uname -m |sed 's/aarch64/arm64/' |sed 's/x86_64/amd64/')
# curl -o /opt/go1.17.tar.gz https://storage.googleapis.com/golang/go1.17.linux-${ARCH}.tar.gz && rm -rf /usr/local/go && tar -C /usr/local -xzf /opt/go1.17.tar.gz && rm -rf /opt/go && ln -s /usr/local/go /opt/go
# export GOROOT=/opt/go
# export PATH=$PATH:$GOROOT/bin
```


## Building binary
```
go mod tidy
go build . -o server
```

## command line flags description
```
  -bidirectional bool
    	bidirectional support (default true)
  -interval int
    	Timer interval,  used for sending payload data at regular intervals. (default 5)
  -max_times int
    	How many times payload should be sent. This is used in conjunction with the timer to control the number of payload sends. (default 1)
  -p string
    	port to listen (default "8080")
  -path string
    	directory path where the files will be stored. (default 'the current working directory')
  -payload string
    	bidirectional payload file full path (default "./payload.raw")
  -sample_rate int
    	payload sample rate (default 8000)
```

## Example usage of server
```
./server -p "8090" -payload "payload.raw"
```
